import { promises as filesystem } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { afterEach, describe, expect, test } from "bun:test";

import { CatalogRepository, type CloudStash } from "@/features/catalog/catalog";
import { LocalObjectStore } from "@/platform/storage/local-object-store";
import type { ObjectStore } from "@/platform/storage/object-store";
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

  test(
    "preserves one hundred simultaneous commits without exhausting CAS retries",
    async () => {
      const config = await createConfig();
      const count = 100;
      const repositories = Array.from(
        { length: count },
        () => new CatalogRepository(new LocalObjectStore(config)),
      );

      await Promise.all(
        repositories.map((repository, index) =>
          repository.commit(
            stash(`stash-${index}`, index.toString(16).padStart(64, "0")),
          ),
        ),
      );

      const stored = await new CatalogRepository(
        new LocalObjectStore(config),
      ).list();
      expect(stored).toHaveLength(count);
      expect(new Set(stored.map((entry) => entry.stashId)).size).toBe(count);
    },
    30_000,
  );

  test("returns a typed retryable error when the CAS deadline is exhausted", async () => {
    let now = 0;
    const repository = new CatalogRepository(
      new AlwaysConflictingObjectStore(),
      "v1/test/catalog.json",
      {
        deadlineMilliseconds: 2,
        delay: async () => {
          now += 1;
        },
        now: () => now,
      },
    );

    await expect(
      repository.commit(stash("busy", "f".repeat(64))),
    ).rejects.toMatchObject({
      code: "catalog_busy",
      status: 503,
    });
  });

  test("rejects an idempotent commit whose stored identity differs", async () => {
    const config = await createConfig();
    const repository = new CatalogRepository(new LocalObjectStore(config));
    const original = stash("identity", "d".repeat(64));
    await repository.commit(original);

    await expect(
      repository.commit({
        ...original,
        etag: "different-etag",
        objectKey: "v1/objects/different.fcheap",
      }),
    ).rejects.toMatchObject({ code: "stash_conflict", status: 409 });
  });
});

class AlwaysConflictingObjectStore implements ObjectStore {
  readonly driver = "local" as const;
  readonly verification = "server-sha256" as const;

  async inspect(): Promise<null> {
    return null;
  }

  async issueUploadGrant(): Promise<never> {
    throw new Error("not used");
  }

  async issueDownloadGrant(): Promise<never> {
    throw new Error("not used");
  }

  async readText(): Promise<null> {
    return null;
  }

  async writeText(): Promise<never> {
    throw new CatalogPreconditionError();
  }
}

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
