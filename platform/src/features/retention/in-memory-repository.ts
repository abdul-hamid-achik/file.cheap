import { randomUUID } from "node:crypto";

import type { PrivateActivityLedgerRepository } from "@/features/activity/repository";
import {
  emptyRetentionCounters,
  retentionCountersSchema,
  retentionFailureAreaSchema,
  type RetentionHealth,
  type RetentionRun,
} from "@/features/retention/contracts";
import type { RetentionRunRepository } from "@/features/retention/repository";

export class InMemoryRetentionRunRepository implements RetentionRunRepository {
  private readonly runs = new Map<string, RetentionRun>();

  constructor(
    private readonly activity: PrivateActivityLedgerRepository,
    private readonly newActivityId: () => string = () => `act_${randomUUID()}`,
  ) {}

  async abandonStale(input: {
    abandonedAt: Date;
    staleBefore: Date;
  }): Promise<RetentionRun[]> {
    const abandoned: RetentionRun[] = [];
    for (const current of this.runs.values()) {
      if (current.status !== "running" || current.heartbeatAt > input.staleBefore) {
        continue;
      }
      const run: RetentionRun = {
        ...cloneRun(current),
        failedAreas: ["run_lease"],
        finishedAt: new Date(input.abandonedAt),
        heartbeatAt: new Date(input.abandonedAt),
        status: "abandoned",
      };
      await this.appendTerminalEvent(run);
      this.runs.set(run.id, run);
      abandoned.push(cloneRun(run));
    }
    return abandoned;
  }

  async finish(input: {
    counters: RetentionRun["counters"];
    failedAreas: RetentionRun["failedAreas"];
    finishedAt: Date;
    id: string;
    oldestDueAt: Date | null;
    status: Exclude<RetentionRun["status"], "running">;
  }): Promise<RetentionRun> {
    const current = this.runs.get(input.id);
    if (!current || current.status !== "running") {
      throw new Error("The retention run is not active");
    }
    const counters = retentionCountersSchema.parse(input.counters);
    const failedAreas = retentionFailureAreaSchema.array().max(8).parse(input.failedAreas);
    const run: RetentionRun = {
      ...cloneRun(current),
      counters,
      failedAreas,
      finishedAt: new Date(input.finishedAt),
      heartbeatAt: new Date(input.finishedAt),
      oldestDueAt: input.oldestDueAt ? new Date(input.oldestDueAt) : null,
      status: input.status,
    };
    await this.appendTerminalEvent(run);
    this.runs.set(run.id, run);
    return cloneRun(run);
  }

  async health(oldestDueAt: Date | null): Promise<RetentionHealth> {
    const runs = [...this.runs.values()].sort(compareNewestRun);
    const activeRuns = runs.filter((run) => run.status === "running");
    const latest = activeRuns[0] ?? runs[0] ?? null;
    return {
      activeRunCount: activeRuns.length,
      counters: latest ? structuredClone(latest.counters) : emptyRetentionCounters(),
      lastFinishedAt: latest?.finishedAt ? new Date(latest.finishedAt) : null,
      lastRunId: latest?.id ?? null,
      lastStartedAt: latest ? new Date(latest.startedAt) : null,
      oldestDueAt: oldestDueAt ? new Date(oldestDueAt) : null,
      status: latest?.status ?? null,
    };
  }

  async heartbeat(id: string, at: Date): Promise<boolean> {
    const current = this.runs.get(id);
    if (!current || current.status !== "running" || at < current.heartbeatAt) {
      return false;
    }
    this.runs.set(id, { ...cloneRun(current), heartbeatAt: new Date(at) });
    return true;
  }

  async start(input: { id: string; startedAt: Date }): Promise<RetentionRun> {
    if (this.runs.has(input.id)) {
      throw new Error("The retention run already exists");
    }
    if ([...this.runs.values()].some((run) => run.status === "running")) {
      throw new Error("A retention run is already active");
    }
    const run: RetentionRun = {
      counters: emptyRetentionCounters(),
      failedAreas: [],
      finishedAt: null,
      heartbeatAt: new Date(input.startedAt),
      id: input.id,
      oldestDueAt: null,
      startedAt: new Date(input.startedAt),
      status: "running",
    };
    await this.activity.append({
      actor: "system:retention",
      details: {},
      eventId: this.newActivityId(),
      eventName: "private.retention_run.started",
      recordedAt: new Date(input.startedAt),
      subject: { id: input.id, type: "retention_run" },
    });
    this.runs.set(run.id, run);
    return cloneRun(run);
  }

  private async appendTerminalEvent(run: RetentionRun): Promise<void> {
    if (run.status === "running") {
      throw new Error("A running retention run cannot emit a terminal event");
    }
    await this.activity.append({
      actor: "system:retention",
      details: {
        counters: structuredClone(run.counters),
        failedAreas: [...run.failedAreas],
        oldestDueAt: run.oldestDueAt ? new Date(run.oldestDueAt) : null,
        status: run.status,
      },
      eventId: this.newActivityId(),
      eventName: `private.retention_run.${run.status}`,
      recordedAt: new Date(run.finishedAt ?? run.heartbeatAt),
      subject: { id: run.id, type: "retention_run" },
    });
  }
}

function cloneRun(run: RetentionRun): RetentionRun {
  return structuredClone(run);
}

function compareNewestRun(left: RetentionRun, right: RetentionRun): number {
  const timeDifference = right.startedAt.getTime() - left.startedAt.getTime();
  return timeDifference === 0 ? right.id.localeCompare(left.id) : timeDifference;
}
