import { describe, expect, test } from "bun:test";

import { InMemoryPrivateActivityLedgerRepository } from "@/features/activity/in-memory-repository";
import { InMemoryRetentionRunRepository } from "@/features/retention/in-memory-repository";
import type {
  RetentionBacklogProbe,
  RetentionRunRepository,
  RetentionStage,
} from "@/features/retention/repository";
import {
  RetentionRunService,
  retentionRunLeaseMilliseconds,
} from "@/features/retention/service";

const runId = "rtn_00000000-0000-4000-8000-000000000001";

describe("private retention observability", () => {
  test("records counters, current backlog, and an append-only success event", async () => {
    const activity = activityRepository();
    const repository = retentionRepository(activity);
    let now = new Date("2026-07-26T12:00:00.000Z");
    const oldestDueAt = new Date("2026-07-26T11:45:00.000Z");
    const service = new RetentionRunService(
      repository,
      backlog(oldestDueAt),
      [
        stage("artifacts", {
          counters: {
            artifactCandidates: 3,
            artifactsDeleted: 3,
          },
        }),
        stage("inbound_email_replays", {
          counters: { inboundReplayRecordsDeleted: 7 },
        }),
        stage("console_sessions", {
          counters: { consoleSessionRecordsDeleted: 2 },
        }),
      ],
      () => now,
      () => runId,
    );

    const run = await service.run();

    expect(run).toMatchObject({
      counters: {
        artifactCandidates: 3,
        artifactsDeleted: 3,
        inboundReplayRecordsDeleted: 7,
        stagesAttempted: 3,
        stagesFailed: 0,
        stagesSucceeded: 3,
      },
      failedAreas: [],
      oldestDueAt,
      status: "succeeded",
    });
    now = new Date("2026-07-26T12:01:00.000Z");
    expect(await service.health()).toMatchObject({
      activeRunCount: 0,
      lastRunId: runId,
      oldestDueAt,
      status: "succeeded",
    });
    const events = await activity.recent(10);
    expect(events.map((event) => event.eventName)).toEqual([
      "private.retention_run.succeeded",
      "private.retention_run.started",
    ]);
  });

  test("isolates cleanup failures and finalizes a partial run before returning 503", async () => {
    const order: string[] = [];
    const activity = activityRepository();
    const repository = retentionRepository(activity);
    const service = new RetentionRunService(
      repository,
      backlog(null),
      [
        failingStage("artifacts", order),
        stage("inbound_email_replays", {
          counters: { inboundReplayRecordsDeleted: 4 },
        }, order),
        failingStage("console_sessions", order),
        stage("console_rate_limits", {
          counters: { consoleRateLimitRecordsDeleted: 5 },
        }, order),
      ],
      () => new Date("2026-07-26T12:00:00.000Z"),
      () => runId,
    );

    await expect(service.run()).rejects.toMatchObject({
      code: "retention_incomplete",
      status: 503,
    });

    expect(order).toEqual([
      "artifacts",
      "inbound_email_replays",
      "console_sessions",
      "console_rate_limits",
    ]);
    expect(await repository.health(null)).toMatchObject({
      activeRunCount: 0,
      counters: {
        consoleRateLimitRecordsDeleted: 5,
        inboundReplayRecordsDeleted: 4,
        stagesAttempted: 4,
        stagesFailed: 2,
        stagesSucceeded: 2,
      },
      status: "partial",
    });
    const terminal = (await activity.recent(1))[0];
    expect(terminal).toMatchObject({
      details: {
        failedAreas: ["artifacts", "console_sessions"],
        status: "partial",
      },
      eventName: "private.retention_run.partial",
    });
  });

  test("records failed when every isolated stage throws", async () => {
    const activity = activityRepository();
    const repository = retentionRepository(activity);
    const service = new RetentionRunService(
      repository,
      backlog(null),
      [
        failingStage("artifacts"),
        failingStage("inbound_email_replays"),
      ],
      () => new Date("2026-07-26T12:00:00.000Z"),
      () => runId,
    );

    await expect(service.run()).rejects.toMatchObject({ status: 503 });
    expect(await repository.health(null)).toMatchObject({
      activeRunCount: 0,
      status: "failed",
    });
    expect((await activity.recent(1))[0]?.eventName).toBe(
      "private.retention_run.failed",
    );
  });

  test("turns stale running leases into abandoned terminal records", async () => {
    const activity = activityRepository();
    const repository = retentionRepository(activity);
    const startedAt = new Date("2026-07-26T12:00:00.000Z");
    await repository.start({ id: runId, startedAt });
    const abandonedAt = new Date(
      startedAt.getTime() + retentionRunLeaseMilliseconds,
    );

    const abandoned = await repository.abandonStale({
      abandonedAt,
      staleBefore: abandonedAt,
    });

    expect(abandoned).toHaveLength(1);
    expect(abandoned[0]).toMatchObject({
      failedAreas: ["run_lease"],
      finishedAt: abandonedAt,
      status: "abandoned",
    });
    expect(await repository.health(null)).toMatchObject({
      activeRunCount: 0,
      status: "abandoned",
    });
    expect((await activity.recent(1))[0]?.eventName).toBe(
      "private.retention_run.abandoned",
    );
  });

  test("reports a live run as running without confusing backlog age with run age", async () => {
    const activity = activityRepository();
    const repository = retentionRepository(activity);
    const startedAt = new Date("2026-07-26T12:00:00.000Z");
    const oldestDueAt = new Date("2026-07-25T08:00:00.000Z");
    await repository.start({ id: runId, startedAt });

    expect(await repository.health(oldestDueAt)).toEqual({
      activeRunCount: 1,
      counters: expect.objectContaining({
        stagesAttempted: 0,
        stagesFailed: 0,
        stagesSucceeded: 0,
      }),
      lastFinishedAt: null,
      lastRunId: runId,
      lastStartedAt: startedAt,
      oldestDueAt,
      status: "running",
    });
  });

  test("rejects non-allowlisted activity detail instead of storing secrets", async () => {
    const activity = activityRepository();
    await expect(activity.append({
      actor: "system:retention",
      details: {
        objectKey: "private/artifact/customer-path",
        token: "secret",
      },
      eventId: "act_00000000-0000-4000-8000-000000000099",
      eventName: "private.retention_run.started",
      recordedAt: new Date("2026-07-26T12:00:00.000Z"),
      subject: { id: runId, type: "retention_run" },
    } as never)).rejects.toThrow();
    expect(await activity.recent(10)).toEqual([]);
  });

  test("aborts the active stage and never finishes after losing its fence", async () => {
    const activity = activityRepository();
    const inner = retentionRepository(activity);
    let heartbeatCalls = 0;
    const repository: RetentionRunRepository = {
      abandonStale: (input) => inner.abandonStale(input),
      finish: (input) => inner.finish(input),
      health: (oldestDueAt) => inner.health(oldestDueAt),
      heartbeat: async (id, at) => {
        heartbeatCalls += 1;
        return heartbeatCalls === 1 && inner.heartbeat(id, at);
      },
      start: (input) => inner.start(input),
    };
    const executed: string[] = [];
    const service = new RetentionRunService(
      repository,
      backlog(null),
      [
        {
          execute: async (_now, signal) => {
            executed.push("artifacts");
            await new Promise<void>((_resolve, reject) => {
              signal.addEventListener("abort", () => reject(signal.reason), { once: true });
            });
            return {};
          },
          name: "artifacts",
        },
        stage("console_sessions", {}),
      ],
      () => new Date("2026-07-26T12:00:00.000Z"),
      () => runId,
      5,
    );

    await expect(service.run()).rejects.toMatchObject({
      code: "retention_lease_lost",
      status: 503,
    });
    expect(executed).toEqual(["artifacts"]);
    expect(await inner.health(null)).toMatchObject({
      activeRunCount: 1,
      status: "running",
    });
    expect((await activity.recent(10)).map((event) => event.eventName)).toEqual([
      "private.retention_run.started",
    ]);
  });
});

function activityRepository(): InMemoryPrivateActivityLedgerRepository {
  return new InMemoryPrivateActivityLedgerRepository();
}

function retentionRepository(
  activity: InMemoryPrivateActivityLedgerRepository,
): InMemoryRetentionRunRepository {
  let sequence = 0;
  return new InMemoryRetentionRunRepository(activity, () => {
    sequence += 1;
    return `act_00000000-0000-4000-8000-${String(sequence).padStart(12, "0")}`;
  });
}

function backlog(value: Date | null): RetentionBacklogProbe {
  return {
    async oldestDueAt(): Promise<Date | null> {
      return value ? new Date(value) : null;
    },
  };
}

function stage(
  name: RetentionStage["name"],
  result: Awaited<ReturnType<RetentionStage["execute"]>>,
  order?: string[],
): RetentionStage {
  return {
    async execute() {
      order?.push(name);
      return result;
    },
    name,
  };
}

function failingStage(
  name: RetentionStage["name"],
  order?: string[],
): RetentionStage {
  return {
    async execute() {
      order?.push(name);
      throw new Error("synthetic cleanup failure with /path and token");
    },
    name,
  };
}
