import { promises as filesystem } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { afterEach, describe, expect, test } from "bun:test";

import { CatalogRepository, type CloudStash } from "@/features/catalog/catalog";
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

describe("CatalogRepository concurrency", () => {
  test("preserves commits from independent local store instances", async () => {
    const config = await createConfig();
    const first = new CatalogRepository(new LocalObjectStore(config));
    const second = new CatalogRepository(new LocalObjectStore(config));

    await Promise.all([
      first.commit(stash("first", "a".repeat(64))),
      second.commit(stash("second", "b".repeat(64))),
    ]);

    const stored = await new CatalogRepository(
      new LocalObjectStore(config),
    ).list();
    expect(stored.map((entry) => entry.stashId).sort()).toEqual([
      "first",
      "second",
    ]);
  });
});

async function createConfig(): Promise<PlatformConfig> {
  const directory = await filesystem.mkdtemp(
    join(tmpdir(), "filecheap-catalog-"),
  );
  temporaryDirectories.push(directory);
  return {
    apiToken: "local-development-token",
    dataDirectory: directory,
    publicUrl: "http://127.0.0.1:3100",
    signingSecret: "test-signing-secret-that-is-long-enough",
    storageDriver: "local",
  };
}

function stash(stashId: string, sha256: string): CloudStash {
  return {
    committedAt: "2026-07-15T22:15:00.000Z",
    contentType: "application/vnd.filecheap.stash",
    etag: sha256,
    objectKey: `v1/objects/${sha256}.fcheap`,
    sha256,
    sizeBytes: 1024,
    stashId,
    storageVerification: "server-sha256",
  };
}
