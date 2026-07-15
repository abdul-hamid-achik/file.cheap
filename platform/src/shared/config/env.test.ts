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
  "NODE_ENV",
  "VERCEL",
] as const;

const mutableEnvironment = process.env as Record<string, string | undefined>;

const originalEnvironment = Object.fromEntries(
  managedKeys.map((key) => [key, process.env[key]]),
);

afterEach(() => {
  for (const key of managedKeys) {
    const original = originalEnvironment[key];
    if (original === undefined) {
      delete mutableEnvironment[key];
    } else {
      mutableEnvironment[key] = original;
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

  test("rejects development credentials in every production environment", () => {
    delete process.env.VERCEL;
    mutableEnvironment.NODE_ENV = "production";
    process.env.PLATFORM_STORAGE_DRIVER = "local";
    delete process.env.PLATFORM_API_TOKEN;
    delete process.env.PLATFORM_SIGNING_SECRET;
    resetConfigForTests();

    expect(() => getConfig()).toThrow(
      "PLATFORM_API_TOKEN must be replaced in production",
    );

    process.env.PLATFORM_API_TOKEN = "production-api-token-long-enough";
    resetConfigForTests();
    expect(() => getConfig()).toThrow(
      "PLATFORM_SIGNING_SECRET must be replaced in production",
    );
  });

  test("allows explicit non-default credentials in self-hosted production", () => {
    prepareLocalEnvironment();
    mutableEnvironment.NODE_ENV = "production";
    process.env.PLATFORM_API_TOKEN = "production-api-token-long-enough";
    process.env.PLATFORM_SIGNING_SECRET =
      "production-signing-secret-that-is-long-enough";
    resetConfigForTests();

    expect(getConfig()).toMatchObject({
      apiToken: "production-api-token-long-enough",
      storageDriver: "local",
    });
  });

  test("requires https outside loopback in production", () => {
    prepareLocalEnvironment();
    mutableEnvironment.NODE_ENV = "production";
    process.env.PLATFORM_API_TOKEN = "production-api-token-long-enough";
    process.env.PLATFORM_SIGNING_SECRET =
      "production-signing-secret-that-is-long-enough";
    process.env.PLATFORM_PUBLIC_URL = "http://cloud.file.cheap";
    resetConfigForTests();

    expect(() => getConfig()).toThrow(
      "PLATFORM_PUBLIC_URL must use https outside loopback in production",
    );
  });

  test("normalizes an HTTP(S) public origin", () => {
    prepareLocalEnvironment();
    process.env.PLATFORM_PUBLIC_URL = "https://cloud.file.cheap///";
    resetConfigForTests();

    expect(getConfig().publicUrl).toBe("https://cloud.file.cheap");
  });

  test("rejects unsafe or ambiguous public URLs", () => {
    for (const publicUrl of [
      "ftp://cloud.file.cheap",
      "https://user:password@cloud.file.cheap",
      "https://cloud.file.cheap/base",
      "https://cloud.file.cheap?token=secret",
      "https://cloud.file.cheap#fragment",
    ]) {
      prepareLocalEnvironment();
      process.env.PLATFORM_PUBLIC_URL = publicUrl;
      resetConfigForTests();

      expect(() => getConfig()).toThrow("PLATFORM_PUBLIC_URL");
    }
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

function prepareLocalEnvironment(): void {
  delete process.env.VERCEL;
  mutableEnvironment.NODE_ENV = "development";
  process.env.PLATFORM_API_TOKEN = "test-api-token-long-enough";
  process.env.PLATFORM_SIGNING_SECRET =
    "test-signing-secret-that-is-long-enough";
  process.env.PLATFORM_STORAGE_DRIVER = "local";
  delete process.env.PLATFORM_PUBLIC_URL;
  resetConfigForTests();
}
