import { afterEach, describe, expect, test } from "bun:test";

import { requireApiToken } from "@/shared/auth/bearer";

const token = "test-api-token-long-enough";
const originalLabEnabled = process.env.PLATFORM_RECOVERY_LAB_ENABLED;
const originalVercel = process.env.VERCEL;
const originalVercelEnvironment = process.env.VERCEL_ENV;

afterEach(() => {
  restoreEnvironment(
    "PLATFORM_RECOVERY_LAB_ENABLED",
    originalLabEnabled,
  );
  restoreEnvironment("VERCEL", originalVercel);
  restoreEnvironment("VERCEL_ENV", originalVercelEnvironment);
});

describe("bearer authentication", () => {
  test("accepts a case-insensitive scheme and optional whitespace", () => {
    for (const authorization of [
      `Bearer ${token}`,
      `bearer ${token}`,
      `bEaReR\t${token}`,
      `  Bearer   ${token}  `,
    ]) {
      expect(() =>
        requireApiToken(requestWithAuthorization(authorization), token),
      ).not.toThrow();
    }
  });

  test("rejects missing, malformed, or incorrect credentials", () => {
    for (const authorization of [
      undefined,
      token,
      "Basic dGVzdDp0ZXN0",
      `Bearer ${token} extra`,
      "Bearer wrong-token-long-enough",
    ]) {
      expect(() =>
        requireApiToken(requestWithAuthorization(authorization), token),
      ).toThrow("valid bearer token");
    }
  });

  test("rejects operational API access before reading credentials when the lab is disabled", () => {
    process.env.VERCEL = "1";
    delete process.env.PLATFORM_RECOVERY_LAB_ENABLED;

    expect(() =>
      requireApiToken(
        requestWithAuthorization(`Bearer ${token}`),
        token,
      ),
    ).toThrow("experimental recovery lab is disabled");
  });

  test("rejects operational API access in Vercel Production even when the flag is true", () => {
    process.env.VERCEL = "1";
    process.env.VERCEL_ENV = "production";
    process.env.PLATFORM_RECOVERY_LAB_ENABLED = "true";

    expect(() =>
      requireApiToken(
        requestWithAuthorization(`Bearer ${token}`),
        token,
      ),
    ).toThrow("experimental recovery lab is disabled");
  });
});

function requestWithAuthorization(authorization?: string): Request {
  return new Request("http://127.0.0.1:3100/api/v1/stashes", {
    headers: authorization ? { authorization } : undefined,
  });
}

function restoreEnvironment(name: string, value: string | undefined): void {
  if (value === undefined) {
    delete process.env[name];
  } else {
    process.env[name] = value;
  }
}
