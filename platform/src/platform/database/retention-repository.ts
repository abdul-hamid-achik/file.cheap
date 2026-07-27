import { randomUUID } from "node:crypto";

import { sql, type SQL } from "drizzle-orm";

import {
  emptyRetentionCounters,
  retentionCountersSchema,
  retentionFailureAreaSchema,
  retentionRunStatusSchema,
  type RetentionHealth,
  type RetentionRun,
} from "@/features/retention/contracts";
import type { RetentionRunRepository } from "@/features/retention/repository";
import { getDatabase } from "@/platform/database/client";
import {
  privateActivityEvents,
  privateRetentionRuns,
} from "@/platform/database/schema";
import { PlatformError } from "@/shared/errors/platform-error";

export interface RetentionDatabase {
  execute(query: SQL): PromiseLike<{ rows: unknown[] }>;
}
type TimestampValue = Date | string;

type RetentionRunRow = {
  artifactCandidates: unknown;
  artifactFailures: unknown;
  artifactsDeleted: unknown;
  consoleAuthorizationRecordsDeleted: unknown;
  consoleDeviceFamilyRecordsDeleted: unknown;
  consoleRateLimitRecordsDeleted: unknown;
  consoleSessionRecordsDeleted: unknown;
  failedAreas: unknown;
  finishedAt: TimestampValue | null;
  heartbeatAt: TimestampValue;
  id: string;
  inboundReplayRecordsDeleted: unknown;
  oldestDueAt: TimestampValue | null;
  stagesAttempted: unknown;
  stagesFailed: unknown;
  stagesSucceeded: unknown;
  startedAt: TimestampValue;
  status: string;
};

type RetentionHealthRow = Partial<RetentionRunRow> & {
  activeRunCount: unknown;
};

export class DrizzleRetentionRunRepository implements RetentionRunRepository {
  constructor(
    private readonly db: RetentionDatabase = getDatabase(),
    private readonly newActivityId: () => string = () => `act_${randomUUID()}`,
  ) {}

  async abandonStale(input: {
    abandonedAt: Date;
    staleBefore: Date;
  }): Promise<RetentionRun[]> {
    const eventId = this.newActivityId();
    const result = await this.db.execute(sql`
      WITH abandoned AS (
        UPDATE ${privateRetentionRuns}
        SET status = 'abandoned',
          heartbeat_at = ${input.abandonedAt.toISOString()}::timestamptz,
          finished_at = ${input.abandonedAt.toISOString()}::timestamptz,
          failed_areas = ARRAY['run_lease']::text[]
        WHERE status = 'running'
          AND heartbeat_at <= ${input.staleBefore.toISOString()}::timestamptz
        RETURNING *
      ), appended_event AS (
        INSERT INTO ${privateActivityEvents} (
          id, event_name, actor, subject_type, subject_id, details, recorded_at
        )
        SELECT
          ${eventId}, 'private.retention_run.abandoned', 'system:retention',
          'retention_run', abandoned.id,
          jsonb_build_object(
            'counters', jsonb_build_object(
              'artifactCandidates', abandoned.artifact_candidates,
              'artifactFailures', abandoned.artifact_failures,
              'artifactsDeleted', abandoned.artifacts_deleted,
              'consoleAuthorizationRecordsDeleted', abandoned.console_authorization_records_deleted,
              'consoleDeviceFamilyRecordsDeleted', abandoned.console_device_family_records_deleted,
              'consoleRateLimitRecordsDeleted', abandoned.console_rate_limit_records_deleted,
              'consoleSessionRecordsDeleted', abandoned.console_session_records_deleted,
              'inboundReplayRecordsDeleted', abandoned.inbound_replay_records_deleted,
              'stagesAttempted', abandoned.stages_attempted,
              'stagesFailed', abandoned.stages_failed,
              'stagesSucceeded', abandoned.stages_succeeded
            ),
            'failedAreas', to_jsonb(abandoned.failed_areas),
            'oldestDueAt', to_jsonb(abandoned.oldest_due_at),
            'status', abandoned.status
          ),
          ${input.abandonedAt.toISOString()}::timestamptz
        FROM abandoned
        RETURNING subject_id
      )
      SELECT ${runProjection("abandoned")}
      FROM abandoned
      INNER JOIN appended_event ON appended_event.subject_id = abandoned.id
    `);
    return (result.rows as RetentionRunRow[]).map(mapRun);
  }

  async finish(input: {
    counters: RetentionRun["counters"];
    failedAreas: RetentionRun["failedAreas"];
    finishedAt: Date;
    id: string;
    oldestDueAt: Date | null;
    status: Exclude<RetentionRun["status"], "running">;
  }): Promise<RetentionRun> {
    const counters = retentionCountersSchema.parse(input.counters);
    const failedAreas = retentionFailureAreaSchema.array().max(8).parse(input.failedAreas);
    const failedAreasJson = JSON.stringify(failedAreas);
    const status = retentionRunStatusSchema.exclude(["running"]).parse(input.status);
    const eventName = `private.retention_run.${status}`;
    const details = JSON.stringify({
      counters,
      failedAreas,
      oldestDueAt: input.oldestDueAt?.toISOString() ?? null,
      status,
    });
    const result = await this.db.execute(sql`
      WITH finished AS (
        UPDATE ${privateRetentionRuns}
        SET status = ${status},
          heartbeat_at = ${input.finishedAt.toISOString()}::timestamptz,
          finished_at = ${input.finishedAt.toISOString()}::timestamptz,
          oldest_due_at = ${input.oldestDueAt?.toISOString() ?? null}::timestamptz,
          failed_areas = ARRAY(
            SELECT jsonb_array_elements_text(${failedAreasJson}::jsonb)
          ),
          artifact_candidates = ${counters.artifactCandidates},
          artifact_failures = ${counters.artifactFailures},
          artifacts_deleted = ${counters.artifactsDeleted},
          inbound_replay_records_deleted = ${counters.inboundReplayRecordsDeleted},
          console_authorization_records_deleted = ${counters.consoleAuthorizationRecordsDeleted},
          console_device_family_records_deleted = ${counters.consoleDeviceFamilyRecordsDeleted},
          console_session_records_deleted = ${counters.consoleSessionRecordsDeleted},
          console_rate_limit_records_deleted = ${counters.consoleRateLimitRecordsDeleted},
          stages_attempted = ${counters.stagesAttempted},
          stages_succeeded = ${counters.stagesSucceeded},
          stages_failed = ${counters.stagesFailed}
        WHERE id = ${input.id} AND status = 'running'
        RETURNING *
      ), appended_event AS (
        INSERT INTO ${privateActivityEvents} (
          id, event_name, actor, subject_type, subject_id, details, recorded_at
        )
        SELECT ${this.newActivityId()}, ${eventName}, 'system:retention',
          'retention_run', finished.id, ${details}::jsonb,
          ${input.finishedAt.toISOString()}::timestamptz
        FROM finished
        RETURNING subject_id
      )
      SELECT ${runProjection("finished")}
      FROM finished
      INNER JOIN appended_event ON appended_event.subject_id = finished.id
    `);
    const row = (result.rows as RetentionRunRow[])[0];
    if (!row) throw leaseLost();
    return mapRun(row);
  }

  async health(oldestDueAt: Date | null): Promise<RetentionHealth> {
    const result = await this.db.execute(sql`
      WITH active AS (
        SELECT count(*)::integer AS active_run_count
        FROM ${privateRetentionRuns}
        WHERE status = 'running'
      ), latest AS (
        SELECT *
        FROM ${privateRetentionRuns}
        ORDER BY (status = 'running') DESC, started_at DESC, id DESC
        LIMIT 1
      )
      SELECT active.active_run_count AS "activeRunCount",
        ${runProjection("latest")}
      FROM active
      LEFT JOIN latest ON true
    `);
    const row = (result.rows as RetentionHealthRow[])[0];
    if (!row) throw new Error("Retention health query did not return a snapshot");
    if (!row.id) {
      return {
        activeRunCount: integerValue(row.activeRunCount, "active retention run count"),
        counters: emptyRetentionCounters(),
        lastFinishedAt: null,
        lastRunId: null,
        lastStartedAt: null,
        oldestDueAt: oldestDueAt ? new Date(oldestDueAt) : null,
        status: null,
      };
    }
    const latest = mapRun(row as RetentionRunRow);
    return {
      activeRunCount: integerValue(row.activeRunCount, "active retention run count"),
      counters: latest.counters,
      lastFinishedAt: latest.finishedAt,
      lastRunId: latest.id,
      lastStartedAt: latest.startedAt,
      oldestDueAt: oldestDueAt ? new Date(oldestDueAt) : null,
      status: latest.status,
    };
  }

  async heartbeat(id: string, at: Date): Promise<boolean> {
    const result = await this.db.execute(sql`
      UPDATE ${privateRetentionRuns}
      SET heartbeat_at = ${at.toISOString()}::timestamptz
      WHERE id = ${id}
        AND status = 'running'
        AND heartbeat_at <= ${at.toISOString()}::timestamptz
      RETURNING id
    `);
    return result.rows.length === 1;
  }

  async start(input: { id: string; startedAt: Date }): Promise<RetentionRun> {
    const result = await this.db.execute(sql`
      WITH started AS (
        INSERT INTO ${privateRetentionRuns} (id, status, started_at, heartbeat_at)
        VALUES (
          ${input.id}, 'running', ${input.startedAt.toISOString()}::timestamptz,
          ${input.startedAt.toISOString()}::timestamptz
        )
        ON CONFLICT DO NOTHING
        RETURNING *
      ), appended_event AS (
        INSERT INTO ${privateActivityEvents} (
          id, event_name, actor, subject_type, subject_id, details, recorded_at
        )
        SELECT ${this.newActivityId()}, 'private.retention_run.started',
          'system:retention', 'retention_run', started.id, '{}'::jsonb,
          ${input.startedAt.toISOString()}::timestamptz
        FROM started
        RETURNING subject_id
      )
      SELECT ${runProjection("started")}
      FROM started
      INNER JOIN appended_event ON appended_event.subject_id = started.id
    `);
    const row = (result.rows as RetentionRunRow[])[0];
    if (!row) {
      throw new PlatformError({
        code: "retention_run_active",
        detail: "Another private retention run still owns the active lease.",
        retryAfterSeconds: 60,
        status: 409,
        title: "Retention run active",
      });
    }
    return mapRun(row);
  }
}

function runProjection(alias: string) {
  return sql.raw(`
    ${alias}.id AS "id",
    ${alias}.status AS "status",
    ${alias}.started_at AS "startedAt",
    ${alias}.heartbeat_at AS "heartbeatAt",
    ${alias}.finished_at AS "finishedAt",
    ${alias}.oldest_due_at AS "oldestDueAt",
    ${alias}.failed_areas AS "failedAreas",
    ${alias}.artifact_candidates AS "artifactCandidates",
    ${alias}.artifact_failures AS "artifactFailures",
    ${alias}.artifacts_deleted AS "artifactsDeleted",
    ${alias}.inbound_replay_records_deleted AS "inboundReplayRecordsDeleted",
    ${alias}.console_authorization_records_deleted AS "consoleAuthorizationRecordsDeleted",
    ${alias}.console_device_family_records_deleted AS "consoleDeviceFamilyRecordsDeleted",
    ${alias}.console_session_records_deleted AS "consoleSessionRecordsDeleted",
    ${alias}.console_rate_limit_records_deleted AS "consoleRateLimitRecordsDeleted",
    ${alias}.stages_attempted AS "stagesAttempted",
    ${alias}.stages_succeeded AS "stagesSucceeded",
    ${alias}.stages_failed AS "stagesFailed"
  `);
}

function mapRun(row: RetentionRunRow): RetentionRun {
  return {
    counters: retentionCountersSchema.parse({
      artifactCandidates: integerValue(row.artifactCandidates, "artifact candidate count"),
      artifactFailures: integerValue(row.artifactFailures, "artifact failure count"),
      artifactsDeleted: integerValue(row.artifactsDeleted, "artifact deletion count"),
      consoleAuthorizationRecordsDeleted: integerValue(row.consoleAuthorizationRecordsDeleted, "authorization cleanup count"),
      consoleDeviceFamilyRecordsDeleted: integerValue(row.consoleDeviceFamilyRecordsDeleted, "device family cleanup count"),
      consoleRateLimitRecordsDeleted: integerValue(row.consoleRateLimitRecordsDeleted, "rate limit cleanup count"),
      consoleSessionRecordsDeleted: integerValue(row.consoleSessionRecordsDeleted, "session cleanup count"),
      inboundReplayRecordsDeleted: integerValue(row.inboundReplayRecordsDeleted, "inbound replay cleanup count"),
      stagesAttempted: integerValue(row.stagesAttempted, "attempted retention stage count"),
      stagesFailed: integerValue(row.stagesFailed, "failed retention stage count"),
      stagesSucceeded: integerValue(row.stagesSucceeded, "successful retention stage count"),
    }),
    failedAreas: retentionFailureAreaSchema.array().max(8).parse(row.failedAreas),
    finishedAt: row.finishedAt ? timestampValue(row.finishedAt, "retention finish time") : null,
    heartbeatAt: timestampValue(row.heartbeatAt, "retention heartbeat time"),
    id: row.id,
    oldestDueAt: row.oldestDueAt ? timestampValue(row.oldestDueAt, "oldest retention due time") : null,
    startedAt: timestampValue(row.startedAt, "retention start time"),
    status: retentionRunStatusSchema.parse(row.status),
  };
}

function integerValue(value: unknown, label: string): number {
  const number = typeof value === "number" ? value : Number(value);
  if (!Number.isSafeInteger(number) || number < 0) throw new Error(`Invalid ${label}`);
  return number;
}

function timestampValue(value: TimestampValue, label: string): Date {
  const timestamp = value instanceof Date ? new Date(value) : new Date(value);
  if (Number.isNaN(timestamp.getTime())) throw new Error(`Invalid ${label}`);
  return timestamp;
}

function leaseLost(): PlatformError {
  return new PlatformError({
    code: "retention_lease_lost",
    detail: "The retention worker no longer owns an active database lease.",
    retryAfterSeconds: 60,
    status: 503,
    title: "Retention lease lost",
  });
}
