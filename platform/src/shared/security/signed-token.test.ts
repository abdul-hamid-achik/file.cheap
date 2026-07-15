import { describe, expect, test } from "bun:test";

import { PlatformError } from "@/shared/errors/platform-error";
import { signPayload, verifyPayload } from "@/shared/security/signed-token";

const secret = "test-signing-secret-that-is-long-enough";

describe("signed transfer tokens", () => {
  test("round-trips an exact upload scope", () => {
    const payload = {
      contentType: "application/octet-stream",
      exp: Date.now() + 60_000,
      key: "v1/objects/archive.fcheap",
      kind: "upload" as const,
      sha256: "a".repeat(64),
      sizeBytes: 42,
    };

    expect(verifyPayload(signPayload(payload, secret), secret)).toEqual(payload);
  });

  test("rejects tampering", () => {
    const token = signPayload(
      {
        exp: Date.now() + 60_000,
        key: "v1/objects/archive.fcheap",
        kind: "download",
      },
      secret,
    );
    const tampered = `${token.slice(0, -1)}${token.endsWith("a") ? "b" : "a"}`;

    expect(() => verifyPayload(tampered, secret)).toThrow(PlatformError);
  });

  test("rejects expired grants", () => {
    const token = signPayload(
      {
        exp: Date.now() - 1,
        key: "v1/objects/archive.fcheap",
        kind: "download",
      },
      secret,
    );

    expect(() => verifyPayload(token, secret)).toThrow("expired");
  });
});
