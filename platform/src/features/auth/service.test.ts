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
    expect(approval?.sessionToken).toStartWith("fcheap_web_");
    expect(await service.authenticate(approval!.sessionToken, "web")).toMatchObject({ email: "owner@example.com" });
    await expect(service.authenticate(approval!.sessionToken, "device")).rejects.toMatchObject({ code: "unauthorized" });

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
    expect(await service.authenticate(approval!.sessionToken, "web")).toMatchObject({ email: "owner@example.com" });
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
});

function refreshToken(character: string): string {
  return `fcheap_refresh_${character.repeat(43)}`;
}
