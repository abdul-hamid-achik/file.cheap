import { createHash } from "node:crypto";

import {
  afterAll,
  beforeAll,
  beforeEach,
  describe,
  expect,
  test,
} from "bun:test";

import type {
  DeviceFamilyIssueInput,
  RefreshRotationInput,
} from "@/features/auth/repository";
import { DrizzleAuthRepository } from "@/platform/database/auth-repository";
import { getDatabase } from "@/platform/database/client";
import {
  consoleAuthorizations,
  consoleDeviceFamilies,
  consoleUsers,
  consoleVerificationDeliveries,
} from "@/platform/database/schema";
import {
  openPostgresTestDatabase,
  truncatePostgresTestData,
} from "./postgres-test-database";

const databaseUrl = process.env.FILECHEAP_POSTGRES_TEST_URL;
const now = new Date("2026-07-26T18:00:00.000Z");
const ownerId = "acc_auth_postgres_owner";

describe.skipIf(!databaseUrl)("auth PostgreSQL repository", () => {
  let harness: ReturnType<typeof openPostgresTestDatabase>;
  let repository: DrizzleAuthRepository;

  beforeAll(() => {
    harness = openPostgresTestDatabase();
    repository = new DrizzleAuthRepository(
      harness.database as unknown as ReturnType<typeof getDatabase>,
    );
  });

  beforeEach(async () => {
    await truncatePostgresTestData(harness);
  });

  afterAll(async () => {
    await truncatePostgresTestData(harness);
    await harness.pool.end();
  });

  test("coalesces concurrent verification delivery claims and activates proof only on acceptance", async () => {
    await insertPendingAuthorization();
    const leaseExpiresAt = new Date(now.getTime() + 2 * 60 * 1_000);
    const claims = await Promise.all(
      ["lease-one", "lease-two", "lease-three"].map((leaseToken) =>
        repository.claimVerificationDelivery({
          eligible: true,
          email: "auth-owner@example.invalid",
          leaseExpiresAt,
          leaseToken,
          maxEmailSends: 3,
          now,
          userCode: "PGOTP001",
        })
      ),
    );
    const claimed = claims.filter((claim) => claim !== null);
    expect(claimed).toHaveLength(1);
    expect(claimed[0]).toMatchObject({
      authorizationId: "authorization-postgres-otp",
      deliveryNumber: 1,
      email: "auth-owner@example.invalid",
      userCode: "PGOTP001",
    });

    const beforeAcceptance = await harness.database.select().from(consoleAuthorizations);
    expect(beforeAcceptance[0]).toMatchObject({
      email: null,
      emailSendCount: 0,
      otpHash: null,
      status: "pending",
    });
    expect(await harness.database.select().from(consoleVerificationDeliveries))
      .toEqual([expect.objectContaining({ status: "sending" })]);

    const claim = claimed[0]!;
    expect(await repository.acceptVerificationDelivery({
      authorizationId: claim.authorizationId,
      deliveryNumber: claim.deliveryNumber,
      email: claim.email,
      leaseToken: claim.leaseToken,
      now,
      otpHash: hash("accepted-otp"),
    })).toBe(true);
    expect((await harness.database.select().from(consoleAuthorizations))[0])
      .toMatchObject({
        email: "auth-owner@example.invalid",
        emailSendCount: 1,
        otpHash: hash("accepted-otp"),
        status: "email_sent",
      });
    expect((await harness.database.select().from(consoleVerificationDeliveries))[0])
      .toMatchObject({
        acceptedAt: now,
        leaseExpiresAt: null,
        leaseToken: null,
        status: "accepted",
      });
  });

  test("retries one failed delivery without consuming a send or replacing prior proof", async () => {
    await insertPendingAuthorization({
      email: "auth-owner@example.invalid",
      emailSendCount: 1,
      otpHash: hash("previous-otp"),
      status: "email_sent",
    });
    const first = await repository.claimVerificationDelivery({
      eligible: true,
      email: "auth-owner@example.invalid",
      leaseExpiresAt: new Date(now.getTime() + 2 * 60 * 1_000),
      leaseToken: "failed-provider-lease",
      maxEmailSends: 3,
      now,
      userCode: "PGOTP001",
    });
    expect(first).toMatchObject({ deliveryNumber: 2 });
    await repository.releaseVerificationDelivery({
      authorizationId: first!.authorizationId,
      deliveryNumber: first!.deliveryNumber,
      leaseToken: first!.leaseToken,
      now,
    });

    expect((await harness.database.select().from(consoleAuthorizations))[0])
      .toMatchObject({
        emailSendCount: 1,
        otpHash: hash("previous-otp"),
        status: "email_sent",
      });
    const retry = await repository.claimVerificationDelivery({
      eligible: true,
      email: "auth-owner@example.invalid",
      leaseExpiresAt: new Date(now.getTime() + 2 * 60 * 1_000),
      leaseToken: "retry-provider-lease",
      maxEmailSends: 3,
      now: new Date(now.getTime() + 1_000),
      userCode: "PGOTP001",
    });
    expect(retry).toMatchObject({
      authorizationId: first!.authorizationId,
      deliveryNumber: first!.deliveryNumber,
    });
    expect(await harness.database.select().from(consoleVerificationDeliveries))
      .toHaveLength(1);
  });

  test("does not create or claim a delivery for an expired authorization", async () => {
    await insertPendingAuthorization({
      createdAt: new Date(now.getTime() - 10 * 60 * 1_000),
      expiresAt: now,
      updatedAt: new Date(now.getTime() - 10 * 60 * 1_000),
    });
    expect(await repository.claimVerificationDelivery({
      eligible: true,
      email: "auth-owner@example.invalid",
      leaseExpiresAt: new Date(now.getTime() + 2 * 60 * 1_000),
      leaseToken: "expired-lease",
      maxEmailSends: 3,
      now,
      userCode: "PGOTP001",
    })).toBeNull();
    expect(await harness.database.select().from(consoleVerificationDeliveries))
      .toHaveLength(0);
  });

  test("runs the authorization lookup without mutating state for an ineligible email", async () => {
    await insertPendingAuthorization();
    expect(await repository.claimVerificationDelivery({
      eligible: false,
      email: "not-allowlisted@example.invalid",
      leaseExpiresAt: new Date(now.getTime() + 2 * 60 * 1_000),
      leaseToken: "ineligible-lease",
      maxEmailSends: 3,
      now,
      userCode: "PGOTP001",
    })).toBeNull();
    expect(await harness.database.select().from(consoleVerificationDeliveries))
      .toHaveLength(0);
    expect((await harness.database.select().from(consoleAuthorizations))[0])
      .toMatchObject({ email: null, emailSendCount: 0, otpHash: null, status: "pending" });
  });

  test("recovers an expired delivery lease and fences the stale worker", async () => {
    await insertPendingAuthorization();
    const first = await repository.claimVerificationDelivery({
      eligible: true,
      email: "auth-owner@example.invalid",
      leaseExpiresAt: new Date(now.getTime() + 1_000),
      leaseToken: "stale-worker",
      maxEmailSends: 3,
      now,
      userCode: "PGOTP001",
    });
    expect(first).not.toBeNull();
    expect(await repository.claimVerificationDelivery({
      eligible: true,
      email: "auth-owner@example.invalid",
      leaseExpiresAt: new Date(now.getTime() + 2_000),
      leaseToken: "early-worker",
      maxEmailSends: 3,
      now: new Date(now.getTime() + 999),
      userCode: "PGOTP001",
    })).toBeNull();

    const recovered = await repository.claimVerificationDelivery({
      eligible: true,
      email: "auth-owner@example.invalid",
      leaseExpiresAt: new Date(now.getTime() + 3_000),
      leaseToken: "recovery-worker",
      maxEmailSends: 3,
      now: new Date(now.getTime() + 1_000),
      userCode: "PGOTP001",
    });
    expect(recovered).toMatchObject({ deliveryNumber: 1 });
    expect(await repository.acceptVerificationDelivery({
      authorizationId: first!.authorizationId,
      deliveryNumber: first!.deliveryNumber,
      email: first!.email,
      leaseToken: first!.leaseToken,
      now: new Date(now.getTime() + 1_001),
      otpHash: hash("stale-worker-otp"),
    })).toBe(false);
    expect(await repository.acceptVerificationDelivery({
      authorizationId: recovered!.authorizationId,
      deliveryNumber: recovered!.deliveryNumber,
      email: recovered!.email,
      leaseToken: recovered!.leaseToken,
      now: new Date(now.getTime() + 1_001),
      otpHash: hash("recovered-worker-otp"),
    })).toBe(true);
    expect((await harness.database.select().from(consoleAuthorizations))[0])
      .toMatchObject({ otpHash: hash("recovered-worker-otp") });
  });

  test("serializes reclaim before stale acceptance without advancing authorization alone", async () => {
    await insertPendingAuthorization();
    const first = await repository.claimVerificationDelivery({
      eligible: true,
      email: "auth-owner@example.invalid",
      leaseExpiresAt: new Date(now.getTime() + 1_000),
      leaseToken: "serialized-stale-worker",
      maxEmailSends: 3,
      now,
      userCode: "PGOTP001",
    });
    expect(first).not.toBeNull();

    const blocker = await harness.pool.connect();
    try {
      await blocker.query("BEGIN");
      await blocker.query(
        "SELECT id FROM console_authorizations WHERE id = $1 FOR UPDATE",
        [first!.authorizationId],
      );

      const recoveredPromise = repository.claimVerificationDelivery({
        eligible: true,
        email: first!.email,
        leaseExpiresAt: new Date(now.getTime() + 4_000),
        leaseToken: "serialized-recovery-worker",
        maxEmailSends: 3,
        now: new Date(now.getTime() + 1_000),
        userCode: "PGOTP001",
      });
      await nextTurn();
      const staleAcceptancePromise = repository.acceptVerificationDelivery({
        authorizationId: first!.authorizationId,
        deliveryNumber: first!.deliveryNumber,
        email: first!.email,
        leaseToken: first!.leaseToken,
        now: new Date(now.getTime() + 1_001),
        otpHash: hash("serialized-stale-otp"),
      });
      await nextTurn();
      await blocker.query("COMMIT");

      const recovered = await recoveredPromise;
      expect(recovered).toMatchObject({
        deliveryNumber: 1,
        leaseToken: "serialized-recovery-worker",
      });
      expect(await staleAcceptancePromise).toBe(false);
      expect((await harness.database.select().from(consoleAuthorizations))[0])
        .toMatchObject({
          email: null,
          emailSendCount: 0,
          otpHash: null,
          status: "pending",
        });
      expect((await harness.database.select().from(consoleVerificationDeliveries))[0])
        .toMatchObject({
          acceptedAt: null,
          leaseToken: "serialized-recovery-worker",
          status: "sending",
        });

      expect(await repository.acceptVerificationDelivery({
        authorizationId: recovered!.authorizationId,
        deliveryNumber: recovered!.deliveryNumber,
        email: recovered!.email,
        leaseToken: recovered!.leaseToken,
        now: new Date(now.getTime() + 1_002),
        otpHash: hash("serialized-recovered-otp"),
      })).toBe(true);
      const acceptedAuthorization = (await harness.database.select()
        .from(consoleAuthorizations))[0];
      const acceptedDelivery = (await harness.database.select()
        .from(consoleVerificationDeliveries))[0];
      expect(acceptedAuthorization).toMatchObject({
        emailSendCount: 1,
        otpHash: hash("serialized-recovered-otp"),
        status: "email_sent",
      });
      expect(acceptedDelivery).toMatchObject({
        leaseToken: null,
        status: "accepted",
      });
    } finally {
      await blocker.query("ROLLBACK").catch(() => undefined);
      blocker.release();
    }
  });

  test("pages device families with an owner-wide snapshot overview", async () => {
    await insertOwner();
    const otherOwnerId = "acc_auth_postgres_other";
    await harness.database.insert(consoleUsers).values({
      createdAt: now,
      email: "auth-other@example.invalid",
      id: otherOwnerId,
      updatedAt: now,
    });
    await harness.database.insert(consoleDeviceFamilies).values([
      ...deviceFamilies(ownerId, 57),
      ...deviceFamilies(otherOwnerId, 3, 100),
    ]);

    const first = await repository.listDeviceFamilies({
      expiringBefore: new Date(now.getTime() + 7 * 24 * 60 * 60 * 1_000),
      limit: 20,
      now,
      userId: ownerId,
    });
    expect(first.families).toHaveLength(20);
    expect(first.hasNextPage).toBe(true);
    expect(first.overview).toEqual({
      active: 48,
      expiring: 6,
      inactive: 9,
      total: 57,
    });

    const last = first.families.at(-1);
    if (!last) throw new Error("Expected a first page of device families");
    const second = await repository.listDeviceFamilies({
      cursor: { createdAt: last.createdAt, id: last.id },
      expiringBefore: new Date(now.getTime() + 7 * 24 * 60 * 60 * 1_000),
      limit: 20,
      now,
      userId: ownerId,
    });
    expect(second.families).toHaveLength(20);
    expect(second.overview).toEqual(first.overview);
    expect(second.families.some((family) =>
      first.families.some((candidate) => candidate.id === family.id)
    )).toBe(false);
    expect(second.families.every((family) => family.userId === ownerId)).toBe(true);
  });

  test("allows exactly one concurrent device authorization claim", async () => {
    await insertApprovedAuthorization();
    const inputs = [issueInput("one"), issueInput("two")];
    const results = await Promise.all(
      inputs.map((input) => repository.consumeDeviceAuthorization(input)),
    );

    expect(results.filter(Boolean)).toHaveLength(1);
    const counts = await harness.pool.query<{
      families: number;
      refresh_tokens: number;
      sessions: number;
    }>(`
      SELECT
        (SELECT count(*)::integer FROM console_device_families) AS families,
        (SELECT count(*)::integer FROM console_refresh_tokens) AS refresh_tokens,
        (SELECT count(*)::integer FROM console_sessions) AS sessions
    `);
    expect(counts.rows[0]).toEqual({
      families: 1,
      refresh_tokens: 1,
      sessions: 1,
    });
  });

  test("makes refresh retries idempotent and revokes a reused family", async () => {
    await insertApprovedAuthorization();
    const issued = issueInput("issued");
    expect(await repository.consumeDeviceAuthorization(issued)).toBe(true);

    const rotation = rotationInput("retry", issued.refresh.tokenHash, "rotation-1");
    const first = await repository.rotateDeviceFamily(rotation);
    const replay = await repository.rotateDeviceFamily({
      ...rotation,
      access: accessToken("retry-access-replay"),
    });
    expect(first).toEqual({
      accessExpiresAt: rotation.access.expiresAt,
      refreshExpiresAt: rotation.nextRefresh.expiresAt,
    });
    expect(replay).toEqual(first);

    const reused = await repository.rotateDeviceFamily(
      rotationInput("reuse", issued.refresh.tokenHash, "rotation-2"),
    );
    expect(reused).toBe("reuse");
    const state = await harness.pool.query<{
      active_sessions: number;
      family_status: string;
      revoke_reason: string | null;
    }>(`
      SELECT family.status AS family_status,
        family.revoke_reason,
        count(session.id) FILTER (WHERE session.revoked_at IS NULL)::integer
          AS active_sessions
      FROM console_device_families family
      LEFT JOIN console_sessions session ON session.refresh_family_id = family.id
      GROUP BY family.id
    `);
    expect(state.rows[0]).toEqual({
      active_sessions: 0,
      family_status: "revoked",
      revoke_reason: "refresh-reuse",
    });
  });

  async function insertOwner(): Promise<void> {
    await harness.database.insert(consoleUsers).values({
      createdAt: now,
      email: "auth-owner@example.invalid",
      id: ownerId,
      updatedAt: now,
    });
  }

  async function insertApprovedAuthorization(): Promise<void> {
    await insertOwner();
    await harness.database.insert(consoleAuthorizations).values({
      approvedUserId: ownerId,
      clientName: "PostgreSQL integration agent",
      clientType: "agent",
      createdAt: now,
      deviceCodeHash: hash("device-code"),
      email: "auth-owner@example.invalid",
      emailSendCount: 1,
      expiresAt: new Date(now.getTime() + 10 * 60 * 1_000),
      id: "authorization-postgres-test",
      lastPolledAt: null,
      otpAttempts: 0,
      otpHash: hash("otp"),
      status: "approved",
      updatedAt: now,
      userCode: "PGTEST01",
    });
  }

  async function insertPendingAuthorization(
    overrides: Partial<typeof consoleAuthorizations.$inferInsert> = {},
  ): Promise<void> {
    await harness.database.insert(consoleAuthorizations).values({
      approvedUserId: null,
      clientName: "PostgreSQL verification agent",
      clientType: "agent",
      createdAt: now,
      deviceCodeHash: hash("otp-device-code"),
      email: null,
      emailSendCount: 0,
      expiresAt: new Date(now.getTime() + 10 * 60 * 1_000),
      id: "authorization-postgres-otp",
      lastPolledAt: null,
      otpAttempts: 0,
      otpHash: null,
      status: "pending",
      updatedAt: now,
      userCode: "PGOTP001",
      ...overrides,
    });
  }
});

function deviceFamilies(userId: string, total: number, offset = 0) {
  return Array.from({ length: total }, (_, index) => {
    const ordinal = offset + index;
    const createdAt = new Date(
      now.getTime() - 2 * 24 * 60 * 60 * 1_000 - Math.floor(index / 3) * 1_000,
    );
    const revoked = index >= 5 && index < 9;
    const idleExpiresAt = index < 5
      ? new Date(now.getTime() - 60 * 60 * 1_000)
      : index < 15
        ? new Date(now.getTime() + 3 * 24 * 60 * 60 * 1_000)
        : new Date(now.getTime() + 30 * 24 * 60 * 60 * 1_000);
    return {
      absoluteExpiresAt: new Date(now.getTime() + 60 * 24 * 60 * 60 * 1_000),
      clientName: `Agent ${ordinal}`,
      createdAt,
      id: `family-postgres-${userId}-${String(ordinal).padStart(4, "0")}`,
      idleExpiresAt,
      lastUsedAt: null,
      revokeReason: revoked ? "owner" : null,
      revokedAt: revoked ? new Date(now.getTime() - 60 * 60 * 1_000) : null,
      status: revoked ? "revoked" : "active",
      updatedAt: now,
      userId,
    } as const;
  });
}

function issueInput(label: string): DeviceFamilyIssueInput {
  return {
    access: accessToken(`${label}-access`),
    authorizationId: "authorization-postgres-test",
    family: {
      absoluteExpiresAt: new Date(now.getTime() + 90 * 24 * 60 * 60 * 1_000),
      clientName: `Agent ${label}`,
      id: `family-${label}`,
      idleExpiresAt: new Date(now.getTime() + 30 * 24 * 60 * 60 * 1_000),
    },
    now,
    refresh: {
      expiresAt: new Date(now.getTime() + 30 * 24 * 60 * 60 * 1_000),
      id: `refresh-${label}`,
      lastFour: "r001",
      tokenHash: hash(`${label}-refresh`),
    },
    userId: ownerId,
  };
}

function rotationInput(
  label: string,
  refreshTokenHash: string,
  rotationId: string,
): RefreshRotationInput {
  return {
    access: accessToken(`${label}-access`),
    nextRefresh: {
      expiresAt: new Date(now.getTime() + 30 * 24 * 60 * 60 * 1_000),
      id: `next-refresh-${label}`,
      lastFour: "r002",
      tokenHash: hash(`${label}-next-refresh`),
    },
    now,
    refreshTokenHash,
    rotationId,
  };
}

function accessToken(label: string): DeviceFamilyIssueInput["access"] {
  return {
    expiresAt: new Date(now.getTime() + 15 * 60 * 1_000),
    id: `access-${label}`,
    lastFour: "a001",
    tokenHash: hash(label),
  };
}

function hash(value: string): string {
  return createHash("sha256").update(value).digest("hex");
}

async function nextTurn(): Promise<void> {
  await new Promise<void>((resolve) => setTimeout(resolve, 20));
}
