import { describe, expect, test } from "bun:test";

import { InMemoryAuthRepository } from "@/features/auth/repository";
import { AuthService, type AuthMailer } from "@/features/auth/service";
import { PlatformError } from "@/shared/errors/platform-error";

class CapturingMailer implements AuthMailer {
  messages: Parameters<AuthMailer["sendVerification"]>[0][] = [];
  async sendVerification(input: Parameters<AuthMailer["sendVerification"]>[0]) {
    this.messages.push(input);
  }
}

class ScriptedMailer extends CapturingMailer {
  failuresRemaining = 0;

  override async sendVerification(
    input: Parameters<AuthMailer["sendVerification"]>[0],
  ): Promise<void> {
    await super.sendVerification(input);
    if (this.failuresRemaining > 0) {
      this.failuresRemaining -= 1;
      throw new Error("provider rejected delivery");
    }
  }
}

class BlockingMailer extends CapturingMailer {
  private releaseDelivery: (() => void) | undefined;
  private noteStarted!: () => void;
  readonly started = new Promise<void>((resolve) => {
    this.noteStarted = resolve;
  });

  override async sendVerification(
    input: Parameters<AuthMailer["sendVerification"]>[0],
  ): Promise<void> {
    await super.sendVerification(input);
    this.noteStarted();
    await new Promise<void>((resolve) => {
      this.releaseDelivery = resolve;
    });
  }

  accept(): void {
    this.releaseDelivery?.();
  }
}

class FencedAcceptanceRepository extends InMemoryAuthRepository {
  releaseCalls = 0;

  override async acceptVerificationDelivery(): Promise<boolean> {
    return false;
  }

  override async releaseVerificationDelivery(
    input: Parameters<InMemoryAuthRepository["releaseVerificationDelivery"]>[0],
  ): Promise<void> {
    this.releaseCalls += 1;
    await super.releaseVerificationDelivery(input);
  }
}

describe("AuthService", () => {
  test("requires email proof and explicit approval before one-time device token redemption", async () => {
    let now = new Date("2026-07-26T12:00:00.000Z");
    const repository = new InMemoryAuthRepository();
    const mailer = new CapturingMailer();
    const service = new AuthService(repository, mailer, {
      allowedEmails: ["owner@example.com"],
      publicUrl: "https://file.cheap",
      secret: "s".repeat(32),
    }, () => now);

    const authorization = await service.startDeviceAuthorization({
      clientName: "MacBook Pro",
      clientType: "cli",
    });
    expect(authorization.userCode).toMatch(/^[A-Z2-9]{4}-[A-Z2-9]{4}$/);

    await expect(service.poll(authorization.deviceCode)).rejects.toMatchObject({
      code: "authorization_pending",
    });
    now = new Date(now.getTime() + 5_000);
    await service.sendVerification({
      email: "OWNER@example.com",
      userCode: authorization.userCode.replace("-", ""),
    });
    expect(mailer.messages).toHaveLength(1);

    const message = mailer.messages[0]!;
    const approval = await service.decide({
      decision: "approve",
      email: "owner@example.com",
      otp: message.otp,
      userCode: authorization.userCode,
    });
    expect(approval).toMatchObject({ browserSession: false, sessionToken: null });

    now = new Date(now.getTime() + 5_000);
    const token = await service.poll(authorization.deviceCode);
    expect(token.accessToken).toStartWith("fcheap_device_");
    expect(token.expiresIn).toBe(15 * 60);
    expect(token.refreshToken).toStartWith("fcheap_refresh_");
    expect(token.refreshExpiresIn).toBe(30 * 24 * 60 * 60);
    expect(await service.authenticate(token.accessToken, "device")).toMatchObject({ email: "owner@example.com" });
    now = new Date(now.getTime() + 5_000);
    await expect(service.poll(authorization.deviceCode)).rejects.toMatchObject({
      code: "expired_authorization",
    });
  });

  test("rotates refresh tokens idempotently and revokes the family on reuse", async () => {
    let now = new Date("2026-07-26T12:00:00.000Z");
    const repository = new InMemoryAuthRepository();
    const mailer = new CapturingMailer();
    const service = new AuthService(repository, mailer, {
      allowedEmails: ["owner@example.com"],
      publicUrl: "https://file.cheap",
      secret: "s".repeat(32),
    }, () => now);
    const authorization = await service.startDeviceAuthorization({ clientName: "CLI", clientType: "cli" });
    await service.sendVerification({ email: "owner@example.com", userCode: authorization.userCode });
    await service.decide({
      decision: "approve",
      email: "owner@example.com",
      otp: mailer.messages[0]!.otp,
      userCode: authorization.userCode,
    });
    now = new Date(now.getTime() + 5_000);
    const initial = await service.poll(authorization.deviceCode);
    const nextRefreshToken = refreshToken("n");
    const rotation = {
      nextRefreshToken,
      refreshToken: initial.refreshToken,
      rotationId: "r".repeat(22),
    };
    const rotated = await service.refresh(rotation);
    expect(rotated.refreshToken).toBe(nextRefreshToken);
    expect(await service.authenticate(rotated.accessToken, "device")).toMatchObject({ email: "owner@example.com" });

    const replayed = await service.refresh(rotation);
    expect(replayed.refreshToken).toBe(nextRefreshToken);
    expect(await service.authenticate(replayed.accessToken, "device")).toMatchObject({ email: "owner@example.com" });

    const advanced = await service.refresh({
      nextRefreshToken: refreshToken("a"),
      refreshToken: nextRefreshToken,
      rotationId: "a".repeat(22),
    });
    await expect(service.refresh(rotation)).rejects.toMatchObject({ code: "invalid_refresh_token" });

    await expect(service.refresh({
      nextRefreshToken: refreshToken("x"),
      refreshToken: initial.refreshToken,
      rotationId: "z".repeat(22),
    })).rejects.toMatchObject({ code: "refresh_token_reused" });
    await expect(service.authenticate(rotated.accessToken, "device")).rejects.toMatchObject({ code: "unauthorized" });
    await expect(service.authenticate(replayed.accessToken, "device")).rejects.toMatchObject({ code: "unauthorized" });
    await expect(service.authenticate(advanced.accessToken, "device")).rejects.toMatchObject({ code: "unauthorized" });
  });

  test("does not reveal whether an email is allowlisted", async () => {
    const repository = new InMemoryAuthRepository();
    const mailer = new CapturingMailer();
    const service = new AuthService(repository, mailer, {
      allowedEmails: ["owner@example.com"],
      publicUrl: "https://file.cheap",
      secret: "s".repeat(32),
    });
    const authorization = await service.startDeviceAuthorization({ clientName: "CLI", clientType: "cli" });
    await expect(service.sendVerification({ email: "stranger@example.com", userCode: authorization.userCode })).resolves.toBeUndefined();
    expect(mailer.messages).toHaveLength(0);
  });

  test("never upgrades a browser challenge into a device bearer", async () => {
    let now = new Date("2026-07-26T12:00:00.000Z");
    const repository = new InMemoryAuthRepository();
    const mailer = new CapturingMailer();
    const service = new AuthService(repository, mailer, {
      allowedEmails: ["owner@example.com"],
      publicUrl: "https://file.cheap",
      secret: "s".repeat(32),
    }, () => now);
    const authorization = await service.startDeviceAuthorization({ clientName: "Browser", clientType: "browser" });
    await service.sendVerification({ email: "owner@example.com", userCode: authorization.userCode });
    const message = mailer.messages[0]!;
    const approval = await service.decide({ decision: "approve", email: "owner@example.com", otp: message.otp, userCode: authorization.userCode });
    expect(approval).toMatchObject({ browserSession: true });
    expect(await service.authenticate(approval!.sessionToken!, "web")).toMatchObject({ email: "owner@example.com" });
    now = new Date(now.getTime() + 5_000);
    await expect(service.poll(authorization.deviceCode)).rejects.toMatchObject({ code: "expired_authorization" });
  });

  test("rejects a stale OTP after a resend changes the stored proof", async () => {
    const repository = new InMemoryAuthRepository();
    const mailer = new CapturingMailer();
    const service = new AuthService(repository, mailer, {
      allowedEmails: ["owner@example.com"],
      publicUrl: "https://file.cheap",
      secret: "s".repeat(32),
    });
    const authorization = await service.startDeviceAuthorization({ clientName: "CLI", clientType: "cli" });
    await service.sendVerification({ email: "owner@example.com", userCode: authorization.userCode });
    const staleOtp = mailer.messages[0]!.otp;
    await service.sendVerification({ email: "owner@example.com", userCode: authorization.userCode });
    await expect(service.decide({ decision: "approve", email: "owner@example.com", otp: staleOtp, userCode: authorization.userCode })).rejects.toMatchObject({ code: "invalid_verification" });
  });

  test("retries a provider failure with the same derived OTP without consuming a send", async () => {
    const now = new Date("2026-07-26T12:00:00.000Z");
    const repository = new InMemoryAuthRepository();
    const mailer = new ScriptedMailer();
    mailer.failuresRemaining = 1;
    const service = new AuthService(repository, mailer, {
      allowedEmails: ["owner@example.com"],
      publicUrl: "https://file.cheap",
      secret: "s".repeat(32),
    }, () => now);
    const authorization = await service.startDeviceAuthorization({ clientName: "CLI", clientType: "cli" });

    await expect(service.sendVerification({
      email: "owner@example.com",
      userCode: authorization.userCode,
    })).resolves.toBeUndefined();
    const failedState = await repository.findAuthorizationByUserCode(authorization.userCode);
    expect(failedState).toMatchObject({
      email: null,
      emailSendCount: 0,
      otpHash: null,
      status: "pending",
    });
    expect([...repository.verificationDeliveries.values()]).toEqual([
      expect.objectContaining({ status: "pending", acceptedAt: null }),
    ]);

    await service.sendVerification({
      email: "owner@example.com",
      userCode: authorization.userCode,
    });
    expect(mailer.messages).toHaveLength(2);
    expect(mailer.messages[1]).toMatchObject({
      idempotencyKey: mailer.messages[0]!.idempotencyKey,
      otp: mailer.messages[0]!.otp,
    });
    expect(await repository.findAuthorizationByUserCode(authorization.userCode))
      .toMatchObject({ emailSendCount: 1, status: "email_sent" });
  });

  test("keeps the previous OTP active when a resend is rejected by the provider", async () => {
    const repository = new InMemoryAuthRepository();
    const mailer = new ScriptedMailer();
    const service = new AuthService(repository, mailer, {
      allowedEmails: ["owner@example.com"],
      publicUrl: "https://file.cheap",
      secret: "s".repeat(32),
    });
    const authorization = await service.startDeviceAuthorization({ clientName: "CLI", clientType: "cli" });
    await service.sendVerification({ email: "owner@example.com", userCode: authorization.userCode });
    const firstOtp = mailer.messages[0]!.otp;
    const acceptedState = await repository.findAuthorizationByUserCode(authorization.userCode);

    mailer.failuresRemaining = 1;
    await service.sendVerification({ email: "owner@example.com", userCode: authorization.userCode });
    expect(await repository.findAuthorizationByUserCode(authorization.userCode)).toMatchObject({
      emailSendCount: 1,
      otpHash: acceptedState!.otpHash,
      status: "email_sent",
    });
    await expect(service.decide({
      decision: "approve",
      email: "owner@example.com",
      otp: firstOtp,
      userCode: authorization.userCode,
    })).resolves.toMatchObject({ browserSession: false });
  });

  test("coalesces concurrent requests and activates proof only after provider acceptance", async () => {
    const repository = new InMemoryAuthRepository();
    const mailer = new BlockingMailer();
    const service = new AuthService(repository, mailer, {
      allowedEmails: ["owner@example.com"],
      publicUrl: "https://file.cheap",
      secret: "s".repeat(32),
    });
    const authorization = await service.startDeviceAuthorization({ clientName: "CLI", clientType: "cli" });
    const first = service.sendVerification({ email: "owner@example.com", userCode: authorization.userCode });
    await mailer.started;

    await expect(service.sendVerification({
      email: "owner@example.com",
      userCode: authorization.userCode,
    })).resolves.toBeUndefined();
    expect(mailer.messages).toHaveLength(1);
    expect(await repository.findAuthorizationByUserCode(authorization.userCode))
      .toMatchObject({ emailSendCount: 0, otpHash: null, status: "pending" });

    mailer.accept();
    await first;
    expect(await repository.findAuthorizationByUserCode(authorization.userCode))
      .toMatchObject({ emailSendCount: 1, status: "email_sent" });
  });

  test("explicitly releases its fenced lease when provider acceptance cannot be sealed", async () => {
    const repository = new FencedAcceptanceRepository();
    const mailer = new CapturingMailer();
    const service = new AuthService(repository, mailer, {
      allowedEmails: ["owner@example.com"],
      publicUrl: "https://file.cheap",
      secret: "s".repeat(32),
    });
    const authorization = await service.startDeviceAuthorization({
      clientName: "CLI",
      clientType: "cli",
    });
    const prepared = await service.prepareVerification({
      email: "owner@example.com",
      userCode: authorization.userCode,
    });
    expect(prepared).not.toBeNull();

    await service.dispatchVerification(prepared!);

    expect(mailer.messages).toHaveLength(1);
    expect(repository.releaseCalls).toBe(1);
    expect(await repository.findAuthorizationByUserCode(authorization.userCode))
      .toMatchObject({ emailSendCount: 0, otpHash: null, status: "pending" });
    expect([...repository.verificationDeliveries.values()])
      .toEqual([expect.objectContaining({ leaseToken: null, status: "pending" })]);
  });

  test("does not call the provider for an expired authorization", async () => {
    let now = new Date("2026-07-26T12:00:00.000Z");
    const repository = new InMemoryAuthRepository();
    const mailer = new CapturingMailer();
    const service = new AuthService(repository, mailer, {
      allowedEmails: ["owner@example.com"],
      publicUrl: "https://file.cheap",
      secret: "s".repeat(32),
    }, () => now);
    const authorization = await service.startDeviceAuthorization({ clientName: "CLI", clientType: "cli" });
    now = new Date(now.getTime() + 10 * 60 * 1_000);

    await service.sendVerification({ email: "owner@example.com", userCode: authorization.userCode });
    expect(mailer.messages).toHaveLength(0);
    expect(repository.verificationDeliveries.size).toBe(0);
  });

  test("rate limits fast polling and rejects bad OTPs uniformly", async () => {
    let now = new Date("2026-07-26T12:00:00.000Z");
    const repository = new InMemoryAuthRepository();
    const mailer = new CapturingMailer();
    const service = new AuthService(repository, mailer, {
      allowedEmails: ["owner@example.com"],
      publicUrl: "https://file.cheap",
      secret: "s".repeat(32),
    }, () => now);
    const authorization = await service.startDeviceAuthorization({ clientName: "CLI", clientType: "cli" });
    await expect(service.poll(authorization.deviceCode)).rejects.toBeInstanceOf(PlatformError);
    await expect(service.poll(authorization.deviceCode)).rejects.toMatchObject({ code: "rate_limited" });
    now = new Date(now.getTime() + 5_000);
    await service.sendVerification({ email: "owner@example.com", userCode: authorization.userCode });
    await expect(service.decide({ decision: "approve", email: "owner@example.com", otp: "000000", userCode: authorization.userCode })).rejects.toMatchObject({ code: "invalid_verification" });
  });

  test("lists owner devices and revokes a complete device family idempotently", async () => {
    let now = new Date("2026-07-26T12:00:00.000Z");
    const repository = new InMemoryAuthRepository();
    const mailer = new CapturingMailer();
    const service = new AuthService(repository, mailer, {
      allowedEmails: ["owner@example.com"],
      publicUrl: "https://file.cheap",
      secret: "s".repeat(32),
    }, () => now);
    const authorization = await service.startDeviceAuthorization({ clientName: "Build Mac", clientType: "agent" });
    await service.sendVerification({ email: "owner@example.com", userCode: authorization.userCode });
    const approval = await service.decide({
      decision: "approve",
      email: "owner@example.com",
      otp: mailer.messages[0]!.otp,
      userCode: authorization.userCode,
    });
    now = new Date(now.getTime() + 5_000);
    const credentials = await service.poll(authorization.deviceCode);

    const page = await service.listAccessDevices(approval!.userId, { limit: 20 });
    const devices = page.devices;
    expect(devices).toHaveLength(1);
    expect(devices[0]).toMatchObject({
      clientName: "Build Mac",
      lastRefreshedAt: null,
      status: "active",
    });
    expect(page.overview).toEqual({ active: 1, expiring: 0, inactive: 0, total: 1 });
    expect((await service.listAccessDevices("another-owner", { limit: 20 }))).toMatchObject({
      devices: [],
      overview: { active: 0, expiring: 0, inactive: 0, total: 0 },
    });

    now = new Date(now.getTime() + 1_000);
    await service.refresh({
      nextRefreshToken: refreshToken("f"),
      refreshToken: credentials.refreshToken,
      rotationId: "f".repeat(22),
    });
    expect((await service.listAccessDevices(approval!.userId, { limit: 20 })).devices[0])
      .toMatchObject({ lastRefreshedAt: now.toISOString() });

    await expect(service.revokeAccessDevice(devices[0]!.id, "another-owner")).rejects.toMatchObject({
      code: "access_device_not_found",
    });
    await expect(service.revokeAccessDevice(devices[0]!.id, approval!.userId)).resolves.toEqual({
      id: devices[0]!.id,
      status: "revoked",
    });
    await expect(service.revokeAccessDevice(devices[0]!.id, approval!.userId)).resolves.toMatchObject({ status: "revoked" });
    await expect(service.authenticate(credentials.accessToken, "device")).rejects.toMatchObject({ code: "unauthorized" });
    expect((await service.listAccessDevices(approval!.userId, { limit: 20 })).devices[0])
      .toMatchObject({ status: "revoked" });
  });

  test("paginates every owner device with stable createdAt and id cursors", async () => {
    const now = new Date("2026-07-26T12:00:00.000Z");
    const createdAt = new Date("2026-07-20T12:00:00.000Z");
    const repository = new InMemoryAuthRepository();
    const service = new AuthService(repository, new CapturingMailer(), {
      allowedEmails: ["owner@example.com"],
      publicUrl: "https://file.cheap",
      secret: "s".repeat(32),
    }, () => now);
    const ownerIds = Array.from({ length: 55 }, (_, index) =>
      `00000000-0000-4000-8000-${String(index).padStart(12, "0")}`,
    );
    for (const [index, id] of ownerIds.entries()) {
      repository.deviceFamilies.set(id, {
        absoluteExpiresAt: new Date("2026-10-24T12:00:00.000Z"),
        clientName: `Owner device ${index}`,
        createdAt,
        id,
        idleExpiresAt: index === 2
          ? new Date("2026-07-29T12:00:00.000Z")
          : index === 1
            ? new Date("2026-07-25T12:00:00.000Z")
            : new Date("2026-08-25T12:00:00.000Z"),
        lastUsedAt: null,
        revokeReason: index === 0 ? "owner" : null,
        revokedAt: index === 0 ? new Date("2026-07-25T12:00:00.000Z") : null,
        status: index === 0 ? "revoked" : "active",
        updatedAt: createdAt,
        userId: "owner-id",
      });
    }
    for (let index = 0; index < 4; index += 1) {
      const id = `10000000-0000-4000-8000-${String(index).padStart(12, "0")}`;
      repository.deviceFamilies.set(id, {
        absoluteExpiresAt: new Date("2026-10-24T12:00:00.000Z"),
        clientName: `Foreign device ${index}`,
        createdAt: new Date("2026-07-25T12:00:00.000Z"),
        id,
        idleExpiresAt: new Date("2026-08-25T12:00:00.000Z"),
        lastUsedAt: null,
        revokeReason: null,
        revokedAt: null,
        status: "active",
        updatedAt: createdAt,
        userId: "foreign-id",
      });
    }

    const first = await service.listAccessDevices("owner-id", { limit: 50 });
    expect(first.devices).toHaveLength(50);
    expect(first.devices.map((device) => device.id)).toEqual(
      [...ownerIds].sort((left, right) => right.localeCompare(left)).slice(0, 50),
    );
    expect(first.pageInfo).toMatchObject({ hasNextPage: true, limit: 50 });
    expect(first.overview).toEqual({ active: 53, expiring: 1, inactive: 2, total: 55 });

    const second = await service.listAccessDevices("owner-id", {
      cursor: first.pageInfo.endCursor!,
      limit: 50,
    });
    expect(second.devices).toHaveLength(5);
    expect(second.pageInfo.hasNextPage).toBe(false);
    expect(new Set([...first.devices, ...second.devices].map((device) => device.id)).size)
      .toBe(55);
    expect(second.overview).toEqual(first.overview);

    await expect(service.listAccessDevices("foreign-id", {
      cursor: first.pageInfo.endCursor!,
      limit: 50,
    })).rejects.toMatchObject({ code: "invalid_cursor" });
  });
});

function refreshToken(character: string): string {
  return `fcheap_refresh_${character.repeat(43)}`;
}
