import { randomUUID } from "node:crypto";

import {
  emptyRetentionCounters,
  retentionStageResultSchema,
  type RetentionCounters,
  type RetentionFailureArea,
  type RetentionHealth,
  type RetentionRun,
  type RetentionRunStatus,
  type RetentionWorkCounters,
} from "@/features/retention/contracts";
import type {
  RetentionBacklogProbe,
  RetentionRunRepository,
  RetentionStage,
} from "@/features/retention/repository";
import { PlatformError } from "@/shared/errors/platform-error";

export const retentionRunLeaseMilliseconds = 15 * 60 * 1_000;
export const retentionHeartbeatMilliseconds = 60 * 1_000;

export class RetentionRunService {
  constructor(
    private readonly repository: RetentionRunRepository,
    private readonly backlog: RetentionBacklogProbe,
    private readonly stages: readonly RetentionStage[],
    private readonly now: () => Date = () => new Date(),
    private readonly newRunId: () => string = () => `rtn_${randomUUID()}`,
    private readonly heartbeatMilliseconds = retentionHeartbeatMilliseconds,
  ) {
    if (stages.length === 0) {
      throw new Error("Retention requires at least one isolated stage");
    }
    const names = stages.map((stage) => stage.name);
    if (new Set(names).size !== names.length) {
      throw new Error("Retention stages must have unique names");
    }
    if (
      !Number.isSafeInteger(heartbeatMilliseconds) ||
      heartbeatMilliseconds < 1 ||
      heartbeatMilliseconds >= retentionRunLeaseMilliseconds
    ) {
      throw new Error("Retention heartbeat interval must fit inside the run lease");
    }
  }

  async health(): Promise<RetentionHealth> {
    const now = this.now();
    return this.repository.health(
      requireOldestDueAt(await this.backlog.oldestDueAt(now), now),
    );
  }

  async run(): Promise<RetentionRun> {
    const startedAt = this.now();
    await this.repository.abandonStale({
      abandonedAt: startedAt,
      staleBefore: new Date(startedAt.getTime() - retentionRunLeaseMilliseconds),
    });
    const run = await this.repository.start({ id: this.newRunId(), startedAt });
    const counters = emptyRetentionCounters();
    const failedAreas: RetentionFailureArea[] = [];
    let completedStage = false;

    for (const stage of this.stages) {
      await this.requireLease(run.id);
      counters.stagesAttempted += 1;
      try {
        const result = retentionStageResultSchema.parse(
          await this.executeWithLease(run.id, stage, startedAt),
        );
        addWorkCounters(counters, result.counters);
        completedStage = true;
        if (result.outcome === "partial") {
          counters.stagesFailed += 1;
          failedAreas.push(stage.name);
        } else {
          counters.stagesSucceeded += 1;
        }
      } catch (error) {
        if (error instanceof RetentionLeaseLostError) throw error;
        counters.stagesFailed += 1;
        failedAreas.push(stage.name);
      }
    }

    await this.requireLease(run.id);

    let oldestDueAt: Date | null = null;
    try {
      const observedAt = this.now();
      oldestDueAt = requireOldestDueAt(
        await this.backlog.oldestDueAt(observedAt),
        observedAt,
      );
    } catch {
      addFailure(failedAreas, "backlog_probe");
    }

    const status = terminalStatus(failedAreas, completedStage);
    const finished = await this.repository.finish({
      counters,
      failedAreas,
      finishedAt: this.now(),
      id: run.id,
      oldestDueAt,
      status,
    });

    if (status !== "succeeded") {
      throw retentionUnavailable(status);
    }
    return finished;
  }

  private async executeWithLease(
    runId: string,
    stage: RetentionStage,
    startedAt: Date,
  ): Promise<unknown> {
    const controller = new AbortController();
    let leaseLoss: RetentionLeaseLostError | undefined;
    let renewal = Promise.resolve();
    const timer = setInterval(() => {
      renewal = renewal.then(async () => {
        if (leaseLoss) return;
        try {
          if (!await this.repository.heartbeat(runId, this.now())) {
            leaseLoss = new RetentionLeaseLostError();
            controller.abort(leaseLoss);
          }
        } catch {
          leaseLoss = new RetentionLeaseLostError();
          controller.abort(leaseLoss);
        }
      });
    }, this.heartbeatMilliseconds);

    let result: unknown;
    let stageError: unknown;
    try {
      result = await stage.execute(startedAt, controller.signal);
    } catch (error) {
      stageError = error;
    } finally {
      clearInterval(timer);
      await renewal;
    }
    if (leaseLoss) throw leaseLoss;
    await this.requireLease(runId);
    if (stageError !== undefined) throw stageError;
    return result;
  }

  private async requireLease(runId: string): Promise<void> {
    try {
      if (await this.repository.heartbeat(runId, this.now())) return;
    } catch {
      // A worker that cannot prove ownership must stop. It must not write a
      // terminal state; a later authenticated cron run will abandon its lease.
    }
    throw new RetentionLeaseLostError();
  }
}

class RetentionLeaseLostError extends PlatformError {
  constructor() {
    super({
      code: "retention_lease_lost",
      detail: "The retention worker lost its database lease and stopped.",
      retryAfterSeconds: 60,
      status: 503,
      title: "Retention lease lost",
    });
    this.name = "RetentionLeaseLostError";
  }
}

function requireOldestDueAt(value: Date | null, observedAt: Date): Date | null {
  if (value === null) return null;
  if (Number.isNaN(value.getTime()) || value > observedAt) {
    throw new Error("The retention backlog probe returned an invalid due time");
  }
  return new Date(value);
}

function addFailure(
  failedAreas: RetentionFailureArea[],
  area: RetentionFailureArea,
): void {
  if (!failedAreas.includes(area)) failedAreas.push(area);
}

function addWorkCounters(
  counters: RetentionCounters,
  addition: Partial<RetentionWorkCounters>,
): void {
  for (const [name, value] of Object.entries(addition) as [
    keyof RetentionWorkCounters,
    number,
  ][]) {
    counters[name] += value;
  }
}

function terminalStatus(
  failedAreas: readonly RetentionFailureArea[],
  completedStage: boolean,
): Exclude<RetentionRunStatus, "running" | "abandoned"> {
  if (failedAreas.length === 0) return "succeeded";
  return completedStage ? "partial" : "failed";
}

function retentionUnavailable(
  status: Exclude<RetentionRunStatus, "running" | "abandoned">,
): PlatformError {
  return new PlatformError({
    code: "retention_incomplete",
    detail: `The private retention run finished with status '${status}'.`,
    retryAfterSeconds: 60,
    status: 503,
    title: "Retention incomplete",
  });
}
