import { Buffer } from "node:buffer";
import { createHash, createHmac, randomBytes, randomInt, randomUUID, timingSafeEqual } from "node:crypto";

import { deviceFamilyIdSchema } from "@/features/auth/contracts";
import type {
  AccessDevice,
  AccessDeviceListQuery,
  AccessDeviceListResponse,
  AuthorizationDecisionInput,
  DeviceAuthorizationInput,
  DeviceAuthorizationResponse,
  DeviceRefreshInput,
  DeviceTokenResponse,
  VerificationEmailInput,
} from "@/features/auth/contracts";
import type {
  AuthRepository,
  AuthorizationRecord,
  DeviceFamilyListCursor,
  DeviceFamilyRecord,
  VerificationDeliveryClaim,
} from "@/features/auth/repository";
import { PlatformError } from "@/shared/errors/platform-error";

const authorizationLifetimeMs = 10 * 60 * 1_000;
const deviceAccessLifetimeMs = 15 * 60 * 1_000;
const deviceRefreshIdleLifetimeMs = 30 * 24 * 60 * 60 * 1_000;
const deviceRefreshAbsoluteLifetimeMs = 90 * 24 * 60 * 60 * 1_000;
const webSessionLifetimeMs = 8 * 60 * 60 * 1_000;
const pollIntervalSeconds = 5;
const maxOtpAttempts = 8;
const maxEmailSends = 3;
const defaultVerificationDeliveryLeaseMs = 2 * 60 * 1_000;
const codeAlphabet = "BCDFGHJKLMNPQRSTVWXYZ23456789";
const accessExpiringSoonMs = 7 * 24 * 60 * 60 * 1_000;

export interface AuthMailer {
  // Resolve only when the provider has returned an accepted message id.
  sendVerification(input: {
    clientName: string;
    email: string;
    idempotencyKey: string;
    otp: string;
    userCode: string;
    verificationUri: string;
  }): Promise<void>;
}

export type AuthServiceOptions = {
  allowedEmails: readonly string[];
  ownerAccountId?: string;
  publicUrl: string;
  secret: string;
  verificationDeliveryLeaseMs?: number;
};

const preparedVerificationState: unique symbol = Symbol(
  "prepared-verification-delivery",
);

type PreparedVerificationState = Readonly<{
  claim: VerificationDeliveryClaim;
  email: string;
  otp: string;
}>;

// The route can retain and pass this value to deferred work, but cannot inspect
// or serialize the OTP, email, lease token, or provider idempotency material.
export type PreparedVerificationDelivery = Readonly<{
  [preparedVerificationState]: PreparedVerificationState;
}>;

export class AuthService {
  private readonly allowedEmails: ReadonlySet<string>;

  constructor(
    private readonly repository: AuthRepository,
    private readonly mailer: AuthMailer,
    private readonly options: AuthServiceOptions,
    private readonly now: () => Date = () => new Date(),
  ) {
    this.allowedEmails = new Set(options.allowedEmails.map(normalizeEmail));
  }

  async startDeviceAuthorization(
    input: DeviceAuthorizationInput,
  ): Promise<DeviceAuthorizationResponse> {
    const now = this.now();
    const expiresAt = new Date(now.getTime() + authorizationLifetimeMs);
    const deviceCode = randomBytes(32).toString("base64url");
    const record: AuthorizationRecord = {
      approvedUserId: null,
      clientName: input.clientName,
      clientType: input.clientType,
      createdAt: now,
      deviceCodeHash: digest(deviceCode),
      email: null,
      emailSendCount: 0,
      expiresAt,
      id: randomUUID(),
      lastPolledAt: null,
      otpAttempts: 0,
      otpHash: null,
      status: "pending",
      updatedAt: now,
      userCode: await this.uniqueUserCode(),
    };
    await this.repository.createAuthorization(record);
    return {
      deviceCode,
      expiresIn: Math.floor(authorizationLifetimeMs / 1_000),
      interval: pollIntervalSeconds,
      userCode: record.userCode,
      verificationUri: `${this.options.publicUrl}/console/activate`,
    };
  }

  async prepareVerification(
    input: VerificationEmailInput,
  ): Promise<PreparedVerificationDelivery | null> {
    const now = this.now();
    const userCode = normalizeUserCode(input.userCode);
    const email = normalizeEmail(input.email);

    const leaseToken = randomUUID();
    const claim = await this.repository.claimVerificationDelivery({
      eligible: this.allowedEmails.has(email),
      email,
      leaseExpiresAt: new Date(
        now.getTime() +
          (this.options.verificationDeliveryLeaseMs ??
            defaultVerificationDeliveryLeaseMs),
      ),
      leaseToken,
      maxEmailSends,
      now,
      userCode,
    });
    if (!claim) return null;

    return {
      [preparedVerificationState]: {
        claim,
        email,
        otp: this.deliveryOtp(claim),
      },
    };
  }

  async dispatchVerification(
    prepared: PreparedVerificationDelivery,
  ): Promise<void> {
    const { claim, email, otp } = prepared[preparedVerificationState];
    try {
      await this.mailer.sendVerification({
        clientName: claim.clientName,
        email,
        idempotencyKey: this.deliveryIdempotencyKey(claim),
        otp,
        userCode: claim.userCode,
        verificationUri: `${this.options.publicUrl}/console/activate`,
      });
    } catch {
      // Provider rejection/timeout is deliberately private. Releasing the
      // lease preserves the previous accepted OTP and lets a later request
      // retry this exact delivery with the same OTP and idempotency key.
      await this.repository.releaseVerificationDelivery({
        authorizationId: claim.authorizationId,
        deliveryNumber: claim.deliveryNumber,
        leaseToken: claim.leaseToken,
        now: this.now(),
      });
      return;
    }

    const acceptedAt = this.now();
    const accepted = await this.repository.acceptVerificationDelivery({
      authorizationId: claim.authorizationId,
      deliveryNumber: claim.deliveryNumber,
      email,
      leaseToken: claim.leaseToken,
      now: acceptedAt,
      otpHash: this.otpDigest(claim.authorizationId, email, otp),
    });
    if (!accepted) {
      // A lease recovery or authorization expiry may fence a provider response.
      // Explicitly release only this worker's lease; a newer lease is untouched.
      await this.repository.releaseVerificationDelivery({
        authorizationId: claim.authorizationId,
        deliveryNumber: claim.deliveryNumber,
        leaseToken: claim.leaseToken,
        now: this.now(),
      });
    }
  }

  async sendVerification(input: VerificationEmailInput): Promise<void> {
    const prepared = await this.prepareVerification(input);
    if (prepared) await this.dispatchVerification(prepared);
  }

  async decide(input: AuthorizationDecisionInput): Promise<{
    browserSession: boolean;
    sessionToken: string | null;
    userId: string;
  } | null> {
    const now = this.now();
    const userCode = normalizeUserCode(input.userCode);
    const email = normalizeEmail(input.email);
    const record = await this.repository.findAuthorizationByUserCode(userCode);
    if (!this.canVerify(record, now) || record.status !== "email_sent" || !record.email || !record.otpHash) {
      throw invalidVerification();
    }
    if (record.otpAttempts >= maxOtpAttempts) throw invalidVerification();
    const received = this.otpDigest(record.id, email, input.otp);
    if (!safeEqual(record.email, email) || !safeEqual(record.otpHash, received)) {
      await this.repository.recordOtpFailure(record.id, now);
      throw invalidVerification();
    }
    if (input.decision === "deny") {
      const denied = await this.repository.deny({ email, id: record.id, now, otpHash: received });
      if (!denied) throw invalidVerification();
      return null;
    }
    const user = await this.repository.upsertUser(email, now, this.options.ownerAccountId);
    const approved = await this.repository.approve({ email, id: record.id, now, otpHash: received, userId: user.id });
    if (!approved) throw invalidVerification();
    if (record.clientType !== "browser") {
      return { browserSession: false, sessionToken: null, userId: user.id };
    }
    const sessionToken = await this.issueWebSession(user.id);
    const consumed = await this.repository.consumeBrowser(record.id, now, user.id);
    if (!consumed) {
      await this.logout(sessionToken);
      throw invalidVerification();
    }
    return { browserSession: true, sessionToken, userId: user.id };
  }

  async poll(deviceCode: string): Promise<DeviceTokenResponse> {
    const now = this.now();
    const record = await this.repository.findAuthorizationByDeviceCodeHash(digest(deviceCode));
    if (!record || record.expiresAt <= now) throw expiredAuthorization();
    // Browser challenges may only yield an HttpOnly cookie. They must never be
    // upgraded into a durable CLI/agent bearer, even if JavaScript saw the code.
    if (record.clientType === "browser") throw expiredAuthorization();
    if (record.lastPolledAt && now.getTime() - record.lastPolledAt.getTime() < pollIntervalSeconds * 1_000) {
      throw rateLimited("Polling is faster than the interval returned by the authorization endpoint.", pollIntervalSeconds);
    }
    await this.repository.notePoll(record.id, now);
    if (record.status === "pending" || record.status === "email_sent") {
      throw new PlatformError({
        code: "authorization_pending",
        detail: "The user has not approved this device yet.",
        retryAfterSeconds: pollIntervalSeconds,
        status: 428,
        title: "Authorization pending",
      });
    }
    if (record.status === "denied") {
      throw new PlatformError({ code: "access_denied", detail: "The user denied this device authorization.", status: 403, title: "Access denied" });
    }
    if (record.status !== "approved" || !record.approvedUserId) throw expiredAuthorization();
    return this.issueDeviceFamily(record.id, record.approvedUserId, record.clientName);
  }

  async refresh(input: DeviceRefreshInput): Promise<DeviceTokenResponse> {
    const now = this.now();
    const accessToken = deviceAccessToken();
    const result = await this.repository.rotateDeviceFamily({
      access: {
        expiresAt: new Date(now.getTime() + deviceAccessLifetimeMs),
        id: randomUUID(),
        lastFour: accessToken.slice(-4),
        tokenHash: this.tokenDigest(accessToken),
      },
      nextRefresh: {
        expiresAt: new Date(now.getTime() + deviceRefreshIdleLifetimeMs),
        id: randomUUID(),
        lastFour: input.nextRefreshToken.slice(-4),
        tokenHash: this.refreshTokenDigest(input.nextRefreshToken),
      },
      now,
      refreshTokenHash: this.refreshTokenDigest(input.refreshToken),
      rotationId: input.rotationId,
    });
    if (result === "reuse") {
      throw new PlatformError({
        code: "refresh_token_reused",
        detail: "This refresh token was already replaced. The device session has been revoked.",
        status: 401,
        title: "Refresh token reuse detected",
      });
    }
    if (!result) throw invalidRefreshToken();
    return {
      accessToken,
      expiresIn: secondsUntil(now, result.accessExpiresAt),
      refreshExpiresIn: secondsUntil(now, result.refreshExpiresAt),
      refreshToken: input.nextRefreshToken,
      tokenType: "Bearer",
    };
  }

  async listAccessDevices(
    userId: string,
    query: AccessDeviceListQuery,
  ): Promise<Omit<AccessDeviceListResponse, "version">> {
    const now = this.now();
    const page = await this.repository.listDeviceFamilies({
      cursor: query.cursor
        ? this.decodeAccessDeviceCursor(query.cursor, userId)
        : undefined,
      expiringBefore: new Date(now.getTime() + accessExpiringSoonMs),
      limit: query.limit,
      now,
      userId,
    });
    const devices = page.families.map((family) => mapAccessDevice(family, now));
    const last = page.families.at(-1);
    return {
      devices,
      overview: page.overview,
      pageInfo: {
        endCursor: last
          ? this.encodeAccessDeviceCursor(last, userId)
          : null,
        hasNextPage: page.hasNextPage,
        limit: query.limit,
      },
    };
  }

  async revokeAccessDevice(familyId: string, userId: string): Promise<{ id: string; status: "revoked" }> {
    const revoked = await this.repository.revokeDeviceFamily({
      familyId,
      now: this.now(),
      reason: "owner",
      userId,
    });
    if (!revoked) {
      throw new PlatformError({
        code: "access_device_not_found",
        detail: "The device session does not exist or is not available to this console owner.",
        status: 404,
        title: "Device not found",
      });
    }
    return { id: familyId, status: "revoked" };
  }

  async authenticate(token: string, expectedKind?: "web" | "device"): Promise<{ email: string; userId: string }> {
    const match = /^fcheap_(web|device)_([A-Za-z0-9_-]{43})$/u.exec(token);
    if (!match || (expectedKind && match[1] !== expectedKind)) {
      throw unauthorized();
    }
    const user = await this.repository.findActiveSession(this.tokenDigest(token), this.now(), expectedKind);
    if (!user) throw unauthorized();
    return { email: user.email, userId: user.id };
  }

  async logout(token: string): Promise<void> {
    await this.repository.revokeSession(this.tokenDigest(token), this.now());
  }

  private async issueDeviceFamily(
    authorizationId: string,
    userId: string,
    clientName: string,
  ): Promise<DeviceTokenResponse> {
    const now = this.now();
    const accessToken = deviceAccessToken();
    const refreshToken = deviceRefreshToken();
    const absoluteExpiresAt = new Date(now.getTime() + deviceRefreshAbsoluteLifetimeMs);
    const refreshExpiresAt = new Date(now.getTime() + deviceRefreshIdleLifetimeMs);
    const accessExpiresAt = new Date(now.getTime() + deviceAccessLifetimeMs);
    const consumed = await this.repository.consumeDeviceAuthorization({
      access: {
        expiresAt: accessExpiresAt,
        id: randomUUID(),
        lastFour: accessToken.slice(-4),
        tokenHash: this.tokenDigest(accessToken),
      },
      authorizationId,
      family: {
        absoluteExpiresAt,
        clientName,
        id: randomUUID(),
        idleExpiresAt: refreshExpiresAt,
      },
      now,
      refresh: {
        expiresAt: refreshExpiresAt,
        id: randomUUID(),
        lastFour: refreshToken.slice(-4),
        tokenHash: this.refreshTokenDigest(refreshToken),
      },
      userId,
    });
    if (!consumed) throw expiredAuthorization();
    return {
      accessToken,
      expiresIn: Math.floor(deviceAccessLifetimeMs / 1_000),
      refreshExpiresIn: Math.floor(deviceRefreshIdleLifetimeMs / 1_000),
      refreshToken,
      tokenType: "Bearer",
    };
  }

  private async issueWebSession(userId: string): Promise<string> {
    const now = this.now();
    const raw = `fcheap_web_${randomBytes(32).toString("base64url")}`;
    await this.repository.createSession({
      expiresAt: new Date(now.getTime() + webSessionLifetimeMs),
      id: randomUUID(),
      lastFour: raw.slice(-4),
      now,
      tokenHash: this.tokenDigest(raw),
      userId,
    });
    return raw;
  }

  private canVerify(record: AuthorizationRecord | null, now: Date): record is AuthorizationRecord {
    return Boolean(record && record.expiresAt > now && ["pending", "email_sent"].includes(record.status));
  }

  private otpDigest(id: string, email: string, otp: string): string {
    return createHmac("sha256", this.options.secret)
      .update(`${id}\n${email}\n${otp}`)
      .digest("hex");
  }

  private deliveryOtp(claim: VerificationDeliveryClaim): string {
    const bytes = createHmac("sha256", this.options.secret)
      .update(
        `verification-delivery-otp-v1\n${claim.authorizationId}\n${claim.deliveryNumber}\n${claim.email}`,
      )
      .digest();
    return String(bytes.readUInt32BE(0) % 1_000_000).padStart(6, "0");
  }

  private deliveryIdempotencyKey(claim: VerificationDeliveryClaim): string {
    const digest = createHmac("sha256", this.options.secret)
      .update(
        `verification-delivery-idempotency-v1\n${claim.authorizationId}\n${claim.deliveryNumber}`,
      )
      .digest("base64url");
    return `device-verification/${digest}`;
  }

  private tokenDigest(value: string): string {
    return createHmac("sha256", this.options.secret)
      .update(`session\n${value}`)
      .digest("hex");
  }

  private refreshTokenDigest(value: string): string {
    return createHmac("sha256", this.options.secret)
      .update(`refresh\n${value}`)
      .digest("hex");
  }

  private encodeAccessDeviceCursor(
    family: Pick<DeviceFamilyRecord, "createdAt" | "id">,
    userId: string,
  ): string {
    const payload = Buffer.from(
      JSON.stringify([1, family.createdAt.toISOString(), family.id]),
      "utf8",
    ).toString("base64url");
    return `${payload}.${this.accessDeviceCursorSignature(payload, userId)}`;
  }

  private decodeAccessDeviceCursor(
    encoded: string,
    userId: string,
  ): DeviceFamilyListCursor {
    try {
      if (encoded.length < 1 || encoded.length > 512) throw new Error("invalid length");
      const parts = encoded.split(".");
      if (
        parts.length !== 2 ||
        !parts[0] ||
        !parts[1] ||
        !/^[A-Za-z0-9_-]+$/u.test(parts[0]) ||
        !/^[A-Za-z0-9_-]{43}$/u.test(parts[1]) ||
        !safeEqual(this.accessDeviceCursorSignature(parts[0], userId), parts[1])
      ) {
        throw new Error("invalid signature");
      }
      const value = JSON.parse(
        Buffer.from(parts[0], "base64url").toString("utf8"),
      ) as unknown;
      if (
        !Array.isArray(value) ||
        value.length !== 3 ||
        value[0] !== 1 ||
        typeof value[1] !== "string" ||
        typeof value[2] !== "string"
      ) {
        throw new Error("invalid payload");
      }
      const createdAt = new Date(value[1]);
      if (
        Number.isNaN(createdAt.getTime()) ||
        createdAt.toISOString() !== value[1]
      ) {
        throw new Error("invalid timestamp");
      }
      deviceFamilyIdSchema.parse(value[2]);
      return { createdAt, id: value[2] };
    } catch {
      throw invalidAccessDeviceCursor();
    }
  }

  private accessDeviceCursorSignature(payload: string, userId: string): string {
    return createHmac("sha256", this.options.secret)
      .update(`access-device-cursor\n${userId}\n${payload}`)
      .digest("base64url");
  }

  private async uniqueUserCode(): Promise<string> {
    for (let attempt = 0; attempt < 8; attempt += 1) {
      const value = `${randomCode(4)}-${randomCode(4)}`;
      if (!(await this.repository.findAuthorizationByUserCode(value))) return value;
    }
    throw new Error("Could not allocate a unique device user code");
  }
}

function mapAccessDevice(family: DeviceFamilyRecord, now: Date): AccessDevice {
  return {
    absoluteExpiresAt: family.absoluteExpiresAt.toISOString(),
    clientName: family.clientName,
    createdAt: family.createdAt.toISOString(),
    id: family.id,
    idleExpiresAt: family.idleExpiresAt.toISOString(),
    lastRefreshedAt: family.lastUsedAt?.toISOString() ?? null,
    revokedAt: family.revokedAt?.toISOString() ?? null,
    status: family.revokedAt
      ? "revoked"
      : family.idleExpiresAt <= now || family.absoluteExpiresAt <= now
        ? "expired"
        : "active",
  };
}

function randomCode(length: number): string {
  let value = "";
  for (let index = 0; index < length; index += 1) {
    value += codeAlphabet[randomInt(0, codeAlphabet.length)];
  }
  return value;
}

export function normalizeUserCode(value: string): string {
  const compact = value.toUpperCase().replace(/[^A-Z0-9]/gu, "");
  return compact.length === 8 ? `${compact.slice(0, 4)}-${compact.slice(4)}` : value.toUpperCase();
}

export function normalizeEmail(value: string): string {
  return value.trim().toLowerCase();
}

function digest(value: string): string {
  return createHash("sha256").update(value).digest("hex");
}

function deviceAccessToken(): string {
  return `fcheap_device_${randomBytes(32).toString("base64url")}`;
}

function deviceRefreshToken(): string {
  return `fcheap_refresh_${randomBytes(32).toString("base64url")}`;
}

function secondsUntil(now: Date, expiresAt: Date): number {
  return Math.max(0, Math.floor((expiresAt.getTime() - now.getTime()) / 1_000));
}

function safeEqual(left: string, right: string): boolean {
  const leftDigest = createHash("sha256").update(left).digest();
  const rightDigest = createHash("sha256").update(right).digest();
  return timingSafeEqual(leftDigest, rightDigest);
}

function invalidVerification(): PlatformError {
  return new PlatformError({ code: "invalid_verification", detail: "The verification code is invalid or expired.", status: 400, title: "Invalid verification" });
}

function expiredAuthorization(): PlatformError {
  return new PlatformError({ code: "expired_authorization", detail: "The device authorization is invalid, expired, or already consumed.", status: 400, title: "Expired authorization" });
}

function invalidRefreshToken(): PlatformError {
  return new PlatformError({
    code: "invalid_refresh_token",
    detail: "The refresh token is invalid, expired, or belongs to a revoked device session.",
    status: 401,
    title: "Invalid refresh token",
  });
}

function invalidAccessDeviceCursor(): PlatformError {
  return new PlatformError({
    code: "invalid_cursor",
    detail: "The access device cursor is invalid.",
    status: 422,
    title: "Invalid cursor",
  });
}

function rateLimited(detail: string, retryAfterSeconds = 60): PlatformError {
  return new PlatformError({ code: "rate_limited", detail, retryAfterSeconds, status: 429, title: "Too many requests" });
}

function unauthorized(): PlatformError {
  return new PlatformError({ code: "unauthorized", detail: "A valid console session is required.", status: 401, title: "Unauthorized" });
}
