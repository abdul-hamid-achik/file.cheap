import { describe, expect, test } from "bun:test";

import { requireApiToken } from "@/shared/auth/bearer";

const token = "test-api-token-long-enough";

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
});

function requestWithAuthorization(authorization?: string): Request {
  return new Request("http://127.0.0.1:3100/api/v1/stashes", {
    headers: authorization ? { authorization } : undefined,
  });
}
