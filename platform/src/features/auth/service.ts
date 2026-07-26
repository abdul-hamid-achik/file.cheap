import { createHash, createHmac, randomBytes, randomInt, randomUUID, timingSafeEqual } from "node:crypto";

import type {
  AuthorizationDecisionInput,
  DeviceAuthorizationInput,
  DeviceAuthorizationResponse,
  DeviceRefreshInput,
  DeviceTokenResponse,
  VerificationEmailInput,
} from "@/features/auth/contracts";
import type { AuthRepository, AuthorizationRecord } from "@/features/auth/repository";
import { PlatformError } from "@/shared/errors/platform-error";

const authorizationLifetimeMs = 10 * 60 * 1_000;
const deviceAccessLifetimeMs = 15 * 60 * 1_000;
const deviceRefreshIdleLifetimeMs = 30 * 24 * 60 * 60 * 1_000;
const deviceRefreshAbsoluteLifetimeMs = 90 * 24 * 60 * 60 * 1_000;
const webSessionLifetimeMs = 8 * 60 * 60 * 1_000;
const pollIntervalSeconds = 5;
const maxOtpAttempts = 8;
const maxEmailSends = 3;
const codeAlphabet = "BCDFGHJKLMNPQRSTVWXYZ23456789";

export interface AuthMailer {
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
};

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

  async sendVerification(input: VerificationEmailInput): Promise<void> {
    const now = this.now();
    const userCode = normalizeUserCode(input.userCode);
    const email = normalizeEmail(input.email);
    const record = await this.repository.findAuthorizationByUserCode(userCode);
    if (!this.canVerify(record, now) || !this.allowedEmails.has(email)) {
      return;
    }
    if (record.emailSendCount >= maxEmailSends) {
      throw rateLimited("Too many verification emails were requested for this code.");
    }
    const otp = String(randomInt(0, 1_000_000)).padStart(6, "0");
    const updated = await this.repository.markEmailSent({
      email,
      id: record.id,
      now,
      otpHash: this.otpDigest(record.id, email, otp),
    });
    if (!updated) return;
    await this.mailer.sendVerification({
      clientName: record.clientName,
      email,
      idempotencyKey: `device-verification/${record.id}/${updated.emailSendCount}`,
      otp,
      userCode,
      verificationUri: `${this.options.publicUrl}/console/activate`,
    });
  }

  async decide(input: AuthorizationDecisionInput): Promise<{ sessionToken: string; userId: string } | null> {
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
    const sessionToken = await this.issueWebSession(user.id);
    if (record.clientType === "browser") {
      const consumed = await this.repository.consumeBrowser(record.id, now, user.id);
      if (!consumed) {
        await this.logout(sessionToken);
        throw invalidVerification();
      }
    }
    return { sessionToken, userId: user.id };
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

  private async uniqueUserCode(): Promise<string> {
    for (let attempt = 0; attempt < 8; attempt += 1) {
      const value = `${randomCode(4)}-${randomCode(4)}`;
      if (!(await this.repository.findAuthorizationByUserCode(value))) return value;
    }
    throw new Error("Could not allocate a unique device user code");
  }
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

function rateLimited(detail: string, retryAfterSeconds = 60): PlatformError {
  return new PlatformError({ code: "rate_limited", detail, retryAfterSeconds, status: 429, title: "Too many requests" });
}

function unauthorized(): PlatformError {
  return new PlatformError({ code: "unauthorized", detail: "A valid console session is required.", status: 401, title: "Unauthorized" });
}
