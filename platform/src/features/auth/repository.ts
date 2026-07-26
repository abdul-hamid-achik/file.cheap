export type AuthorizationStatus =
  | "pending"
  | "email_sent"
  | "approved"
  | "denied"
  | "consumed";

export type AuthorizationRecord = {
  approvedUserId: string | null;
  clientName: string;
  clientType: "cli" | "tv" | "agent" | "browser";
  createdAt: Date;
  deviceCodeHash: string;
  email: string | null;
  emailSendCount: number;
  expiresAt: Date;
  id: string;
  lastPolledAt: Date | null;
  otpAttempts: number;
  otpHash: string | null;
  status: AuthorizationStatus;
  updatedAt: Date;
  userCode: string;
};

export type UserRecord = {
  createdAt: Date;
  email: string;
  id: string;
};

export type DeviceFamilyIssueInput = {
  access: {
    expiresAt: Date;
    id: string;
    lastFour: string;
    tokenHash: string;
  };
  authorizationId: string;
  family: {
    absoluteExpiresAt: Date;
    clientName: string;
    id: string;
    idleExpiresAt: Date;
  };
  now: Date;
  refresh: {
    expiresAt: Date;
    id: string;
    lastFour: string;
    tokenHash: string;
  };
  userId: string;
};

export type RefreshRotationInput = {
  access: {
    expiresAt: Date;
    id: string;
    lastFour: string;
    tokenHash: string;
  };
  nextRefresh: {
    expiresAt: Date;
    id: string;
    lastFour: string;
    tokenHash: string;
  };
  now: Date;
  refreshTokenHash: string;
  rotationId: string;
};

export type RefreshRotationResult = {
  accessExpiresAt: Date;
  refreshExpiresAt: Date;
} | "reuse" | null;

export interface AuthRepository {
  createAuthorization(record: AuthorizationRecord): Promise<void>;
  findAuthorizationByDeviceCodeHash(
    deviceCodeHash: string,
  ): Promise<AuthorizationRecord | null>;
  findAuthorizationByUserCode(
    userCode: string,
  ): Promise<AuthorizationRecord | null>;
  markEmailSent(input: {
    email: string;
    id: string;
    now: Date;
    otpHash: string;
  }): Promise<AuthorizationRecord | null>;
  recordOtpFailure(id: string, now: Date): Promise<void>;
  approve(input: {
    email: string;
    id: string;
    now: Date;
    otpHash: string;
    userId: string;
  }): Promise<AuthorizationRecord | null>;
  deny(input: { email: string; id: string; now: Date; otpHash: string }): Promise<boolean>;
  consumeBrowser(id: string, now: Date, userId: string): Promise<boolean>;
  consumeDeviceAuthorization(input: DeviceFamilyIssueInput): Promise<boolean>;
  rotateDeviceFamily(input: RefreshRotationInput): Promise<RefreshRotationResult>;
  createSession(input: {
    expiresAt: Date;
    id: string;
    lastFour: string;
    now: Date;
    tokenHash: string;
    userId: string;
  }): Promise<void>;
  notePoll(id: string, now: Date): Promise<void>;
  findActiveSession(tokenHash: string, now: Date, kind?: "web" | "device"): Promise<UserRecord | null>;
  revokeSession(tokenHash: string, now: Date): Promise<void>;
  upsertUser(email: string, now: Date, preferredId?: string): Promise<UserRecord>;
}

export class InMemoryAuthRepository implements AuthRepository {
  readonly authorizations = new Map<string, AuthorizationRecord>();
  readonly deviceFamilies = new Map<string, {
    absoluteExpiresAt: Date;
    idleExpiresAt: Date;
    revokedAt: Date | null;
    userId: string;
  }>();
  readonly refreshTokens = new Map<string, {
    expiresAt: Date;
    familyId: string;
    generation: number;
    replacedByTokenHash: string | null;
    rotationId: string | null;
    tokenHash: string;
    usedAt: Date | null;
  }>();
  readonly tokens = new Map<string, { expiresAt: Date; kind: "web" | "device"; refreshFamilyId: string | null; revokedAt: Date | null; tokenHash: string; userId: string }>();
  readonly users = new Map<string, UserRecord>();

  async createAuthorization(record: AuthorizationRecord): Promise<void> {
    if ([...this.authorizations.values()].some((item) => item.userCode === record.userCode)) {
      throw new Error("duplicate user code");
    }
    this.authorizations.set(record.id, { ...record });
  }

  async findAuthorizationByDeviceCodeHash(deviceCodeHash: string) {
    return [...this.authorizations.values()].find(
      (record) => record.deviceCodeHash === deviceCodeHash,
    ) ?? null;
  }

  async findAuthorizationByUserCode(userCode: string) {
    return [...this.authorizations.values()].find(
      (record) => record.userCode === userCode,
    ) ?? null;
  }

  async markEmailSent(input: { email: string; id: string; now: Date; otpHash: string }) {
    const record = this.authorizations.get(input.id);
    if (!record || !["pending", "email_sent"].includes(record.status)) return null;
    record.email = input.email;
    record.emailSendCount += 1;
    record.otpHash = input.otpHash;
    record.status = "email_sent";
    record.updatedAt = input.now;
    return { ...record };
  }

  async recordOtpFailure(id: string, now: Date): Promise<void> {
    const record = this.authorizations.get(id);
    if (record) {
      record.otpAttempts += 1;
      record.updatedAt = now;
    }
  }

  async approve(input: { email: string; id: string; now: Date; otpHash: string; userId: string }) {
    const record = this.authorizations.get(input.id);
    if (
      !record ||
      record.status !== "email_sent" ||
      record.email !== input.email ||
      record.otpHash !== input.otpHash ||
      record.otpAttempts >= 8 ||
      record.expiresAt <= input.now
    ) return null;
    record.approvedUserId = input.userId;
    record.status = "approved";
    record.updatedAt = input.now;
    return { ...record };
  }

  async deny(input: { email: string; id: string; now: Date; otpHash: string }): Promise<boolean> {
    const record = this.authorizations.get(input.id);
    if (
      record &&
      record.status === "email_sent" &&
      record.email === input.email &&
      record.otpHash === input.otpHash &&
      record.otpAttempts < 8 &&
      record.expiresAt > input.now
    ) {
      record.status = "denied";
      record.updatedAt = input.now;
      return true;
    }
    return false;
  }

  async consumeBrowser(id: string, now: Date, userId: string): Promise<boolean> {
    const record = this.authorizations.get(id);
    if (
      !record ||
      record.clientType !== "browser" ||
      record.status !== "approved" ||
      record.approvedUserId !== userId ||
      record.expiresAt <= now
    ) return false;
    record.status = "consumed";
    record.updatedAt = now;
    return true;
  }

  async consumeDeviceAuthorization(input: DeviceFamilyIssueInput) {
    const record = this.authorizations.get(input.authorizationId);
    if (
      !record ||
      record.clientType === "browser" ||
      record.status !== "approved" ||
      record.approvedUserId !== input.userId ||
      record.expiresAt <= input.now
    ) return false;
    record.status = "consumed";
    record.updatedAt = input.now;
    this.deviceFamilies.set(input.family.id, {
      absoluteExpiresAt: input.family.absoluteExpiresAt,
      idleExpiresAt: input.family.idleExpiresAt,
      revokedAt: null,
      userId: input.userId,
    });
    this.refreshTokens.set(input.refresh.tokenHash, {
      expiresAt: input.refresh.expiresAt,
      familyId: input.family.id,
      generation: 0,
      replacedByTokenHash: null,
      rotationId: null,
      tokenHash: input.refresh.tokenHash,
      usedAt: null,
    });
    this.tokens.set(input.access.id, {
      expiresAt: input.access.expiresAt,
      kind: "device",
      refreshFamilyId: input.family.id,
      revokedAt: null,
      tokenHash: input.access.tokenHash,
      userId: input.userId,
    });
    return true;
  }

  async rotateDeviceFamily(input: RefreshRotationInput): Promise<RefreshRotationResult> {
    const current = this.refreshTokens.get(input.refreshTokenHash);
    if (!current) return null;
    const family = this.deviceFamilies.get(current.familyId);
    if (!family) return null;
    if (current.usedAt) {
      if (
        current.rotationId === input.rotationId &&
        current.replacedByTokenHash === input.nextRefresh.tokenHash &&
        !family.revokedAt &&
        family.absoluteExpiresAt > input.now &&
        family.idleExpiresAt > input.now
      ) {
        const replacement = this.refreshTokens.get(input.nextRefresh.tokenHash);
        if (
          !replacement ||
          replacement.usedAt ||
          replacement.expiresAt <= input.now
        ) {
          return null;
        }
        const accessExpiresAt = earlier(input.access.expiresAt, family.absoluteExpiresAt);
        this.tokens.set(input.access.id, {
          expiresAt: accessExpiresAt,
          kind: "device",
          refreshFamilyId: current.familyId,
          revokedAt: null,
          tokenHash: input.access.tokenHash,
          userId: family.userId,
        });
        return { accessExpiresAt, refreshExpiresAt: replacement.expiresAt };
      }
      this.revokeFamily(current.familyId, input.now);
      return "reuse";
    }
    if (
      family.revokedAt ||
      current.expiresAt <= input.now ||
      family.absoluteExpiresAt <= input.now ||
      family.idleExpiresAt <= input.now
    ) return null;
    const refreshExpiresAt = earlier(input.nextRefresh.expiresAt, family.absoluteExpiresAt);
    const accessExpiresAt = earlier(input.access.expiresAt, family.absoluteExpiresAt);
    current.usedAt = input.now;
    current.rotationId = input.rotationId;
    current.replacedByTokenHash = input.nextRefresh.tokenHash;
    this.refreshTokens.set(input.nextRefresh.tokenHash, {
      expiresAt: refreshExpiresAt,
      familyId: current.familyId,
      generation: current.generation + 1,
      replacedByTokenHash: null,
      rotationId: null,
      tokenHash: input.nextRefresh.tokenHash,
      usedAt: null,
    });
    family.idleExpiresAt = refreshExpiresAt;
    this.tokens.set(input.access.id, {
      expiresAt: accessExpiresAt,
      kind: "device",
      refreshFamilyId: current.familyId,
      revokedAt: null,
      tokenHash: input.access.tokenHash,
      userId: family.userId,
    });
    return { accessExpiresAt, refreshExpiresAt };
  }

  async createSession(input: { expiresAt: Date; id: string; lastFour: string; now: Date; tokenHash: string; userId: string }): Promise<void> {
    this.tokens.set(input.id, {
      expiresAt: input.expiresAt,
      kind: "web",
      refreshFamilyId: null,
      revokedAt: null,
      tokenHash: input.tokenHash,
      userId: input.userId,
    });
  }

  async notePoll(id: string, now: Date): Promise<void> {
    const record = this.authorizations.get(id);
    if (record) record.lastPolledAt = now;
  }

  async findActiveSession(tokenHash: string, now: Date, kind?: "web" | "device"): Promise<UserRecord | null> {
    const token = [...this.tokens.values()].find((item) => item.tokenHash === tokenHash);
    if (!token || token.revokedAt || token.expiresAt <= now || (kind && token.kind !== kind)) return null;
    if (token.kind === "device") {
      const family = token.refreshFamilyId
        ? this.deviceFamilies.get(token.refreshFamilyId)
        : null;
      if (!family || family.revokedAt || family.absoluteExpiresAt <= now || family.idleExpiresAt <= now) return null;
    }
    return [...this.users.values()].find((user) => user.id === token.userId) ?? null;
  }

  async revokeSession(tokenHash: string, now: Date): Promise<void> {
    const token = [...this.tokens.values()].find((item) => item.tokenHash === tokenHash);
    if (!token) return;
    if (token.kind === "device" && token.refreshFamilyId) {
      this.revokeFamily(token.refreshFamilyId, now);
      return;
    }
    token.revokedAt = now;
  }

  async upsertUser(email: string, now: Date, preferredId?: string): Promise<UserRecord> {
    const existing = this.users.get(email);
    if (existing) return existing;
    const user = { createdAt: now, email, id: preferredId ?? crypto.randomUUID() };
    this.users.set(email, user);
    return user;
  }

  private revokeFamily(familyId: string, now: Date): void {
    const family = this.deviceFamilies.get(familyId);
    if (family && !family.revokedAt) family.revokedAt = now;
    for (const token of this.tokens.values()) {
      if (token.refreshFamilyId === familyId && !token.revokedAt) token.revokedAt = now;
    }
  }
}

function earlier(left: Date, right: Date): Date {
  return left <= right ? left : right;
}
