import { describe, expect, test } from "bun:test";

import {
  type BlobSdk,
  VercelBlobStore,
} from "@/platform/storage/vercel-blob-store";
import type { PlatformConfig } from "@/shared/config/env";

const config: PlatformConfig = {
  apiToken: "local-development-token",
  blobReadWriteToken: "test-blob-token",
  dataDirectory: "/unused",
  publicUrl: "http://127.0.0.1:3100",
  signingSecret: "test-signing-secret-that-is-long-enough",
  storageDriver: "vercel-blob",
};

describe("VercelBlobStore signed grants", () => {
  test("scopes an upload to one exact immutable pathname and size", async () => {
    const calls: Array<Record<string, unknown>> = [];
    const blob = grantOnlyBlobSdk(calls);
    const store = new VercelBlobStore(config, blob);
    expect(store.verification).toBe("presence-size-etag");
    const key = `v1/vaults/test/objects/${"a".repeat(64)}.age`;

    const grant = await store.issueUploadGrant({
      contentType: "application/octet-stream",
      key,
      sha256: "a".repeat(64),
      sizeBytes: 1024,
      validUntil: new Date("2026-07-15T23:00:00.000Z"),
    });

    expect(grant).toMatchObject({
      headers: { "content-type": "application/octet-stream" },
      method: "PUT",
      url: "https://example.private.blob.vercel-storage.com/signed",
    });
    expect(calls[0]).toMatchObject({
      allowedContentTypes: ["application/octet-stream"],
      maximumSizeInBytes: 1024,
      operations: ["put"],
      pathname: key,
    });
    expect(calls[1]).toMatchObject({
      addRandomSuffix: false,
      allowOverwrite: false,
      maximumSizeInBytes: 1024,
      operation: "put",
      pathname: key,
    });
    expect(calls.flatMap(Object.values)).not.toContain("*");
  });

  test("scopes a download to one exact GET pathname", async () => {
    const calls: Array<Record<string, unknown>> = [];
    const store = new VercelBlobStore(config, grantOnlyBlobSdk(calls));
    const key = `v1/vaults/test/objects/${"b".repeat(64)}.age`;

    const grant = await store.issueDownloadGrant({
      key,
      validUntil: new Date("2026-07-15T23:00:00.000Z"),
    });

    expect(grant.method).toBe("GET");
    expect(calls[0]).toMatchObject({ operations: ["get"], pathname: key });
    expect(calls[1]).toMatchObject({ operation: "get", pathname: key });
    expect(calls.flatMap(Object.values)).not.toContain("*");
  });
});

function grantOnlyBlobSdk(calls: Array<Record<string, unknown>>): BlobSdk {
  return {
    get: async () => null,
    head: async () => {
      throw new Error("not used");
    },
    issueSignedToken: async (options) => {
      calls.push(options as Record<string, unknown>);
      return {
        clientSigningToken: "client-signing-token",
        delegationToken: "delegation-token",
        validUntil: options.validUntil ?? Date.now() + 60_000,
      };
    },
    presignUrl: async (_token, options) => {
      calls.push(options as unknown as Record<string, unknown>);
      return {
        presignedUrl: "https://example.private.blob.vercel-storage.com/signed",
      };
    },
    put: async () => {
      throw new Error("not used");
    },
  } as BlobSdk;
}
