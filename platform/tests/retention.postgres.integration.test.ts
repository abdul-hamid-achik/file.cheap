import { afterAll, beforeAll, beforeEach, describe, expect, test } from "bun:test";

import { emptyRetentionCounters } from "@/features/retention/contracts";
import type { RetentionStage } from "@/features/retention/repository";
import { RetentionRunService } from "@/features/retention/service";
import { DrizzlePrivateActivityLedgerRepository } from "@/platform/database/activity-repository";
import { getDatabase } from "@/platform/database/client";
import {
  cleanupConsoleAuthorizations,
  cleanupConsoleDeviceFamilies,
  cleanupConsoleRateLimits,
  cleanupConsoleSessions,
  consoleCleanupBatchSize,
} from "@/platform/database/console-cleanup";
import { DrizzleInboundReplayRepository } from "@/platform/database/inbound-email-replay-repository";
import { DrizzleRetentionBacklogProbe } from "@/platform/database/retention-backlog";
import { DrizzleRetentionRunRepository } from "@/platform/database/retention-repository";
import {
  artifacts,
  consoleAuthorizations,
  consoleDeviceFamilies,
  consoleRateLimits,
  consoleSessions,
  consoleUsers,
  inboundEmailReplays,
} from "@/platform/database/schema";
import {
  openPostgresTestDatabase,
  truncatePostgresTestData,
} from "./postgres-test-database";

const databaseUrl = process.env.FILECHEAP_POSTGRES_TEST_URL;
const baseTime = new Date("2026-07-26T18:00:00.000Z");

describe.skipIf(!databaseUrl)("retention PostgreSQL workflow", () => {
  let harness: ReturnType<typeof openPostgresTestDatabase>;

  beforeAll(() => {
    harness = openPostgresTestDatabase();
  });

  beforeEach(async () => {
    await truncatePostgresTestData(harness);
  });

  afterAll(async () => {
    await truncatePostgresTestData(harness);
    await harness.pool.end();
  });

  test("allows one active run and atomically appends its started event", async () => {
    const first = retentionRepository(1);
    const second = retentionRepository(10);
    const results = await Promise.allSettled([
      first.start({ id: runId(1), startedAt: baseTime }),
      second.start({ id: runId(2), startedAt: baseTime }),
    ]);

    expect(results.filter((result) => result.status === "fulfilled")).toHaveLength(1);
    expect(results.filter((result) => result.status === "rejected")).toHaveLength(1);
    expect((results.find((result) => result.status === "rejected") as PromiseRejectedResult).reason)
      .toMatchObject({ code: "retention_run_active", status: 409 });
    const rows = await harness.pool.query<{ count: number }>(
      "SELECT count(*)::integer AS count FROM private_retention_runs WHERE status = 'running'",
    );
    const events = await harness.pool.query<{ count: number }>(
      "SELECT count(*)::integer AS count FROM private_activity_events WHERE event_name = 'private.retention_run.started'",
    );
    expect(rows.rows[0]?.count).toBe(1);
    expect(events.rows[0]?.count).toBe(1);
  });

  test("fences heartbeat and finish after deterministic stale abandonment", async () => {
    const repository = retentionRepository(20);
    const id = runId(20);
    await repository.start({ id, startedAt: baseTime });
    const heartbeatAt = new Date(baseTime.getTime() + 5 * 60_000);
    expect(await repository.heartbeat(id, heartbeatAt)).toBe(true);

    expect(await repository.abandonStale({
      abandonedAt: new Date(baseTime.getTime() + 20 * 60_000),
      staleBefore: new Date(heartbeatAt.getTime() - 1),
    })).toEqual([]);
    const abandoned = await repository.abandonStale({
      abandonedAt: new Date(baseTime.getTime() + 20 * 60_000),
      staleBefore: heartbeatAt,
    });
    expect(abandoned).toHaveLength(1);
    expect(abandoned[0]?.status).toBe("abandoned");
    expect(await repository.heartbeat(id, new Date(baseTime.getTime() + 21 * 60_000))).toBe(false);
    await expect(repository.finish({
      counters: emptyRetentionCounters(),
      failedAreas: [],
      finishedAt: new Date(baseTime.getTime() + 21 * 60_000),
      id,
      oldestDueAt: null,
      status: "succeeded",
    })).rejects.toMatchObject({ code: "retention_lease_lost", status: 503 });

    const replacement = await retentionRepository(30).start({
      id: runId(30),
      startedAt: new Date(baseTime.getTime() + 21 * 60_000),
    });
    expect(replacement.status).toBe("running");
  });

  test("rolls back a run transition when its terminal activity insert fails", async () => {
    const duplicateEventId = activityId(40);
    const repository = new DrizzleRetentionRunRepository(
      harness.database,
      () => duplicateEventId,
    );
    const id = runId(40);
    await repository.start({ id, startedAt: baseTime });

    await expect(repository.finish({
      counters: emptyRetentionCounters(),
      failedAreas: [],
      finishedAt: new Date(baseTime.getTime() + 1_000),
      id,
      oldestDueAt: null,
      status: "succeeded",
    })).rejects.toThrow();
    const row = await harness.pool.query<{ finished_at: Date | null; status: string }>(
      "SELECT status, finished_at FROM private_retention_runs WHERE id = $1",
      [id],
    );
    expect(row.rows).toEqual([{ finished_at: null, status: "running" }]);
    expect((await harness.pool.query<{ count: number }>(
      "SELECT count(*)::integer AS count FROM private_activity_events WHERE subject_id = $1",
      [id],
    )).rows[0]?.count).toBe(1);
  });

  test("persists partial and failed terminal runs before the service returns 503", async () => {
    const repository = retentionRepository(50);
    const partial = new RetentionRunService(
      repository,
      { oldestDueAt: async () => null },
      [successfulStage("artifacts"), failedStage("console_sessions")],
      () => baseTime,
      () => runId(50),
    );
    await expect(partial.run()).rejects.toMatchObject({ status: 503 });
    expect(await repository.health(null)).toMatchObject({
      activeRunCount: 0,
      status: "partial",
    });

    const failed = new RetentionRunService(
      retentionRepository(60),
      { oldestDueAt: async () => null },
      [failedStage("artifacts"), failedStage("console_sessions")],
      () => new Date(baseTime.getTime() + 1_000),
      () => runId(60),
    );
    await expect(failed.run()).rejects.toMatchObject({ status: 503 });
    const terminal = await harness.pool.query<{ status: string }>(`
      SELECT status FROM private_retention_runs
      WHERE id IN ('${runId(50)}', '${runId(60)}')
      ORDER BY id
    `);
    expect(terminal.rows.map((row) => row.status)).toEqual(["partial", "failed"]);
  });

  test("enforces strict activity details and append-only rows in PostgreSQL", async () => {
    const repository = retentionRepository(70);
    const id = runId(70);
    await repository.start({ id, startedAt: baseTime });
    const malformedCounters = {
      ...emptyRetentionCounters(),
      artifactCandidates: "secret-token-value",
    };
    for (const [eventId, details] of [
      [activityId(71), {
        counters: malformedCounters,
        failedAreas: ["artifacts"],
        oldestDueAt: null,
        status: "partial",
      }],
      [activityId(72), {
        counters: emptyRetentionCounters(),
        failedAreas: ["artifacts"],
        oldestDueAt: { objectKey: "private/customer/path" },
        status: "partial",
      }],
    ] as const) {
      await expect(harness.pool.query(`
        INSERT INTO private_activity_events (
          id, event_name, actor, subject_type, subject_id, details, recorded_at
        ) VALUES ($1, 'private.retention_run.partial', 'system:retention',
          'retention_run', $2, $3::jsonb, $4)
      `, [eventId, id, JSON.stringify(details), baseTime])).rejects.toMatchObject({ code: "23514" });
    }

    const activity = new DrizzlePrivateActivityLedgerRepository(harness.database);
    expect((await activity.recent(10)).map((event) => event.eventName)).toEqual([
      "private.retention_run.started",
    ]);
    await expect(harness.pool.query(
      "UPDATE private_activity_events SET recorded_at = recorded_at + interval '1 second' WHERE subject_id = $1",
      [id],
    )).rejects.toMatchObject({ code: "55000" });
    await expect(harness.pool.query(
      "DELETE FROM private_activity_events WHERE subject_id = $1",
      [id],
    )).rejects.toMatchObject({ code: "55000" });
  });

  test("uses exact bounded cleanup counts and leaves the oldest remainder visible", async () => {
    await seedBacklog();
    const probe = new DrizzleRetentionBacklogProbe(harness.database);
    expect(await probe.oldestDueAt(baseTime)).toEqual(
      new Date("2026-07-26T15:00:00.000Z"),
    );
    await harness.database.delete(artifacts);

    const replay = new DrizzleInboundReplayRepository(
      harness.database as unknown as ReturnType<typeof getDatabase>,
    );
    expect(await replay.cleanup(baseTime)).toBe(1);
    expect(await cleanupConsoleAuthorizations(baseTime, harness.database)).toBe(1);
    expect(await cleanupConsoleSessions(baseTime, harness.database)).toBe(1);
    expect(await cleanupConsoleDeviceFamilies(baseTime, harness.database)).toBe(1);
    expect(await cleanupConsoleRateLimits(baseTime, harness.database)).toBe(consoleCleanupBatchSize);
    expect(await probe.oldestDueAt(baseTime)).toEqual(
      new Date("2026-07-26T17:50:00.000Z"),
    );
    expect(await cleanupConsoleRateLimits(baseTime, harness.database)).toBe(1);
    expect(await probe.oldestDueAt(baseTime)).toBeNull();
  });

  function retentionRepository(sequence: number): DrizzleRetentionRunRepository {
    let next = sequence;
    return new DrizzleRetentionRunRepository(harness.database, () => {
      const id = activityId(next);
      next += 1;
      return id;
    });
  }

  async function seedBacklog(): Promise<void> {
    const ownerId = "acc_retention_postgres_owner";
    const createdAt = new Date("2026-07-26T14:00:00.000Z");
    await harness.database.insert(consoleUsers).values({
      createdAt,
      email: "retention-postgres@example.invalid",
      id: ownerId,
      updatedAt: createdAt,
    });
    await harness.database.insert(artifacts).values({
      artifactId: "art_retention_postgres_due",
      contentType: "application/zstd",
      createdAt,
      kind: "chalupa.log-chunk",
      ownerAccountId: ownerId,
      planExpiresAt: new Date("2026-07-26T15:00:00.000Z"),
      planToken: "retention-postgres-plan-token",
      producer: { tool: "chalupa" },
      sha256: "a".repeat(64),
      sizeBytes: 32,
      state: "planned",
      verification: "server-sha256",
    });
    await harness.database.insert(inboundEmailReplays).values({
      attempts: 1,
      createdAt,
      emailIdSha256: "b".repeat(64),
      expiresAt: new Date("2026-07-26T16:04:00.000Z"),
      id: "replay_retention_due",
      status: "ignored",
      svixIdSha256: "c".repeat(64),
      updatedAt: createdAt,
    });
    await harness.database.insert(consoleAuthorizations).values({
      clientName: "retention-test",
      clientType: "cli",
      createdAt,
      deviceCodeHash: "d".repeat(64),
      emailSendCount: 0,
      expiresAt: new Date("2026-07-26T16:01:00.000Z"),
      id: "auth_retention_due",
      otpAttempts: 0,
      status: "pending",
      updatedAt: createdAt,
      userCode: "RTTN-DUE1",
    });
    await harness.database.insert(consoleDeviceFamilies).values({
      absoluteExpiresAt: new Date("2026-07-26T19:00:00.000Z"),
      clientName: "retention-test",
      createdAt,
      id: "family_retention_due",
      idleExpiresAt: new Date("2026-07-26T16:02:00.000Z"),
      status: "active",
      updatedAt: createdAt,
      userId: ownerId,
    });
    await harness.database.insert(consoleSessions).values({
      createdAt,
      expiresAt: new Date("2026-07-26T16:03:00.000Z"),
      id: "session_retention_due",
      kind: "device",
      lastFour: "1234",
      refreshFamilyId: "family_retention_due",
      tokenHash: "e".repeat(64),
      updatedAt: createdAt,
      userId: ownerId,
    });
    const rateLimits = Array.from(
      { length: consoleCleanupBatchSize + 1 },
      (_, index) => ({
        action: "retention-test",
        count: 1,
        expiresAt: index === consoleCleanupBatchSize
          ? new Date("2026-07-26T17:50:00.000Z")
          : new Date(new Date("2026-07-26T17:00:00.000Z").getTime() + index * 1_000),
        id: `rate_retention_${String(index).padStart(3, "0")}`,
        keyHash: index.toString(16).padStart(64, "0"),
        windowStartedAt: new Date(createdAt.getTime() - 1_000),
      }),
    );
    await harness.database.insert(consoleRateLimits).values(rateLimits);
  }
});

function runId(sequence: number): string {
  return `rtn_00000000-0000-4000-8000-${String(sequence).padStart(12, "0")}`;
}

function activityId(sequence: number): string {
  return `act_00000000-0000-4000-8000-${String(sequence).padStart(12, "0")}`;
}

function successfulStage(name: RetentionStage["name"]): RetentionStage {
  return { execute: async () => ({}), name };
}

function failedStage(name: RetentionStage["name"]): RetentionStage {
  return {
    execute: async () => { throw new Error("synthetic stage failure"); },
    name,
  };
}
