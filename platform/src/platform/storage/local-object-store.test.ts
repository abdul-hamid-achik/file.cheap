import { createHash } from "node:crypto";
import { promises as filesystem } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { afterEach, describe, expect, test } from "bun:test";

import { LocalObjectStore } from "@/platform/storage/local-object-store";
import type { PlatformConfig } from "@/shared/config/env";
import { CatalogPreconditionError } from "@/shared/errors/platform-error";

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
    expect(response.headers.get("etag")).toBe(`"${sha256}"`);
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

    for (const key of ["../outside", "v1/../outside", "v1/./outside", "v1//outside"]) {
      await expect(store.inspect(key)).rejects.toThrow("Unsafe object key");
    }
  });

  test("rejects an oversized body with a typed error and removes temporary bytes", async () => {
    const { config, store } = await createStoreWithConfig();
    const signedBytes = new TextEncoder().encode("a");
    const uploadedBytes = new TextEncoder().encode("ab");
    const sha256 = createHash("sha256").update(signedBytes).digest("hex");
    const key = `v1/objects/${sha256}.fcheap`;
    const grant = await store.issueUploadGrant({
      contentType: "application/vnd.filecheap.stash",
      key,
      sha256,
      sizeBytes: signedBytes.byteLength,
      validUntil: new Date(Date.now() + 60_000),
    });
    const uploadUrl = new URL(grant.url);

    await expect(
      store.acceptUpload(
        new Request(grant.url, {
          body: uploadedBytes,
          headers: grant.headers,
          method: "PUT",
        }),
        uploadUrl.searchParams.get("key")!,
        uploadUrl.searchParams.get("token")!,
      ),
    ).rejects.toMatchObject({ code: "upload_too_large", status: 413 });

    expect(await store.inspect(key)).toBeNull();
    const remainingFiles = await filesystem.readdir(
      join(config.dataDirectory, "objects"),
      { recursive: true },
    );
    expect(remainingFiles.map(String).some((entry) => entry.endsWith(".tmp"))).toBe(
      false,
    );
  });

  test("enforces compare-and-swap across store instances", async () => {
    const { config, store: firstStore } = await createStoreWithConfig();
    const secondStore = new LocalObjectStore(config);
    const key = "v1/workspaces/default/catalog/concurrency.json";
    const initial = await firstStore.writeText({ body: "initial", key });

    const results = await Promise.allSettled([
      firstStore.writeText({ body: "first", expectedEtag: initial.etag, key }),
      secondStore.writeText({ body: "second", expectedEtag: initial.etag, key }),
    ]);

    expect(results.filter((result) => result.status === "fulfilled")).toHaveLength(1);
    const rejected = results.find((result) => result.status === "rejected");
    expect(rejected).toBeDefined();
    if (rejected?.status === "rejected") {
      expect(rejected.reason).toBeInstanceOf(CatalogPreconditionError);
    }
  });

  test("recovers a catalog lock left by a terminated process", async () => {
    const { config, store } = await createStoreWithConfig();
    const key = "v1/workspaces/default/catalog/stale-lock.json";
    const path = join(config.dataDirectory, "objects", ...key.split("/"));
    const lockPath = `${path}.lock`;
    await filesystem.mkdir(join(path, ".."), { recursive: true });
    await writeDeadLock(lockPath);

    await expect(store.writeText({ body: "recovered", key })).resolves.toBeDefined();
    await expect(filesystem.stat(lockPath)).rejects.toMatchObject({ code: "ENOENT" });
  });

  test("serializes two processes recovering the same abandoned lock", async () => {
    const { config, store: firstStore } = await createStoreWithConfig();
    const secondStore = new LocalObjectStore(config);
    const key = "v1/workspaces/default/catalog/stale-race.json";
    const initial = await firstStore.writeText({ body: "initial", key });
    const path = join(config.dataDirectory, "objects", ...key.split("/"));
    const lockPath = `${path}.lock`;
    await writeDeadLock(lockPath);

    const results = await Promise.allSettled([
      firstStore.writeText({ body: "first", expectedEtag: initial.etag, key }),
      secondStore.writeText({ body: "second", expectedEtag: initial.etag, key }),
    ]);

    expect(results.filter((result) => result.status === "fulfilled")).toHaveLength(1);
    expect(results.filter((result) => result.status === "rejected")).toHaveLength(1);
    expect(await firstStore.readText(key)).not.toBeNull();
  });

  test("ignores an abandoned recovery claim from a terminated recoverer", async () => {
    const { config, store } = await createStoreWithConfig();
    const key = "v1/workspaces/default/catalog/stale-recovery.json";
    const path = join(config.dataDirectory, "objects", ...key.split("/"));
    const lockPath = `${path}.lock`;
    const recoveryPath = `${lockPath}.recovery.0000000000000000-2147483647-dead`;
    await filesystem.mkdir(join(path, ".."), { recursive: true });
    await writeDeadLock(lockPath);
    await filesystem.writeFile(recoveryPath, "");

    await expect(store.writeText({ body: "recovered", key })).resolves.toBeDefined();
    await expect(filesystem.stat(recoveryPath)).rejects.toMatchObject({ code: "ENOENT" });
  });
});

async function writeDeadLock(path: string): Promise<void> {
  await filesystem.writeFile(
    path,
    `${JSON.stringify({
      pid: 2_147_483_647,
      token: "terminated-test-process",
      version: 1,
    })}\n`,
  );
}

async function createStore(): Promise<LocalObjectStore> {
  return (await createStoreWithConfig()).store;
}

async function createStoreWithConfig(): Promise<{
  config: PlatformConfig;
  store: LocalObjectStore;
}> {
  const directory = await filesystem.mkdtemp(join(tmpdir(), "filecheap-platform-"));
  temporaryDirectories.push(directory);
  const config: PlatformConfig = {
    apiToken: "local-development-token",
    dataDirectory: directory,
    publicUrl: "http://127.0.0.1:3100",
    signingSecret: "test-signing-secret-that-is-long-enough",
    storageDriver: "local",
  };
  return { config, store: new LocalObjectStore(config) };
}
