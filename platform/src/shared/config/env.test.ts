import { afterEach, describe, expect, test } from "bun:test";

import { getConfig, resetConfigForTests } from "@/shared/config/env";

const managedKeys = [
  "BLOB_READ_WRITE_TOKEN",
  "PLATFORM_API_TOKEN",
  "PLATFORM_BLOB_INTEGRITY",
  "PLATFORM_DATA_DIR",
  "PLATFORM_PUBLIC_URL",
  "PLATFORM_SIGNING_SECRET",
  "PLATFORM_STORAGE_DRIVER",
  "VERCEL",
] as const;

const originalEnvironment = Object.fromEntries(
  managedKeys.map((key) => [key, process.env[key]]),
);

afterEach(() => {
  for (const key of managedKeys) {
    const original = originalEnvironment[key];
    if (original === undefined) {
      delete process.env[key];
    } else {
      process.env[key] = original;
    }
  }
  resetConfigForTests();
});

describe("platform environment", () => {
  test("keeps Vercel Blob disabled without the explicit experimental acknowledgement", () => {
    prepareBlobEnvironment();

    expect(() => getConfig()).toThrow(
      "Vercel Blob direct uploads are presence-only",
    );
  });

  test("allows a controlled Blob spike only with the exact acknowledgement", () => {
    prepareBlobEnvironment();
    process.env.PLATFORM_BLOB_INTEGRITY =
      "presence-size-etag-experimental";

    expect(getConfig()).toMatchObject({
      blobReadWriteToken: "test-blob-token",
      storageDriver: "vercel-blob",
    });
  });
});

function prepareBlobEnvironment(): void {
  delete process.env.VERCEL;
  process.env.BLOB_READ_WRITE_TOKEN = "test-blob-token";
  process.env.PLATFORM_API_TOKEN = "test-api-token-long-enough";
  process.env.PLATFORM_SIGNING_SECRET =
    "test-signing-secret-that-is-long-enough";
  process.env.PLATFORM_STORAGE_DRIVER = "vercel-blob";
  delete process.env.PLATFORM_BLOB_INTEGRITY;
  resetConfigForTests();
}
