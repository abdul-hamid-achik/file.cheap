import { promises as filesystem } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { afterEach, describe, expect, test } from "bun:test";

import { CatalogRepository, type CloudStash } from "@/features/catalog/catalog";
import { protocolV1MaximumCatalogEntries } from "@/features/sync/contracts";
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

  test("stops CAS retries when the request is canceled", async () => {
    const controller = new AbortController();
    let observedSignal: AbortSignal | undefined;
    const repository = new CatalogRepository(
      new AlwaysConflictingObjectStore(),
      "v1/test/canceled-catalog.json",
      {
        deadlineMilliseconds: 15_000,
        delay: async (_attempt, signal) => {
          observedSignal = signal;
          controller.abort();
        },
        now: () => 0,
      },
    );

    await expect(
      repository.commit(
        stash("canceled", "c".repeat(64)),
        controller.signal,
      ),
    ).rejects.toMatchObject({ code: "request_aborted", status: 408 });
    expect(observedSignal).toBe(controller.signal);
  });

  test("cancels a queued commit without breaking serialization", async () => {
    const store = new BlockingWriteObjectStore();
    const repository = new CatalogRepository(store);
    const first = repository.commit(stash("first", "1".repeat(64)));
    await store.writeStarted;

    const controller = new AbortController();
    const queued = repository.commit(
      stash("queued", "2".repeat(64)),
      controller.signal,
    );
    controller.abort();

    await expect(queued).rejects.toMatchObject({
      code: "request_aborted",
      status: 408,
    });
    expect(store.readCount).toBe(1);

    store.releaseWrite();
    await first;
    await Promise.resolve();
    expect(store.readCount).toBe(1);
  });

  test("fails closed at the bounded protocol-v1 catalog capacity", async () => {
    const fullCatalog = catalogWithEntries(protocolV1MaximumCatalogEntries);
    const store = new SeededCatalogObjectStore(fullCatalog);
    const repository = new CatalogRepository(store);

    await expect(repository.list()).resolves.toHaveLength(
      protocolV1MaximumCatalogEntries,
    );
    await expect(
      repository.commit(stash("overflow", "f".repeat(64))),
    ).rejects.toMatchObject({
      code: "catalog_capacity_reached",
      status: 409,
    });
    expect(store.writeCount).toBe(0);

    const overCapacity = new SeededCatalogObjectStore(
      catalogWithEntries(protocolV1MaximumCatalogEntries + 1),
    );
    await expect(new CatalogRepository(overCapacity).list()).rejects.toBeDefined();
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

  test("fails closed on structurally corrupt persisted catalogs", async () => {
    const config = await createConfig();
    const store = new LocalObjectStore(config);
    const key = "v1/test/corrupt-catalog.json";
    const validStash = stash("embedded-id", "e".repeat(64));
    const corruptCatalogs = [
      {
        revision: 1,
        schemaVersion: 1,
        stashes: { "record-id": validStash },
        updatedAt: "2026-07-15T22:15:00.000Z",
      },
      {
        revision: 1,
        schemaVersion: 1,
        stashes: { "embedded-id": { ...validStash, unexpected: true } },
        updatedAt: "2026-07-15T22:15:00.000Z",
      },
      {
        revision: 1,
        schemaVersion: 1,
        stashes: { "embedded-id": { ...validStash, sizeBytes: 0 } },
        updatedAt: "2026-07-15T22:15:00.000Z",
      },
    ];

    for (const [index, catalog] of corruptCatalogs.entries()) {
      const catalogKey = `${key}.${index}`;
      await store.writeText({
        body: JSON.stringify(catalog),
        key: catalogKey,
      });
      await expect(
        new CatalogRepository(store, catalogKey).list(),
      ).rejects.toBeDefined();
    }
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

class BlockingWriteObjectStore implements ObjectStore {
  readonly driver = "local" as const;
  readonly verification = "server-sha256" as const;
  readCount = 0;
  private resolveWrite!: () => void;
  readonly writeStarted = new Promise<void>((resolve) => {
    this.resolveWrite = resolve;
  });
  private unblockWrite!: () => void;
  private readonly writeBlocked = new Promise<void>((resolve) => {
    this.unblockWrite = resolve;
  });

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
    this.readCount += 1;
    return null;
  }

  async writeText(): Promise<{ etag: string }> {
    this.resolveWrite();
    await this.writeBlocked;
    return { etag: "written" };
  }

  releaseWrite(): void {
    this.unblockWrite();
  }
}

class SeededCatalogObjectStore implements ObjectStore {
  readonly driver = "local" as const;
  readonly verification = "server-sha256" as const;
  writeCount = 0;

  constructor(private readonly body: string) {}

  async inspect(): Promise<null> {
    return null;
  }

  async issueUploadGrant(): Promise<never> {
    throw new Error("not used");
  }

  async issueDownloadGrant(): Promise<never> {
    throw new Error("not used");
  }

  async readText(): Promise<{ body: string; etag: string }> {
    return { body: this.body, etag: "seed" };
  }

  async writeText(): Promise<{ etag: string }> {
    this.writeCount += 1;
    return { etag: "written" };
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

function catalogWithEntries(count: number): string {
  const stashes = Object.fromEntries(
    Array.from({ length: count }, (_, index) => {
      const stashId = `stash-${index}`;
      return [
        stashId,
        stash(stashId, index.toString(16).padStart(64, "0")),
      ];
    }),
  );
  return JSON.stringify({
    revision: count,
    schemaVersion: 1,
    stashes,
    updatedAt: "2026-07-15T22:15:00.000Z",
  });
}
