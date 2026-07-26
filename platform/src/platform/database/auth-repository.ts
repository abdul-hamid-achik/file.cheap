import { randomUUID } from "node:crypto";

import { and, eq, gt, inArray, isNull, lt, sql } from "drizzle-orm";

import type {
  AuthRepository,
  AuthorizationRecord,
  DeviceFamilyIssueInput,
  RefreshRotationInput,
  RefreshRotationResult,
  UserRecord,
} from "@/features/auth/repository";
import { getDatabase } from "@/platform/database/client";
import {
  consoleAuthorizations,
  consoleDeviceFamilies,
  consoleRefreshTokens,
  consoleSessions,
  consoleUsers,
} from "@/platform/database/schema";

export class DrizzleAuthRepository implements AuthRepository {
  private readonly db = getDatabase();

  async createAuthorization(record: AuthorizationRecord): Promise<void> {
    await this.db.insert(consoleAuthorizations).values(record);
  }

  async findAuthorizationByDeviceCodeHash(deviceCodeHash: string) {
    const row = (await this.db.select().from(consoleAuthorizations)
      .where(eq(consoleAuthorizations.deviceCodeHash, deviceCodeHash)).limit(1))[0];
    return row ? mapAuthorization(row) : null;
  }

  async findAuthorizationByUserCode(userCode: string) {
    const row = (await this.db.select().from(consoleAuthorizations)
      .where(eq(consoleAuthorizations.userCode, userCode)).limit(1))[0];
    return row ? mapAuthorization(row) : null;
  }

  async markEmailSent(input: { email: string; id: string; now: Date; otpHash: string }) {
    const row = (await this.db.update(consoleAuthorizations).set({
      email: input.email,
      emailSendCount: sql`${consoleAuthorizations.emailSendCount} + 1`,
      otpHash: input.otpHash,
      status: "email_sent",
      updatedAt: input.now,
    }).where(and(
      eq(consoleAuthorizations.id, input.id),
      inArray(consoleAuthorizations.status, ["pending", "email_sent"]),
      lt(consoleAuthorizations.emailSendCount, 3),
    )).returning())[0];
    return row ? mapAuthorization(row) : null;
  }

  async recordOtpFailure(id: string, now: Date): Promise<void> {
    await this.db.update(consoleAuthorizations).set({
      otpAttempts: sql`${consoleAuthorizations.otpAttempts} + 1`,
      updatedAt: now,
    }).where(and(
      eq(consoleAuthorizations.id, id),
      lt(consoleAuthorizations.otpAttempts, 8),
    ));
  }

  async approve(input: { email: string; id: string; now: Date; otpHash: string; userId: string }) {
    const row = (await this.db.update(consoleAuthorizations).set({
      approvedUserId: input.userId,
      status: "approved",
      updatedAt: input.now,
    }).where(and(
      eq(consoleAuthorizations.id, input.id),
      eq(consoleAuthorizations.status, "email_sent"),
      eq(consoleAuthorizations.email, input.email),
      eq(consoleAuthorizations.otpHash, input.otpHash),
      lt(consoleAuthorizations.otpAttempts, 8),
      gt(consoleAuthorizations.expiresAt, input.now),
    )).returning())[0];
    return row ? mapAuthorization(row) : null;
  }

  async deny(input: { email: string; id: string; now: Date; otpHash: string }): Promise<boolean> {
    const rows = await this.db.update(consoleAuthorizations).set({ status: "denied", updatedAt: input.now })
      .where(and(
        eq(consoleAuthorizations.id, input.id),
        eq(consoleAuthorizations.status, "email_sent"),
        eq(consoleAuthorizations.email, input.email),
        eq(consoleAuthorizations.otpHash, input.otpHash),
        lt(consoleAuthorizations.otpAttempts, 8),
        gt(consoleAuthorizations.expiresAt, input.now),
      )).returning({ id: consoleAuthorizations.id });
    return rows.length === 1;
  }

  async consumeBrowser(id: string, now: Date, userId: string): Promise<boolean> {
    const rows = await this.db.update(consoleAuthorizations).set({ status: "consumed", updatedAt: now })
      .where(and(
        eq(consoleAuthorizations.id, id),
        eq(consoleAuthorizations.clientType, "browser"),
        eq(consoleAuthorizations.status, "approved"),
        eq(consoleAuthorizations.approvedUserId, userId),
        gt(consoleAuthorizations.expiresAt, now),
      )).returning({ id: consoleAuthorizations.id });
    return rows.length === 1;
  }

  async consumeDeviceAuthorization(input: DeviceFamilyIssueInput): Promise<boolean> {
    const result = await this.db.execute(sql`
      WITH claimed AS (
        UPDATE ${consoleAuthorizations}
        SET status = 'consumed', updated_at = ${input.now.toISOString()}::timestamptz
        WHERE id = ${input.authorizationId}
          AND status = 'approved'
          AND approved_user_id = ${input.userId}
          AND client_type <> 'browser'
          AND expires_at > ${input.now.toISOString()}::timestamptz
        RETURNING approved_user_id
      ), inserted_family AS (
        INSERT INTO ${consoleDeviceFamilies} (
          id, user_id, client_name, status, absolute_expires_at,
          idle_expires_at, last_used_at, created_at, updated_at
        )
        SELECT ${input.family.id}, approved_user_id, ${input.family.clientName},
          'active', ${input.family.absoluteExpiresAt.toISOString()}::timestamptz,
          ${input.family.idleExpiresAt.toISOString()}::timestamptz,
          ${input.now.toISOString()}::timestamptz,
          ${input.now.toISOString()}::timestamptz,
          ${input.now.toISOString()}::timestamptz
        FROM claimed
        RETURNING id, user_id
      ), inserted_refresh AS (
        INSERT INTO ${consoleRefreshTokens} (
          id, family_id, generation, token_hash, last_four, expires_at,
          created_at, updated_at
        )
        SELECT ${input.refresh.id}, id, 0, ${input.refresh.tokenHash},
          ${input.refresh.lastFour}, ${input.refresh.expiresAt.toISOString()}::timestamptz,
          ${input.now.toISOString()}::timestamptz,
          ${input.now.toISOString()}::timestamptz
        FROM inserted_family
        RETURNING family_id
      )
      INSERT INTO ${consoleSessions} (
        id, user_id, token_hash, kind, refresh_family_id, last_four,
        expires_at, created_at, updated_at
      )
      SELECT ${input.access.id}, inserted_family.user_id, ${input.access.tokenHash},
        'device', inserted_family.id, ${input.access.lastFour},
        ${input.access.expiresAt.toISOString()}::timestamptz,
        ${input.now.toISOString()}::timestamptz, ${input.now.toISOString()}::timestamptz
      FROM inserted_family
      INNER JOIN inserted_refresh ON inserted_refresh.family_id = inserted_family.id
      RETURNING id
    `);
    return result.rows.length === 1;
  }

  async rotateDeviceFamily(input: RefreshRotationInput): Promise<RefreshRotationResult> {
    const rotated = await this.db.execute(sql`
      WITH current_token AS (
        SELECT token.id, token.family_id, token.generation,
          family.user_id, family.absolute_expires_at
        FROM ${consoleRefreshTokens} token
        INNER JOIN ${consoleDeviceFamilies} family ON family.id = token.family_id
        WHERE token.token_hash = ${input.refreshTokenHash}
          AND token.used_at IS NULL
          AND token.expires_at > ${input.now.toISOString()}::timestamptz
          AND family.status = 'active'
          AND family.revoked_at IS NULL
          AND family.idle_expires_at > ${input.now.toISOString()}::timestamptz
          AND family.absolute_expires_at > ${input.now.toISOString()}::timestamptz
      ), claimed AS (
        UPDATE ${consoleRefreshTokens} token
        SET used_at = ${input.now.toISOString()}::timestamptz,
          rotation_id = ${input.rotationId},
          replaced_by_token_hash = ${input.nextRefresh.tokenHash},
          updated_at = ${input.now.toISOString()}::timestamptz
        FROM current_token
        WHERE token.id = current_token.id AND token.used_at IS NULL
        RETURNING token.family_id, token.generation
      ), updated_family AS (
        UPDATE ${consoleDeviceFamilies} family
        SET idle_expires_at = LEAST(
            ${input.nextRefresh.expiresAt.toISOString()}::timestamptz,
            family.absolute_expires_at
          ),
          last_used_at = ${input.now.toISOString()}::timestamptz,
          updated_at = ${input.now.toISOString()}::timestamptz
        FROM claimed
        WHERE family.id = claimed.family_id
        RETURNING family.id, family.user_id, family.absolute_expires_at,
          family.idle_expires_at, claimed.generation
      ), inserted_refresh AS (
        INSERT INTO ${consoleRefreshTokens} (
          id, family_id, generation, token_hash, last_four, expires_at,
          created_at, updated_at
        )
        SELECT ${input.nextRefresh.id}, id, generation + 1,
          ${input.nextRefresh.tokenHash}, ${input.nextRefresh.lastFour},
          idle_expires_at, ${input.now.toISOString()}::timestamptz,
          ${input.now.toISOString()}::timestamptz
        FROM updated_family
        RETURNING family_id, expires_at
      ), inserted_access AS (
        INSERT INTO ${consoleSessions} (
          id, user_id, token_hash, kind, refresh_family_id, last_four,
          expires_at, created_at, updated_at
        )
        SELECT ${input.access.id}, updated_family.user_id,
          ${input.access.tokenHash}, 'device', updated_family.id,
          ${input.access.lastFour}, LEAST(
            ${input.access.expiresAt.toISOString()}::timestamptz,
            updated_family.absolute_expires_at
          ), ${input.now.toISOString()}::timestamptz,
          ${input.now.toISOString()}::timestamptz
        FROM updated_family
        INNER JOIN inserted_refresh ON inserted_refresh.family_id = updated_family.id
        RETURNING expires_at, refresh_family_id
      )
      SELECT EXTRACT(EPOCH FROM inserted_access.expires_at) * 1000 AS access_expires_ms,
        EXTRACT(EPOCH FROM inserted_refresh.expires_at) * 1000 AS refresh_expires_ms
      FROM inserted_access
      INNER JOIN inserted_refresh ON inserted_refresh.family_id = inserted_access.refresh_family_id
    `);
    const rotatedRow = rotated.rows[0] as Record<string, unknown> | undefined;
    if (rotatedRow) return rotationResult(rotatedRow);

    // A retried request with the same rotation id and replacement token is
    // idempotent. It receives a fresh short-lived access token without creating
    // another refresh generation.
    const replayed = await this.db.execute(sql`
      WITH matching AS (
        SELECT token.family_id, replacement.expires_at AS refresh_expires_at,
          family.user_id, family.absolute_expires_at
        FROM ${consoleRefreshTokens} token
        INNER JOIN ${consoleDeviceFamilies} family ON family.id = token.family_id
        INNER JOIN ${consoleRefreshTokens} replacement
          ON replacement.family_id = token.family_id
          AND replacement.token_hash = token.replaced_by_token_hash
        WHERE token.token_hash = ${input.refreshTokenHash}
          AND token.used_at IS NOT NULL
          AND token.rotation_id = ${input.rotationId}
          AND token.replaced_by_token_hash = ${input.nextRefresh.tokenHash}
          AND replacement.token_hash = ${input.nextRefresh.tokenHash}
          AND replacement.used_at IS NULL
          AND replacement.expires_at > ${input.now.toISOString()}::timestamptz
          AND family.status = 'active'
          AND family.revoked_at IS NULL
          AND family.idle_expires_at > ${input.now.toISOString()}::timestamptz
          AND family.absolute_expires_at > ${input.now.toISOString()}::timestamptz
      ), inserted_access AS (
        INSERT INTO ${consoleSessions} (
          id, user_id, token_hash, kind, refresh_family_id, last_four,
          expires_at, created_at, updated_at
        )
        SELECT ${input.access.id}, user_id, ${input.access.tokenHash}, 'device',
          family_id, ${input.access.lastFour}, LEAST(
            ${input.access.expiresAt.toISOString()}::timestamptz,
            absolute_expires_at
          ), ${input.now.toISOString()}::timestamptz,
          ${input.now.toISOString()}::timestamptz
        FROM matching
        RETURNING expires_at, refresh_family_id
      )
      SELECT EXTRACT(EPOCH FROM inserted_access.expires_at) * 1000 AS access_expires_ms,
        EXTRACT(EPOCH FROM matching.refresh_expires_at) * 1000 AS refresh_expires_ms
      FROM inserted_access
      INNER JOIN matching ON matching.family_id = inserted_access.refresh_family_id
    `);
    const replayedRow = replayed.rows[0] as Record<string, unknown> | undefined;
    if (replayedRow) return rotationResult(replayedRow);

    const reused = await this.db.execute(sql`
      WITH reused_family AS (
        SELECT family_id
        FROM ${consoleRefreshTokens}
        WHERE token_hash = ${input.refreshTokenHash}
          AND used_at IS NOT NULL
          AND (
            rotation_id IS DISTINCT FROM ${input.rotationId}
            OR replaced_by_token_hash IS DISTINCT FROM ${input.nextRefresh.tokenHash}
          )
      ), revoked_family AS (
        UPDATE ${consoleDeviceFamilies} family
        SET status = 'revoked', revoked_at = ${input.now.toISOString()}::timestamptz,
          revoke_reason = 'refresh-reuse', updated_at = ${input.now.toISOString()}::timestamptz
        FROM reused_family
        WHERE family.id = reused_family.family_id AND family.status = 'active'
        RETURNING family.id
      ), revoked_sessions AS (
        UPDATE ${consoleSessions} session
        SET revoked_at = ${input.now.toISOString()}::timestamptz,
          updated_at = ${input.now.toISOString()}::timestamptz
        FROM revoked_family
        WHERE session.refresh_family_id = revoked_family.id
          AND session.revoked_at IS NULL
        RETURNING session.id
      )
      SELECT id FROM revoked_family
    `);
    return reused.rows.length > 0 ? "reuse" : null;
  }

  async createSession(input: { expiresAt: Date; id: string; lastFour: string; now: Date; tokenHash: string; userId: string }): Promise<void> {
    await this.db.insert(consoleSessions).values({
      createdAt: input.now,
      expiresAt: input.expiresAt,
      id: input.id,
      kind: "web",
      lastFour: input.lastFour,
      tokenHash: input.tokenHash,
      updatedAt: input.now,
      userId: input.userId,
    });
  }

  async notePoll(id: string, now: Date): Promise<void> {
    await this.db.update(consoleAuthorizations).set({ lastPolledAt: now, updatedAt: now })
      .where(eq(consoleAuthorizations.id, id));
  }

  async findActiveSession(tokenHash: string, now: Date, kind?: "web" | "device"): Promise<UserRecord | null> {
    const row = (await this.db.select({ user: consoleUsers }).from(consoleSessions)
      .innerJoin(consoleUsers, eq(consoleUsers.id, consoleSessions.userId))
      .where(and(
        eq(consoleSessions.tokenHash, tokenHash),
        ...(kind ? [eq(consoleSessions.kind, kind)] : []),
        isNull(consoleSessions.revokedAt),
        gt(consoleSessions.expiresAt, now),
      )).limit(1))[0];
    if (!row) return null;
    await this.db.update(consoleSessions).set({ lastUsedAt: now, updatedAt: now })
      .where(eq(consoleSessions.tokenHash, tokenHash));
    return row.user;
  }

  async revokeSession(tokenHash: string, now: Date): Promise<void> {
    const family = await this.db.execute(sql`
      WITH target_family AS (
        SELECT refresh_family_id AS id
        FROM ${consoleSessions}
        WHERE token_hash = ${tokenHash}
          AND kind = 'device'
          AND refresh_family_id IS NOT NULL
        LIMIT 1
      ), revoked_family AS (
        UPDATE ${consoleDeviceFamilies} family
        SET status = 'revoked', revoked_at = COALESCE(family.revoked_at, ${now.toISOString()}::timestamptz),
          revoke_reason = COALESCE(family.revoke_reason, 'logout'),
          updated_at = ${now.toISOString()}::timestamptz
        FROM target_family
        WHERE family.id = target_family.id
        RETURNING family.id
      ), revoked_sessions AS (
        UPDATE ${consoleSessions} session
        SET revoked_at = COALESCE(session.revoked_at, ${now.toISOString()}::timestamptz),
          updated_at = ${now.toISOString()}::timestamptz
        FROM revoked_family
        WHERE session.refresh_family_id = revoked_family.id
        RETURNING session.id
      )
      SELECT id FROM revoked_family
    `);
    if (family.rows.length > 0) return;
    await this.db.update(consoleSessions).set({ revokedAt: now, updatedAt: now })
      .where(and(eq(consoleSessions.tokenHash, tokenHash), isNull(consoleSessions.revokedAt)));
  }

  async upsertUser(email: string, now: Date, preferredId?: string): Promise<UserRecord> {
    const rows = await this.db.insert(consoleUsers).values({
      createdAt: now,
      email,
      id: preferredId ?? randomUUID(),
      updatedAt: now,
    }).onConflictDoUpdate({
      set: { updatedAt: now },
      target: consoleUsers.email,
    }).returning();
    const user = rows[0];
    if (!user) throw new Error("Console user upsert did not return a row");
    return user;
  }
}

function mapAuthorization(row: typeof consoleAuthorizations.$inferSelect): AuthorizationRecord {
  return {
    approvedUserId: row.approvedUserId,
    clientName: row.clientName,
    clientType: row.clientType as AuthorizationRecord["clientType"],
    createdAt: row.createdAt,
    deviceCodeHash: row.deviceCodeHash,
    email: row.email,
    emailSendCount: row.emailSendCount,
    expiresAt: row.expiresAt,
    id: row.id,
    lastPolledAt: row.lastPolledAt,
    otpAttempts: row.otpAttempts,
    otpHash: row.otpHash,
    status: row.status as AuthorizationRecord["status"],
    updatedAt: row.updatedAt,
    userCode: row.userCode,
  };
}

function rotationResult(row: Record<string, unknown>): Exclude<RefreshRotationResult, "reuse" | null> {
  const accessExpiresAt = new Date(Number(row.access_expires_ms));
  const refreshExpiresAt = new Date(Number(row.refresh_expires_ms));
  if (Number.isNaN(accessExpiresAt.getTime()) || Number.isNaN(refreshExpiresAt.getTime())) {
    throw new Error("Refresh rotation returned invalid expiry timestamps");
  }
  return { accessExpiresAt, refreshExpiresAt };
}
