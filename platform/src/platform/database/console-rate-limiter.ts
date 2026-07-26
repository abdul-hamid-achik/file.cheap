import { createHmac } from "node:crypto";

import { lt, sql } from "drizzle-orm";

import { getDatabase } from "@/platform/database/client";
import { consoleRateLimits } from "@/platform/database/schema";
import { getAuthConfig } from "@/shared/config/auth";
import { PlatformError } from "@/shared/errors/platform-error";

export async function enforceConsoleRateLimit(input: {
  action: string;
  includeAddress?: boolean;
  key: string;
  limit: number;
  request: Request;
  windowSeconds: number;
}): Promise<void> {
  const now = new Date();
  const windowMs = input.windowSeconds * 1_000;
  const windowStartedAt = new Date(Math.floor(now.getTime() / windowMs) * windowMs);
  const rawKey = input.includeAddress === false
    ? input.key
    : `${requestAddress(input.request)}\n${input.key}`;
  const keyHash = createHmac("sha256", getAuthConfig().secret)
    .update(`rate-limit\n${input.action}\n${rawKey}`)
    .digest("hex");
  const id = createHmac("sha256", getAuthConfig().secret)
    .update(`${input.action}\n${keyHash}\n${windowStartedAt.toISOString()}`)
    .digest("hex");
  const rows = await getDatabase().insert(consoleRateLimits).values({
    action: input.action,
    count: 1,
    expiresAt: new Date(windowStartedAt.getTime() + windowMs * 2),
    id,
    keyHash,
    windowStartedAt,
  }).onConflictDoUpdate({
    set: { count: sql`${consoleRateLimits.count} + 1` },
    setWhere: lt(consoleRateLimits.count, input.limit),
    target: consoleRateLimits.id,
  }).returning({ count: consoleRateLimits.count });
  if (rows.length === 0 || rows[0]!.count > input.limit) {
    throw new PlatformError({
      code: "rate_limited",
      detail: "Too many console authentication attempts. Retry after the current window.",
      retryAfterSeconds: Math.max(1, Math.ceil((windowStartedAt.getTime() + windowMs - now.getTime()) / 1_000)),
      status: 429,
      title: "Too many requests",
    });
  }
}

function requestAddress(request: Request): string {
  const forwarded = (process.env.VERCEL
    ? request.headers.get("x-vercel-forwarded-for")
    : request.headers.get("x-forwarded-for"))?.split(",", 1)[0]?.trim();
  return forwarded || request.headers.get("x-real-ip") || "unknown";
}
