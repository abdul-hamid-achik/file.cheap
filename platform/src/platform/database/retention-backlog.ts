import { sql, type SQL } from "drizzle-orm";

import { artifactDeletionLeaseMilliseconds } from "@/features/artifacts/service";
import type { RetentionBacklogProbe } from "@/features/retention/repository";
import { getDatabase } from "@/platform/database/client";
import {
  artifacts,
  consoleAuthorizations,
  consoleDeviceFamilies,
  consoleRateLimits,
  consoleSessions,
  inboundEmailReplays,
} from "@/platform/database/schema";

export class DrizzleRetentionBacklogProbe implements RetentionBacklogProbe {
  constructor(private readonly db: RetentionBacklogDatabase = getDatabase()) {}

  async oldestDueAt(now: Date): Promise<Date | null> {
    const result = await this.db.execute(sql`
      WITH due_items AS (
        SELECT CASE
          WHEN state = 'planned' THEN plan_expires_at
          WHEN state = 'committed' THEN expires_at
          WHEN state = 'deleting' THEN deleting_at + (${artifactDeletionLeaseMilliseconds} * interval '1 millisecond')
        END AS due_at
        FROM ${artifacts}
        WHERE (state = 'planned' AND plan_expires_at <= ${now.toISOString()}::timestamptz)
          OR (state = 'committed' AND expires_at <= ${now.toISOString()}::timestamptz)
          OR (state = 'deleting' AND deleting_at + (${artifactDeletionLeaseMilliseconds} * interval '1 millisecond') <= ${now.toISOString()}::timestamptz)
        UNION ALL
        SELECT expires_at FROM ${inboundEmailReplays}
        WHERE expires_at <= ${now.toISOString()}::timestamptz
        UNION ALL
        SELECT expires_at FROM ${consoleAuthorizations}
        WHERE expires_at <= ${now.toISOString()}::timestamptz
        UNION ALL
        SELECT CASE WHEN status = 'revoked' THEN revoked_at
          ELSE least(idle_expires_at, absolute_expires_at) END
        FROM ${consoleDeviceFamilies}
        WHERE status = 'revoked'
          OR idle_expires_at <= ${now.toISOString()}::timestamptz
          OR absolute_expires_at <= ${now.toISOString()}::timestamptz
        UNION ALL
        SELECT CASE
          WHEN revoked_at is not null AND revoked_at <= expires_at THEN revoked_at
          ELSE expires_at
        END
        FROM ${consoleSessions}
        WHERE expires_at <= ${now.toISOString()}::timestamptz
          OR revoked_at <= ${now.toISOString()}::timestamptz
        UNION ALL
        SELECT expires_at FROM ${consoleRateLimits}
        WHERE expires_at <= ${now.toISOString()}::timestamptz
      )
      SELECT min(due_at) AS "oldestDueAt" FROM due_items
    `);
    const value = (result.rows as Array<{ oldestDueAt: Date | string | null }>)[0]?.oldestDueAt;
    if (value === null || value === undefined) return null;
    const oldest = value instanceof Date ? new Date(value) : new Date(value);
    if (Number.isNaN(oldest.getTime()) || oldest > now) {
      throw new Error("Retention backlog query returned an invalid due time");
    }
    return oldest;
  }
}

export interface RetentionBacklogDatabase {
  execute(query: SQL): PromiseLike<{ rows: unknown[] }>;
}
