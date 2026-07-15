import { createHash } from "node:crypto";
import { promises as filesystem } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { afterEach, describe, expect, test } from "bun:test";

import { LocalObjectStore } from "@/platform/storage/local-object-store";
import type { PlatformConfig } from "@/shared/config/env";

const temporaryDirectories: string[] = [];

afterEach(async () => {
  await Promise.all(
    temporaryDirectories.splice(0).map((directory) =>
      filesystem.rm(directory, { force: true, recursive: true }),
    ),
  );
});

describe("LocalObjectStore", () => {
  test("streams a signed immutable upload and download with exact integrity", async () => {
    const store = await createStore();
    const bytes = new TextEncoder().encode("recover me byte for byte\n");
    const sha256 = createHash("sha256").update(bytes).digest("hex");
    const key = `v1/objects/${sha256}.fcheap`;
    const grant = await store.issueUploadGrant({
      contentType: "application/vnd.filecheap.stash",
      key,
      sha256,
      sizeBytes: bytes.byteLength,
      validUntil: new Date(Date.now() + 60_000),
    });
    const uploadUrl = new URL(grant.url);

    const metadata = await store.acceptUpload(
      new Request(grant.url, {
        body: bytes,
        headers: grant.headers,
        method: "PUT",
      }),
      uploadUrl.searchParams.get("key")!,
      uploadUrl.searchParams.get("token")!,
    );
    expect(metadata.etag).toBe(sha256);
    expect(metadata.sizeBytes).toBe(bytes.byteLength);

    const downloadGrant = await store.issueDownloadGrant({
      key,
      validUntil: new Date(Date.now() + 60_000),
    });
    const downloadUrl = new URL(downloadGrant.url);
    const response = await store.serveDownload(
      downloadUrl.searchParams.get("key")!,
      downloadUrl.searchParams.get("token")!,
    );
    expect(new Uint8Array(await response.arrayBuffer())).toEqual(bytes);
  });

  test("rejects bytes that do not match the signed SHA-256", async () => {
    const store = await createStore();
    const bytes = new TextEncoder().encode("actual");
    const grant = await store.issueUploadGrant({
      contentType: "application/vnd.filecheap.stash",
      key: `v1/objects/${"f".repeat(64)}.fcheap`,
      sha256: "f".repeat(64),
      sizeBytes: bytes.byteLength,
      validUntil: new Date(Date.now() + 60_000),
    });
    const uploadUrl = new URL(grant.url);

    await expect(
      store.acceptUpload(
        new Request(grant.url, {
          body: bytes,
          headers: grant.headers,
          method: "PUT",
        }),
        uploadUrl.searchParams.get("key")!,
        uploadUrl.searchParams.get("token")!,
      ),
    ).rejects.toMatchObject({ code: "integrity_mismatch" });
  });

  test("rejects traversal object keys", async () => {
    const store = await createStore();

    await expect(store.inspect("../outside")).rejects.toThrow("Unsafe object key");
  });
});

async function createStore(): Promise<LocalObjectStore> {
  const directory = await filesystem.mkdtemp(join(tmpdir(), "filecheap-platform-"));
  temporaryDirectories.push(directory);
  const config: PlatformConfig = {
    apiToken: "local-development-token",
    dataDirectory: directory,
    publicUrl: "http://127.0.0.1:3100",
    signingSecret: "test-signing-secret-that-is-long-enough",
    storageDriver: "local",
  };
  return new LocalObjectStore(config);
}
