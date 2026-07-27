import { randomUUID } from "node:crypto";

import { and, eq, gt, isNull, lt, or, sql } from "drizzle-orm";

import type {
  AuthRepository,
  AuthorizationRecord,
  DeviceFamilyIssueInput,
  DeviceFamilyListInput,
  DeviceFamilyListPage,
  DeviceFamilyRecord,
  RefreshRotationInput,
  RefreshRotationResult,
  UserRecord,
  VerificationDeliveryClaim,
} from "@/features/auth/repository";
import { getDatabase } from "@/platform/database/client";
import {
  consoleAuthorizations,
  consoleDeviceFamilies,
  consoleRefreshTokens,
  consoleSessions,
  consoleUsers,
  consoleVerificationDeliveries,
} from "@/platform/database/schema";

type AuthDatabase = ReturnType<typeof getDatabase>;
type TimestampValue = Date | string;

type DeviceFamilySnapshotRow = {
  absoluteExpiresAt: TimestampValue | null;
  active: unknown;
  clientName: string | null;
  createdAt: TimestampValue | null;
  expiring: unknown;
  hasNextPage: boolean;
  id: string | null;
  idleExpiresAt: TimestampValue | null;
  inactive: unknown;
  lastUsedAt: TimestampValue | null;
  revokeReason: string | null;
  revokedAt: TimestampValue | null;
  status: string | null;
  total: unknown;
  updatedAt: TimestampValue | null;
  userId: string | null;
};

export class DrizzleAuthRepository implements AuthRepository {
  constructor(private readonly db: AuthDatabase = getDatabase()) {}

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

  async claimVerificationDelivery(input: {
    eligible: boolean;
    email: string;
    leaseExpiresAt: Date;
    leaseToken: string;
    maxEmailSends: number;
    now: Date;
    userCode: string;
  }): Promise<VerificationDeliveryClaim | null> {
    // INSERT .. ON CONFLICT performs creation, expired-lease recovery and
    // same-ordinal coalescing as one PostgreSQL statement. Concurrent callers
    // cannot both receive a live claim.
    const result = await this.db.execute(sql`
      WITH candidate AS MATERIALIZED (
        SELECT authz.id, authz.client_name,
          authz.user_code, authz.email_send_count
        FROM ${consoleAuthorizations} AS authz
        WHERE authz.user_code = ${input.userCode}
          AND authz.status IN ('pending', 'email_sent')
          AND authz.expires_at > ${input.now.toISOString()}::timestamptz
          AND authz.email_send_count < ${input.maxEmailSends}
        FOR UPDATE OF authz
      ), eligible AS MATERIALIZED (
        SELECT * FROM candidate WHERE ${input.eligible}
      ), claimed AS (
        INSERT INTO ${consoleVerificationDeliveries} AS delivery (
          authorization_id, delivery_number, email, status, lease_token,
          lease_expires_at, created_at, updated_at
        )
        SELECT eligible.id, eligible.email_send_count + 1, ${input.email},
          'sending', ${input.leaseToken},
          ${input.leaseExpiresAt.toISOString()}::timestamptz,
          ${input.now.toISOString()}::timestamptz,
          ${input.now.toISOString()}::timestamptz
        FROM eligible
        ON CONFLICT (authorization_id, delivery_number) DO UPDATE
        SET status = 'sending',
          lease_token = EXCLUDED.lease_token,
          lease_expires_at = EXCLUDED.lease_expires_at,
          updated_at = EXCLUDED.updated_at
        WHERE delivery.email = EXCLUDED.email
          AND (
            delivery.status = 'pending'
            OR (
              delivery.status = 'sending'
              AND delivery.lease_expires_at <= ${input.now.toISOString()}::timestamptz
            )
          )
        RETURNING authorization_id, delivery_number, email, lease_token
      )
      SELECT claimed.authorization_id, eligible.client_name,
        claimed.delivery_number, claimed.email, claimed.lease_token,
        eligible.user_code
      FROM claimed
      INNER JOIN eligible ON eligible.id = claimed.authorization_id
    `);
    const row = result.rows[0] as Record<string, unknown> | undefined;
    if (!row) return null;
    return {
      authorizationId: stringValue(row.authorization_id, "verification authorization ID"),
      clientName: stringValue(row.client_name, "verification client name"),
      deliveryNumber: integerValue(row.delivery_number, "verification delivery number"),
      email: stringValue(row.email, "verification email"),
      leaseToken: stringValue(row.lease_token, "verification lease token"),
      userCode: stringValue(row.user_code, "verification user code"),
    };
  }

  async acceptVerificationDelivery(input: {
    authorizationId: string;
    deliveryNumber: number;
    email: string;
    leaseToken: string;
    now: Date;
    otpHash: string;
  }): Promise<boolean> {
    // Provider acceptance activates the new proof and advances the accepted
    // send counter in the same statement that seals the delivery.
    const result = await this.db.execute(sql`
      WITH locked_authorization AS MATERIALIZED (
        SELECT authz.id
        FROM ${consoleAuthorizations} AS authz
        WHERE authz.id = ${input.authorizationId}
          AND authz.status IN ('pending', 'email_sent')
          AND authz.expires_at > ${input.now.toISOString()}::timestamptz
          AND authz.email_send_count = ${input.deliveryNumber - 1}
        FOR UPDATE OF authz
      ), locked_delivery AS MATERIALIZED (
        SELECT delivery.authorization_id
        FROM ${consoleVerificationDeliveries} AS delivery
        INNER JOIN locked_authorization
          ON locked_authorization.id = delivery.authorization_id
        WHERE delivery.delivery_number = ${input.deliveryNumber}
          AND delivery.email = ${input.email}
          AND delivery.status = 'sending'
          AND delivery.lease_token = ${input.leaseToken}
        FOR UPDATE OF delivery
      ), activated AS (
        UPDATE ${consoleAuthorizations} AS authz
        SET email = ${input.email}, otp_hash = ${input.otpHash},
          email_send_count = ${input.deliveryNumber}, status = 'email_sent',
          updated_at = ${input.now.toISOString()}::timestamptz
        FROM locked_delivery
        WHERE authz.id = locked_delivery.authorization_id
        RETURNING authz.id
      )
      UPDATE ${consoleVerificationDeliveries} AS delivery
      SET status = 'accepted', lease_token = NULL, lease_expires_at = NULL,
        accepted_at = ${input.now.toISOString()}::timestamptz,
        updated_at = ${input.now.toISOString()}::timestamptz
      FROM activated
      WHERE delivery.authorization_id = activated.id
        AND delivery.delivery_number = ${input.deliveryNumber}
        AND delivery.email = ${input.email}
        AND delivery.status = 'sending'
        AND delivery.lease_token = ${input.leaseToken}
      RETURNING delivery.authorization_id
    `);
    return result.rows.length === 1;
  }

  async releaseVerificationDelivery(input: {
    authorizationId: string;
    deliveryNumber: number;
    leaseToken: string;
    now: Date;
  }): Promise<void> {
    await this.db.update(consoleVerificationDeliveries).set({
      leaseExpiresAt: null,
      leaseToken: null,
      status: "pending",
      updatedAt: input.now,
    }).where(and(
      eq(consoleVerificationDeliveries.authorizationId, input.authorizationId),
      eq(consoleVerificationDeliveries.deliveryNumber, input.deliveryNumber),
      eq(consoleVerificationDeliveries.status, "sending"),
      eq(consoleVerificationDeliveries.leaseToken, input.leaseToken),
    ));
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
          idle_expires_at, created_at, updated_at
        )
        SELECT ${input.family.id}, approved_user_id, ${input.family.clientName},
          'active', ${input.family.absoluteExpiresAt.toISOString()}::timestamptz,
          ${input.family.idleExpiresAt.toISOString()}::timestamptz,
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

  async listDeviceFamilies(input: DeviceFamilyListInput): Promise<DeviceFamilyListPage> {
    const cursorPredicate = input.cursor
      ? or(
          lt(consoleDeviceFamilies.createdAt, input.cursor.createdAt),
          and(
            eq(consoleDeviceFamilies.createdAt, input.cursor.createdAt),
            lt(consoleDeviceFamilies.id, input.cursor.id),
          ),
        )
      : undefined;
    const result = await this.db.execute<DeviceFamilySnapshotRow>(sql`
      WITH page_candidates AS (
        SELECT *
        FROM ${consoleDeviceFamilies}
        WHERE ${and(
          eq(consoleDeviceFamilies.userId, input.userId),
          cursorPredicate,
        ) ?? sql`true`}
        ORDER BY ${consoleDeviceFamilies.createdAt} DESC,
          ${consoleDeviceFamilies.id} DESC
        LIMIT ${input.limit + 1}
      ), overview AS (
        SELECT
          count(*) FILTER (
            WHERE ${consoleDeviceFamilies.revokedAt} IS NULL
              AND ${consoleDeviceFamilies.idleExpiresAt} > ${input.now}
              AND ${consoleDeviceFamilies.absoluteExpiresAt} > ${input.now}
          ) AS active,
          count(*) FILTER (
            WHERE ${consoleDeviceFamilies.revokedAt} IS NULL
              AND ${consoleDeviceFamilies.idleExpiresAt} > ${input.now}
              AND ${consoleDeviceFamilies.absoluteExpiresAt} > ${input.now}
              AND least(
                ${consoleDeviceFamilies.idleExpiresAt},
                ${consoleDeviceFamilies.absoluteExpiresAt}
              ) <= ${input.expiringBefore}
          ) AS expiring,
          count(*) FILTER (
            WHERE ${consoleDeviceFamilies.revokedAt} IS NOT NULL
              OR ${consoleDeviceFamilies.idleExpiresAt} <= ${input.now}
              OR ${consoleDeviceFamilies.absoluteExpiresAt} <= ${input.now}
          ) AS inactive,
          count(*) AS total
        FROM ${consoleDeviceFamilies}
        WHERE ${consoleDeviceFamilies.userId} = ${input.userId}
      )
      SELECT
        page_candidates.id AS "id",
        page_candidates.user_id AS "userId",
        page_candidates.client_name AS "clientName",
        page_candidates.status AS "status",
        page_candidates.absolute_expires_at AS "absoluteExpiresAt",
        page_candidates.idle_expires_at AS "idleExpiresAt",
        page_candidates.last_used_at AS "lastUsedAt",
        page_candidates.revoked_at AS "revokedAt",
        page_candidates.revoke_reason AS "revokeReason",
        page_candidates.created_at AS "createdAt",
        page_candidates.updated_at AS "updatedAt",
        overview.active AS "active",
        overview.expiring AS "expiring",
        overview.inactive AS "inactive",
        overview.total AS "total",
        ((SELECT count(*) FROM page_candidates) > ${input.limit})
          AS "hasNextPage"
      FROM overview
      LEFT JOIN page_candidates ON true
      ORDER BY page_candidates.created_at DESC NULLS LAST,
        page_candidates.id DESC NULLS LAST
    `);
    const snapshot = result.rows[0];
    if (!snapshot) throw new Error("Device family snapshot did not return a row");
    const families = result.rows
      .flatMap((row) => {
        const family = mapDeviceFamilySnapshot(row);
        return family ? [family] : [];
      })
      .slice(0, input.limit);
    return {
      families,
      hasNextPage: snapshot.hasNextPage,
      overview: {
        active: integerValue(snapshot.active, "active device family total"),
        expiring: integerValue(snapshot.expiring, "expiring device family total"),
        inactive: integerValue(snapshot.inactive, "inactive device family total"),
        total: integerValue(snapshot.total, "device family total"),
      },
    };
  }

  async revokeDeviceFamily(input: {
    familyId: string;
    now: Date;
    reason: "owner";
    userId: string;
  }): Promise<boolean> {
    const result = await this.db.execute(sql`
      WITH owned_family AS (
        SELECT id
        FROM ${consoleDeviceFamilies}
        WHERE id = ${input.familyId} AND user_id = ${input.userId}
      ), revoked_family AS (
        UPDATE ${consoleDeviceFamilies} family
        SET status = 'revoked',
          revoked_at = COALESCE(family.revoked_at, ${input.now.toISOString()}::timestamptz),
          revoke_reason = COALESCE(family.revoke_reason, ${input.reason}),
          updated_at = ${input.now.toISOString()}::timestamptz
        FROM owned_family
        WHERE family.id = owned_family.id
        RETURNING family.id
      ), revoked_sessions AS (
        UPDATE ${consoleSessions} session
        SET revoked_at = COALESCE(session.revoked_at, ${input.now.toISOString()}::timestamptz),
          updated_at = ${input.now.toISOString()}::timestamptz
        FROM revoked_family
        WHERE session.refresh_family_id = revoked_family.id
          AND session.revoked_at IS NULL
        RETURNING session.id
      )
      SELECT id FROM revoked_family
    `);
    return result.rows.length === 1;
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

function integerValue(value: unknown, label: string): number {
  const parsed = typeof value === "number" ? value : Number(value);
  if (!Number.isSafeInteger(parsed) || parsed < 0) {
    throw new Error(`${label} is invalid`);
  }
  return parsed;
}

function stringValue(value: unknown, label: string): string {
  if (typeof value !== "string" || value.length === 0) {
    throw new Error(`${label} is invalid`);
  }
  return value;
}

function mapDeviceFamilySnapshot(
  row: DeviceFamilySnapshotRow,
): DeviceFamilyRecord | null {
  if (row.id === null) return null;
  if (
    row.userId === null ||
    row.clientName === null ||
    row.status === null ||
    row.absoluteExpiresAt === null ||
    row.idleExpiresAt === null ||
    row.createdAt === null ||
    row.updatedAt === null
  ) {
    throw new Error("Device family snapshot is incomplete");
  }
  if (row.status !== "active" && row.status !== "revoked") {
    throw new Error("Device family snapshot has an invalid status");
  }
  return {
    absoluteExpiresAt: timestampValue(
      row.absoluteExpiresAt,
      "device family absolute expiry",
    ),
    clientName: row.clientName,
    createdAt: timestampValue(row.createdAt, "device family creation time"),
    id: row.id,
    idleExpiresAt: timestampValue(
      row.idleExpiresAt,
      "device family idle expiry",
    ),
    lastUsedAt: row.lastUsedAt
      ? timestampValue(row.lastUsedAt, "device family last use")
      : null,
    revokeReason: row.revokeReason,
    revokedAt: row.revokedAt
      ? timestampValue(row.revokedAt, "device family revocation time")
      : null,
    status: row.status,
    updatedAt: timestampValue(row.updatedAt, "device family update time"),
    userId: row.userId,
  };
}

function timestampValue(value: TimestampValue, label: string): Date {
  const parsed = value instanceof Date ? value : new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    throw new Error(`${label} is invalid`);
  }
  return parsed;
}
