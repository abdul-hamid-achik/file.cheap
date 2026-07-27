import { describe, expect, test } from "bun:test";

import {
  handleVerificationEmailRequest,
  type VerificationEmailRouteDependencies,
} from "@/app/api/console/auth/verification-emails/route";
import type { PreparedVerificationDelivery } from "@/features/auth/service";
import { PlatformError } from "@/shared/errors/platform-error";

describe("verification email route", () => {
  test.each([
    ["unknown authorization", null, async () => undefined],
    ["email outside the allowlist", null, async () => undefined],
    ["provider failure", preparedClaim(), async () => {
      throw new Error("provider unavailable");
    }],
  ])("returns the same delayed 202 for %s", async (_label, prepared, dispatch) => {
    const deferred: Array<() => Promise<void>> = [];
    const rateLimits: unknown[] = [];
    const startedAt = performance.now();
    const response = await handleVerificationEmailRequest(
      verificationRequest(),
      dependencies({
        defer: (task) => deferred.push(task),
        dispatch,
        enforceRateLimit: async (input) => {
          rateLimits.push(input);
        },
        prepare: async () => prepared,
        responseFloorMs: 25,
      }),
    );
    const elapsedMs = performance.now() - startedAt;

    expect(response.status).toBe(202);
    expect(await response.json()).toEqual({ status: "verification_requested" });
    expect(elapsedMs).toBeGreaterThanOrEqual(20);
    expect(rateLimits).toEqual([
      expect.objectContaining({
        action: "verification-email-ip",
        key: "request",
        limit: 12,
      }),
    ]);
    expect(deferred).toHaveLength(prepared ? 1 : 0);
    if (deferred[0]) await expect(deferred[0]()).resolves.toBeUndefined();
  });

  test("keeps request rate-limit failures public and does not schedule delivery", async () => {
    const deferred: Array<() => Promise<void>> = [];
    const response = await handleVerificationEmailRequest(
      verificationRequest(),
      dependencies({
        defer: (task) => deferred.push(task),
        enforceRateLimit: async () => {
          throw new PlatformError({
            code: "rate_limited",
            detail: "Too many requests.",
            retryAfterSeconds: 60,
            status: 429,
            title: "Rate limited",
          });
        },
        responseFloorMs: 1,
      }),
    );

    expect(response.status).toBe(429);
    expect(await response.json()).toMatchObject({ code: "rate_limited" });
    expect(deferred).toHaveLength(0);
  });

  test("false account and authorization requests do not consume global owner buckets", async () => {
    const deferred: Array<() => Promise<void>> = [];
    const prepared = preparedClaim();
    const preparedInputs: Array<{ email: string; userCode: string }> = [];
    const rateLimitActions: string[] = [];
    const requestBodies = [
      { email: "owner@example.com", userCode: "FAKE-0001" },
      { email: "stranger@example.com", userCode: "ABCD-EFGH" },
      { email: "owner@example.com", userCode: "FAKE-0002" },
      { email: "owner@example.com", userCode: "ABCD-EFGH" },
    ];

    for (const body of requestBodies) {
      const response = await handleVerificationEmailRequest(
        verificationRequest(body),
        dependencies({
          defer: (task) => deferred.push(task),
          enforceRateLimit: async (input) => {
            rateLimitActions.push(input.action);
          },
          prepare: async (input) => {
            preparedInputs.push(input);
            return input.email === "owner@example.com" && input.userCode === "ABCD-EFGH"
              ? prepared
              : null;
          },
        }),
      );
      expect(response.status).toBe(202);
    }

    expect(preparedInputs).toEqual(requestBodies);
    expect(rateLimitActions).toEqual(
      Array.from({ length: requestBodies.length }, () => "verification-email-ip"),
    );
    expect(deferred).toHaveLength(1);
  });
});

function verificationRequest(
  body: { email: string; userCode: string } = {
    email: "owner@example.com",
    userCode: "ABCD-EFGH",
  },
): Request {
  return new Request("https://file.cheap/api/console/auth/verification-emails", {
    body: JSON.stringify(body),
    headers: {
      "content-type": "application/json",
      origin: "https://file.cheap",
      "sec-fetch-site": "same-origin",
    },
    method: "POST",
  });
}

function dependencies(
  overrides: Partial<VerificationEmailRouteDependencies> = {},
): VerificationEmailRouteDependencies {
  return {
    defer: () => undefined,
    dispatch: async () => undefined,
    enforceRateLimit: async () => undefined,
    prepare: async () => null,
    responseFloorMs: 1,
    ...overrides,
  };
}

function preparedClaim(): PreparedVerificationDelivery {
  return Object.freeze({}) as PreparedVerificationDelivery;
}
