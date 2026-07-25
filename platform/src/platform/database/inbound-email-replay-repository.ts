import { and, eq, lte, sql } from "drizzle-orm";

import {
  inboundProcessingLeaseMilliseconds,
  inboundReplayRetentionMilliseconds,
  maxInboundReplayAttempts,
  newReplayId,
  type InboundReplayClaim,
  type InboundReplayRepository,
} from "@/features/mail/replay-repository";
import { getDatabase } from "@/platform/database/client";
import { inboundEmailReplays } from "@/platform/database/schema";

export class DrizzleInboundReplayRepository implements InboundReplayRepository {
  private readonly db = getDatabase();

  async claim(input: {
    emailIdSha256: string;
    now: Date;
    svixIdSha256: string;
  }): Promise<InboundReplayClaim> {
    const leaseExpiresAt = new Date(
      input.now.getTime() + inboundProcessingLeaseMilliseconds,
    );
    const expiresAt = new Date(
      input.now.getTime() + inboundReplayRetentionMilliseconds,
    );
    const leaseToken = newReplayId();
    const inserted = await this.db.insert(inboundEmailReplays).values({
      attempts: 1,
      createdAt: input.now,
      emailIdSha256: input.emailIdSha256,
      expiresAt,
      id: newReplayId(),
      leaseToken,
      processingLeaseExpiresAt: leaseExpiresAt,
      status: "processing",
      svixIdSha256: input.svixIdSha256,
      updatedAt: input.now,
    }).onConflictDoNothing().returning({ id: inboundEmailReplays.id });
    if (inserted.length === 1) return { leaseToken, state: "claimed" };

    const exhausted = await this.db.update(inboundEmailReplays).set({
      leaseToken: null,
      processingLeaseExpiresAt: null,
      status: "rejected",
      updatedAt: input.now,
    }).where(and(
      eq(inboundEmailReplays.emailIdSha256, input.emailIdSha256),
      eq(inboundEmailReplays.status, "processing"),
      lte(inboundEmailReplays.processingLeaseExpiresAt, input.now),
      sql`${inboundEmailReplays.attempts} >= ${maxInboundReplayAttempts}`,
    )).returning({ id: inboundEmailReplays.id });
    if (exhausted.length === 1) return { state: "duplicate" };

    const reclaimed = await this.db.update(inboundEmailReplays).set({
      attempts: sql`${inboundEmailReplays.attempts} + 1`,
      leaseToken,
      processingLeaseExpiresAt: leaseExpiresAt,
      updatedAt: input.now,
    }).where(and(
      eq(inboundEmailReplays.emailIdSha256, input.emailIdSha256),
      eq(inboundEmailReplays.status, "processing"),
      lte(inboundEmailReplays.processingLeaseExpiresAt, input.now),
      sql`${inboundEmailReplays.attempts} < ${maxInboundReplayAttempts}`,
    )).returning({ id: inboundEmailReplays.id });
    if (reclaimed.length === 1) return { leaseToken, state: "claimed" };

    const existing = await this.db.select({
      status: inboundEmailReplays.status,
    }).from(inboundEmailReplays).where(sql`
      ${inboundEmailReplays.emailIdSha256} = ${input.emailIdSha256}
      OR ${inboundEmailReplays.svixIdSha256} = ${input.svixIdSha256}
    `).limit(1);
    return existing[0]?.status === "processing"
      ? { state: "in_progress" }
      : { state: "duplicate" };
  }

  async markForwarded(emailIdSha256: string, leaseToken: string, now: Date): Promise<boolean> {
    return this.complete(emailIdSha256, leaseToken, "forwarded", now);
  }

  async markIgnored(emailIdSha256: string, leaseToken: string, now: Date): Promise<boolean> {
    return this.complete(emailIdSha256, leaseToken, "ignored", now);
  }

  async markAmbiguous(emailIdSha256: string, leaseToken: string, now: Date): Promise<boolean> {
    return this.complete(emailIdSha256, leaseToken, "ambiguous", now);
  }

  async markRejected(emailIdSha256: string, leaseToken: string, now: Date): Promise<boolean> {
    return this.complete(emailIdSha256, leaseToken, "rejected", now);
  }

  async release(emailIdSha256: string, leaseToken: string, now: Date): Promise<void> {
    await this.db.update(inboundEmailReplays).set({
      processingLeaseExpiresAt: now,
      updatedAt: now,
    }).where(and(
      eq(inboundEmailReplays.emailIdSha256, emailIdSha256),
      eq(inboundEmailReplays.leaseToken, leaseToken),
      eq(inboundEmailReplays.status, "processing"),
    ));
  }

  async cleanup(now: Date): Promise<void> {
    await this.db.execute(sql`
      DELETE FROM ${inboundEmailReplays}
      WHERE ${inboundEmailReplays.id} IN (
        SELECT ${inboundEmailReplays.id}
        FROM ${inboundEmailReplays}
        WHERE ${inboundEmailReplays.expiresAt} <= ${now.toISOString()}::timestamptz
        ORDER BY ${inboundEmailReplays.expiresAt} ASC
        LIMIT 100
      )
    `);
  }

  private async complete(
    emailIdSha256: string,
    leaseToken: string,
    status: "ambiguous" | "forwarded" | "ignored" | "rejected",
    now: Date,
  ): Promise<boolean> {
    const result = await this.db.update(inboundEmailReplays).set({
      forwardedAt: status === "forwarded" ? now : null,
      ambiguousAt: status === "ambiguous" ? now : null,
      leaseToken: null,
      processingLeaseExpiresAt: null,
      status,
      updatedAt: now,
    }).where(and(
      eq(inboundEmailReplays.emailIdSha256, emailIdSha256),
      eq(inboundEmailReplays.leaseToken, leaseToken),
      eq(inboundEmailReplays.status, "processing"),
    )).returning({ id: inboundEmailReplays.id });
    return result.length === 1;
  }
}
