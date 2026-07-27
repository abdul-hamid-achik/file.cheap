import { sql, type SQL } from "drizzle-orm";

import { getDatabase } from "@/platform/database/client";
import {
  consoleAuthorizations,
  consoleDeviceFamilies,
  consoleRateLimits,
  consoleSessions,
} from "@/platform/database/schema";

export interface ConsoleCleanupDatabase {
  execute(query: SQL): PromiseLike<{ rows: unknown[] }>;
}
export const consoleCleanupBatchSize = 100;

export async function cleanupConsoleAuthorizations(
  now: Date,
  db: ConsoleCleanupDatabase = getDatabase(),
): Promise<number> {
  const result = await db.execute(sql`
    DELETE FROM ${consoleAuthorizations}
    WHERE id IN (
      SELECT candidate.id
      FROM ${consoleAuthorizations} candidate
      WHERE candidate.expires_at <= ${now.toISOString()}::timestamptz
      ORDER BY candidate.expires_at ASC, candidate.id ASC
      LIMIT ${consoleCleanupBatchSize}
    )
    RETURNING id
  `);
  return result.rows.length;
}

export async function cleanupConsoleDeviceFamilies(
  now: Date,
  db: ConsoleCleanupDatabase = getDatabase(),
): Promise<number> {
  const result = await db.execute(sql`
    DELETE FROM ${consoleDeviceFamilies}
    WHERE id IN (
      SELECT candidate.id
      FROM ${consoleDeviceFamilies} candidate
      WHERE candidate.status = 'revoked'
        OR candidate.idle_expires_at <= ${now.toISOString()}::timestamptz
        OR candidate.absolute_expires_at <= ${now.toISOString()}::timestamptz
      ORDER BY CASE WHEN candidate.status = 'revoked'
        THEN candidate.revoked_at
        ELSE least(candidate.idle_expires_at, candidate.absolute_expires_at)
      END ASC, candidate.id ASC
      LIMIT ${consoleCleanupBatchSize}
    )
    RETURNING id
  `);
  return result.rows.length;
}

export async function cleanupConsoleSessions(
  now: Date,
  db: ConsoleCleanupDatabase = getDatabase(),
): Promise<number> {
  const result = await db.execute(sql`
    DELETE FROM ${consoleSessions}
    WHERE id IN (
      SELECT candidate.id
      FROM ${consoleSessions} candidate
      WHERE candidate.expires_at <= ${now.toISOString()}::timestamptz
        OR candidate.revoked_at <= ${now.toISOString()}::timestamptz
      ORDER BY CASE
        WHEN candidate.revoked_at is not null
          AND candidate.revoked_at <= candidate.expires_at
          THEN candidate.revoked_at
        ELSE candidate.expires_at
      END ASC, candidate.id ASC
      LIMIT ${consoleCleanupBatchSize}
    )
    RETURNING id
  `);
  return result.rows.length;
}

export async function cleanupConsoleRateLimits(
  now: Date,
  db: ConsoleCleanupDatabase = getDatabase(),
): Promise<number> {
  const result = await db.execute(sql`
    DELETE FROM ${consoleRateLimits}
    WHERE id IN (
      SELECT candidate.id
      FROM ${consoleRateLimits} candidate
      WHERE candidate.expires_at <= ${now.toISOString()}::timestamptz
      ORDER BY candidate.expires_at ASC, candidate.id ASC
      LIMIT ${consoleCleanupBatchSize}
    )
    RETURNING id
  `);
  return result.rows.length;
}

export type ConsoleCleanupReport = Readonly<{
  authorizations: number;
  deviceFamilies: number;
  rateLimits: number;
  sessions: number;
}>;

/** Compatibility wrapper for callers that need the complete cleanup family. */
export async function cleanupConsoleState(
  now: Date,
  db: ConsoleCleanupDatabase = getDatabase(),
): Promise<ConsoleCleanupReport> {
  // Sessions are counted before a family cascade can remove them.
  const authorizations = await cleanupConsoleAuthorizations(now, db);
  const sessions = await cleanupConsoleSessions(now, db);
  const deviceFamilies = await cleanupConsoleDeviceFamilies(now, db);
  const rateLimits = await cleanupConsoleRateLimits(now, db);
  return { authorizations, deviceFamilies, rateLimits, sessions };
}
