import { eq, lte, or } from "drizzle-orm";

import { getDatabase } from "@/platform/database/client";
import {
  consoleAuthorizations,
  consoleDeviceFamilies,
  consoleRateLimits,
  consoleSessions,
} from "@/platform/database/schema";

export async function cleanupConsoleState(now: Date): Promise<void> {
  const db = getDatabase();
  await db.delete(consoleAuthorizations).where(lte(consoleAuthorizations.expiresAt, now));
  await db.delete(consoleDeviceFamilies).where(or(
    eq(consoleDeviceFamilies.status, "revoked"),
    lte(consoleDeviceFamilies.idleExpiresAt, now),
    lte(consoleDeviceFamilies.absoluteExpiresAt, now),
  ));
  await db.delete(consoleSessions).where(or(
    lte(consoleSessions.expiresAt, now),
    lte(consoleSessions.revokedAt, now),
  ));
  await db.delete(consoleRateLimits).where(lte(consoleRateLimits.expiresAt, now));
}
