import { createHash } from "node:crypto";
import { promises as filesystem } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { afterEach, describe, expect, test } from "bun:test";

import {
  LocalObjectStore,
  localTransferTokenHeader,
  type LocalObjectStoreOptions,
} from "@/platform/storage/local-object-store";
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
      grant.headers[localTransferTokenHeader]!,
    );
    expect(uploadUrl.searchParams.has("token")).toBe(false);
    expect(metadata.etag).toBe(sha256);
    expect(metadata.sizeBytes).toBe(bytes.byteLength);

    const downloadGrant = await store.issueDownloadGrant({
      key,
      validUntil: new Date(Date.now() + 60_000),
    });
    const downloadUrl = new URL(downloadGrant.url);
    const response = await store.serveDownload(
      downloadUrl.searchParams.get("key")!,
      downloadGrant.headers[localTransferTokenHeader]!,
    );
    expect(downloadUrl.searchParams.has("token")).toBe(false);
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
        grant.headers[localTransferTokenHeader]!,
      ),
    ).rejects.toMatchObject({ code: "integrity_mismatch" });
  });

  test("rejects traversal object keys", async () => {
    const store = await createStore();

    for (const key of ["../outside", "v1/../outside", "v1/./outside", "v1//outside"]) {
      await expect(store.inspect(key)).rejects.toThrow("Unsafe object key");
    }
  });

  test("does not start a local storage read for an already canceled request", async () => {
    const store = await createStore();
    const controller = new AbortController();
    controller.abort();

    await expect(
      store.inspect(`v1/objects/${"a".repeat(64)}.fcheap`, controller.signal),
    ).rejects.toMatchObject({ code: "request_aborted", status: 408 });
  });

  test("does not hash an existing upload target after the retry request is canceled", async () => {
    const { config, store } = await createStoreWithConfig();
    const bytes = new TextEncoder().encode("already stored");
    const sha256 = createHash("sha256").update(bytes).digest("hex");
    const key = `v1/objects/${sha256}.fcheap`;
    const path = join(config.dataDirectory, "objects", ...key.split("/"));
    await filesystem.mkdir(join(path, ".."), { recursive: true });
    await filesystem.writeFile(path, bytes);
    const grant = await store.issueUploadGrant({
      contentType: "application/vnd.filecheap.stash",
      key,
      sha256,
      sizeBytes: bytes.byteLength,
      validUntil: new Date(Date.now() + 60_000),
    });
    const controller = new AbortController();
    controller.abort();
    const request = new Request(grant.url, {
      body: bytes,
      headers: grant.headers,
      method: "PUT",
      signal: controller.signal,
    });

    await expect(
      store.acceptUpload(
        request,
        key,
        grant.headers[localTransferTokenHeader]!,
      ),
    ).rejects.toMatchObject({ code: "request_aborted", status: 408 });
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
        grant.headers[localTransferTokenHeader]!,
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

  test("cancels an in-flight upload and removes its temporary bytes", async () => {
    const { config, store } = await createStoreWithConfig();
    const bytes = new TextEncoder().encode("eventually");
    const sha256 = createHash("sha256").update(bytes).digest("hex");
    const key = `v1/objects/${sha256}.fcheap`;
    const grant = await store.issueUploadGrant({
      contentType: "application/vnd.filecheap.stash",
      key,
      sha256,
      sizeBytes: bytes.byteLength,
      validUntil: new Date(Date.now() + 60_000),
    });
    const controller = new AbortController();
    let transferStarted!: () => void;
    const started = new Promise<void>((resolve) => {
      transferStarted = resolve;
    });
    let closeTimer: ReturnType<typeof setTimeout> | undefined;
    let sentFirstChunk = false;
    const request = new Request(grant.url, {
      body: new ReadableStream<Uint8Array>({
        cancel() {
          clearTimeout(closeTimer);
        },
        pull(streamController) {
          if (!sentFirstChunk) {
            sentFirstChunk = true;
            streamController.enqueue(bytes.slice(0, 1));
            transferStarted();
            return;
          }
          closeTimer ??= setTimeout(() => streamController.close(), 1_000);
        },
      }),
      duplex: "half",
      headers: grant.headers,
      method: "PUT",
      signal: controller.signal,
    } as RequestInit & { duplex: "half" });
    const upload = store.acceptUpload(
      request,
      key,
      grant.headers[localTransferTokenHeader]!,
    );

    await started;
    controller.abort();

    await expect(upload).rejects.toMatchObject({
      code: "upload_canceled",
      status: 408,
    });
    expect(await store.inspect(key)).toBeNull();
    await expectNoTemporaryFiles(config);
  });

  test("expires an upload that does not finish within its signed grant", async () => {
    const { config, store } = await createStoreWithConfig();
    const bytes = new TextEncoder().encode("too late");
    const sha256 = createHash("sha256").update(bytes).digest("hex");
    const key = `v1/objects/${sha256}.fcheap`;
    const grant = await store.issueUploadGrant({
      contentType: "application/vnd.filecheap.stash",
      key,
      sha256,
      sizeBytes: bytes.byteLength,
      validUntil: new Date(Date.now() + 50),
    });
    const request = new Request(grant.url, {
      body: new ReadableStream<Uint8Array>({ start() {} }),
      duplex: "half",
      headers: grant.headers,
      method: "PUT",
    } as RequestInit & { duplex: "half" });

    await expect(
      store.acceptUpload(
        request,
        key,
        grant.headers[localTransferTokenHeader]!,
      ),
    ).rejects.toMatchObject({ code: "expired_grant", status: 410 });
    expect(await store.inspect(key)).toBeNull();
    await expectNoTemporaryFiles(config);
  });

  test("enforces the server upload deadline independently of grant expiry", async () => {
    const { config, store } = await createStoreWithConfig({
      uploadDeadlineMilliseconds: 25,
    });
    const bytes = new TextEncoder().encode("stalled upload");
    const sha256 = createHash("sha256").update(bytes).digest("hex");
    const key = `v1/objects/${sha256}.fcheap`;
    const grant = await store.issueUploadGrant({
      contentType: "application/vnd.filecheap.stash",
      key,
      sha256,
      sizeBytes: bytes.byteLength,
      validUntil: new Date(Date.now() + 60_000),
    });
    const request = new Request(grant.url, {
      body: new ReadableStream<Uint8Array>({ start() {} }),
      duplex: "half",
      headers: grant.headers,
      method: "PUT",
    } as RequestInit & { duplex: "half" });

    await expect(
      store.acceptUpload(
        request,
        key,
        grant.headers[localTransferTokenHeader]!,
      ),
    ).rejects.toMatchObject({ code: "upload_timeout", status: 408 });
    expect(await store.inspect(key)).toBeNull();
    await expectNoTemporaryFiles(config);
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

  test("returns catalog bytes and their ETag from one opened file", async () => {
    const { config, store } = await createStoreWithConfig({
      testHooks: {
        afterOpen: async ({ operation, path }) => {
          if (operation === "readText") {
            await filesystem.rename(`${path}.replacement`, path);
          }
        },
      },
    });
    const key = "v1/workspaces/default/catalog/same-inode.json";
    await store.writeText({ body: "original", key });
    const path = join(config.dataDirectory, "objects", ...key.split("/"));
    const replacementPath = `${path}.replacement`;
    await filesystem.writeFile(replacementPath, "replacement");

    const stored = await store.readText(key);
    expect(stored?.body).toBe("original");
    expect(stored?.etag).toBe(
      createHash("sha256").update("original").digest("hex"),
    );
    expect(await filesystem.readFile(path, "utf8")).toBe("replacement");
  });

  test("streams download bytes and ETag from one opened file", async () => {
    const { config, store } = await createStoreWithConfig({
      testHooks: {
        afterOpen: async ({ operation, path }) => {
          if (operation === "download") {
            await filesystem.rename(`${path}.replacement`, path);
          }
        },
      },
    });
    const original = new TextEncoder().encode("original download");
    const replacement = new TextEncoder().encode("replacement bytes");
    const sha256 = createHash("sha256").update(original).digest("hex");
    const key = `v1/objects/${sha256}.fcheap`;
    const path = join(config.dataDirectory, "objects", ...key.split("/"));
    await filesystem.mkdir(join(path, ".."), { recursive: true });
    await filesystem.writeFile(path, original);
    const replacementPath = `${path}.replacement`;
    await filesystem.writeFile(replacementPath, replacement);
    const grant = await store.issueDownloadGrant({
      key,
      validUntil: new Date(Date.now() + 60_000),
    });

    const response = await store.serveDownload(
      key,
      grant.headers[localTransferTokenHeader]!,
    );
    const streamed = new Uint8Array(await response.arrayBuffer());
    const responseHash = createHash("sha256").update(streamed).digest("hex");
    expect(response.headers.get("etag")).toBe(`"${responseHash}"`);
    expect(Number(response.headers.get("content-length"))).toBe(streamed.byteLength);
    expect(streamed).toEqual(original);
    expect(new Uint8Array(await filesystem.readFile(path))).toEqual(replacement);
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

  test("removes only the recovered dead owner's matching candidate link", async () => {
    const { config, store } = await createStoreWithConfig();
    const key = "v1/workspaces/default/catalog/stale-candidate.json";
    const path = join(config.dataDirectory, "objects", ...key.split("/"));
    const lockPath = `${path}.lock`;
    const token = "00000000-0000-4000-8000-000000000001";
    const candidatePath = `${lockPath}.candidate.${token}`;
    await filesystem.mkdir(join(path, ".."), { recursive: true });
    await writeDeadLock(lockPath, token);
    await filesystem.link(lockPath, candidatePath);

    await expect(store.writeText({ body: "recovered", key })).resolves.toBeDefined();
    await expect(filesystem.lstat(candidatePath)).rejects.toMatchObject({
      code: "ENOENT",
    });
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
    const recoveryPath = `${lockPath}.recovery.0000000000000000-2147483647-00000000-0000-4000-8000-000000000002`;
    await filesystem.mkdir(join(path, ".."), { recursive: true });
    await writeDeadLock(lockPath);
    await filesystem.writeFile(recoveryPath, "");

    await expect(store.writeText({ body: "recovered", key })).resolves.toBeDefined();
    await expect(filesystem.stat(recoveryPath)).rejects.toMatchObject({ code: "ENOENT" });
  });

  test("preserves malformed recovery claims while recovering the canonical lock", async () => {
    const { config, store } = await createStoreWithConfig();
    const key = "v1/workspaces/default/catalog/malformed-recovery.json";
    const path = join(config.dataDirectory, "objects", ...key.split("/"));
    const lockPath = `${path}.lock`;
    const malformedClaim = `${lockPath}.recovery.0000000000000000-2147483647-unknown`;
    await filesystem.mkdir(join(path, ".."), { recursive: true });
    await writeDeadLock(lockPath);
    await filesystem.writeFile(malformedClaim, "preserve");

    await expect(store.writeText({ body: "recovered", key })).resolves.toBeDefined();
    expect(await filesystem.readFile(malformedClaim, "utf8")).toBe("preserve");
  });

  test("cancels while waiting behind another abandoned-lock recoverer", async () => {
    const { config, store } = await createStoreWithConfig();
    const key = "v1/workspaces/default/catalog/canceled-recovery.json";
    const path = join(config.dataDirectory, "objects", ...key.split("/"));
    const lockPath = `${path}.lock`;
    const activeClaim = `${lockPath}.recovery.0000000000000000-${process.pid}-00000000-0000-4000-8000-000000000003`;
    await filesystem.mkdir(join(path, ".."), { recursive: true });
    await writeDeadLock(lockPath);
    await filesystem.writeFile(activeClaim, "");
    const controller = new AbortController();
    const write = store.writeText(
      { body: "must-not-commit", key },
      controller.signal,
    );

    await waitForRecoveryClaimCount(join(path, ".."), 2);
    controller.abort();

    await expect(write).rejects.toMatchObject({
      code: "request_aborted",
      status: 408,
    });
    expect(await filesystem.lstat(lockPath)).toBeDefined();
    expect(await filesystem.lstat(activeClaim)).toBeDefined();
    expect(await store.readText(key)).toBeNull();
  });

  test("fails fast without touching malformed catalog locks or catalog bytes", async () => {
    const malformedOwners = [
      "",
      "{not-json",
      `${JSON.stringify({ pid: 2_147_483_647, token: "bad-version", version: 2 })}\n`,
      `${JSON.stringify({ pid: 2_147_483_648, token: "oversized-pid", version: 1 })}\n`,
    ];

    for (const [index, malformedOwner] of malformedOwners.entries()) {
      const { config, store } = await createStoreWithConfig();
      const key = `v1/workspaces/default/catalog/malformed-${index}.json`;
      const initial = await store.writeText({ body: `preserve-${index}`, key });
      const path = join(config.dataDirectory, "objects", ...key.split("/"));
      const lockPath = `${path}.lock`;
      await filesystem.writeFile(lockPath, malformedOwner);
      const startedAt = performance.now();

      await expect(
        store.writeText({
          body: `replace-${index}`,
          expectedEtag: initial.etag,
          key,
        }),
      ).rejects.toMatchObject({ code: "catalog_lock_invalid", status: 503 });

      expect(performance.now() - startedAt).toBeLessThan(1_000);
      expect(await filesystem.readFile(lockPath, "utf8")).toBe(malformedOwner);
      expect((await store.readText(key))?.body).toBe(`preserve-${index}`);
    }
  });

  test("fails fast and preserves unsafe catalog lock directory entries", async () => {
    for (const kind of ["broken-symlink", "directory", "oversized"] as const) {
      const { config, store } = await createStoreWithConfig();
      const key = `v1/workspaces/default/catalog/unsafe-${kind}.json`;
      const initial = await store.writeText({ body: `preserve-${kind}`, key });
      const path = join(config.dataDirectory, "objects", ...key.split("/"));
      const lockPath = `${path}.lock`;
      if (kind === "broken-symlink") {
        await filesystem.symlink(`${lockPath}.missing-target`, lockPath);
      } else if (kind === "directory") {
        await filesystem.mkdir(lockPath);
      } else {
        await filesystem.writeFile(lockPath, "x".repeat(4 * 1024 + 1));
      }
      const startedAt = performance.now();

      await expect(
        store.writeText({
          body: "must-not-commit",
          expectedEtag: initial.etag,
          key,
        }),
      ).rejects.toMatchObject({ code: "catalog_lock_invalid", status: 503 });
      expect(performance.now() - startedAt).toBeLessThan(1_000);
      expect((await store.readText(key))?.body).toBe(`preserve-${kind}`);
      expect(await filesystem.lstat(lockPath)).toBeDefined();
    }
  });
});

async function writeDeadLock(
  path: string,
  token = "00000000-0000-4000-8000-000000000000",
): Promise<void> {
  await filesystem.writeFile(
    path,
    `${JSON.stringify({
      pid: 2_147_483_647,
      token,
      version: 1,
    })}\n`,
  );
}

async function expectNoTemporaryFiles(config: PlatformConfig): Promise<void> {
  let remainingFiles: string[];
  try {
    remainingFiles = (
      await filesystem.readdir(join(config.dataDirectory, "objects"), {
        recursive: true,
      })
    ).map(String);
  } catch (error) {
    if (error instanceof Error && "code" in error && error.code === "ENOENT") {
      return;
    }
    throw error;
  }
  expect(remainingFiles.some((entry) => entry.endsWith(".tmp"))).toBe(
    false,
  );
}

async function waitForRecoveryClaimCount(
  directory: string,
  expected: number,
): Promise<void> {
  const deadline = Date.now() + 1_000;
  while (Date.now() < deadline) {
    const entries = await filesystem.readdir(directory);
    if (entries.filter((entry) => entry.includes(".recovery.")).length >= expected) {
      return;
    }
    await new Promise((resolve) => setTimeout(resolve, 5));
  }
  throw new Error("Timed out waiting for recovery claims");
}

async function createStore(): Promise<LocalObjectStore> {
  return (await createStoreWithConfig()).store;
}

async function createStoreWithConfig(
  options?: LocalObjectStoreOptions,
): Promise<{
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
  return { config, store: new LocalObjectStore(config, options) };
}
