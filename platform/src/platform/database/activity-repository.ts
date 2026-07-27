import { sql, type SQL } from "drizzle-orm";

import {
  privateActivityEventSchema,
  type PrivateActivityEvent,
} from "@/features/activity/contracts";
import type { PrivateActivityLedgerRepository } from "@/features/activity/repository";
import { getDatabase } from "@/platform/database/client";
import { privateActivityEvents } from "@/platform/database/schema";

interface ActivityDatabase {
  execute(query: SQL): PromiseLike<{ rows: unknown[] }>;
}

type ActivityRow = {
  actor: string;
  details: unknown;
  eventId: string;
  eventName: string;
  recordedAt: Date | string;
  subjectId: string;
  subjectType: string;
};

export class DrizzlePrivateActivityLedgerRepository implements PrivateActivityLedgerRepository {
  constructor(private readonly db: ActivityDatabase = getDatabase()) {}

  async append(input: PrivateActivityEvent): Promise<void> {
    const event = privateActivityEventSchema.parse(input);
    await this.db.execute(sql`
      INSERT INTO ${privateActivityEvents} (
        id, event_name, actor, subject_type, subject_id, details, recorded_at
      ) VALUES (
        ${event.eventId}, ${event.eventName}, ${event.actor},
        ${event.subject.type}, ${event.subject.id},
        ${JSON.stringify(serializeDetails(event))}::jsonb,
        ${event.recordedAt.toISOString()}::timestamptz
      )
    `);
  }

  async recent(limit: number): Promise<PrivateActivityEvent[]> {
    if (!Number.isSafeInteger(limit) || limit < 1 || limit > 100) {
      throw new Error("Activity history limits must be integers from 1 through 100");
    }
    const result = await this.db.execute(sql`
      SELECT id AS "eventId", event_name AS "eventName", actor,
        subject_type AS "subjectType", subject_id AS "subjectId", details,
        recorded_at AS "recordedAt"
      FROM ${privateActivityEvents}
      ORDER BY recorded_at DESC, id DESC
      LIMIT ${limit}
    `);
    return (result.rows as ActivityRow[]).map((row) => privateActivityEventSchema.parse({
      actor: row.actor,
      details: deserializeDetails(row.eventName, row.details),
      eventId: row.eventId,
      eventName: row.eventName,
      recordedAt: row.recordedAt instanceof Date
        ? new Date(row.recordedAt)
        : new Date(row.recordedAt),
      subject: { id: row.subjectId, type: row.subjectType },
    }));
  }
}

function serializeDetails(event: PrivateActivityEvent): unknown {
  if (event.eventName === "private.retention_run.started") return {};
  return {
    ...event.details,
    oldestDueAt: event.details.oldestDueAt?.toISOString() ?? null,
  };
}

function deserializeDetails(eventName: string, value: unknown): unknown {
  if (eventName === "private.retention_run.started") return value;
  if (!value || typeof value !== "object" || Array.isArray(value)) return value;
  const details = value as Record<string, unknown>;
  return {
    ...details,
    oldestDueAt: typeof details.oldestDueAt === "string"
      ? new Date(details.oldestDueAt)
      : null,
  };
}
