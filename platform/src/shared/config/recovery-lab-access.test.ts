import { describe, expect, test } from "bun:test";

import {
  isRecoveryLabEnabled,
  requireRecoveryLabAccess,
} from "@/shared/config/recovery-lab-access";
import { PlatformError } from "@/shared/errors/platform-error";

describe("recovery lab access", () => {
  test("is enabled by default only in local non-production environments", () => {
    expect(isRecoveryLabEnabled({})).toBe(true);
    expect(isRecoveryLabEnabled({ NODE_ENV: "development" })).toBe(true);
    expect(isRecoveryLabEnabled({ NODE_ENV: "test" })).toBe(true);

    expect(isRecoveryLabEnabled({ NODE_ENV: "production" })).toBe(false);
    expect(isRecoveryLabEnabled({ VERCEL: "1" })).toBe(false);
    expect(
      isRecoveryLabEnabled({ NODE_ENV: "development", VERCEL: "1" }),
    ).toBe(false);
  });

  test("honors only exact explicit values and remains fail-closed on Vercel", () => {
    expect(
      isRecoveryLabEnabled({
        NODE_ENV: "production",
        PLATFORM_RECOVERY_LAB_ENABLED: "true",
      }),
    ).toBe(true);
    expect(
      isRecoveryLabEnabled({
        PLATFORM_RECOVERY_LAB_ENABLED: "true",
        VERCEL: "1",
      }),
    ).toBe(true);
    expect(
      isRecoveryLabEnabled({
        NODE_ENV: "production",
        PLATFORM_RECOVERY_LAB_ENABLED: "TRUE",
      }),
    ).toBe(false);
    expect(
      isRecoveryLabEnabled({
        NODE_ENV: "development",
        PLATFORM_RECOVERY_LAB_ENABLED: "false",
      }),
    ).toBe(false);
    expect(
      isRecoveryLabEnabled({
        NODE_ENV: "development",
        PLATFORM_RECOVERY_LAB_ENABLED: "TRUE",
      }),
    ).toBe(false);
  });

  test("returns a typed not-found error when access is disabled", () => {
    expect(() =>
      requireRecoveryLabAccess({ NODE_ENV: "production" }),
    ).toThrow(PlatformError);

    try {
      requireRecoveryLabAccess({ VERCEL: "1" });
      throw new Error("expected recovery lab access to be rejected");
    } catch (error) {
      expect(error).toBeInstanceOf(PlatformError);
      expect(error).toMatchObject({
        code: "route_unavailable",
        status: 404,
      });
    }
  });
});
