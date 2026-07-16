import { describe, expect, test } from "bun:test";

import { CatalogRepository } from "@/features/catalog/catalog";
import {
  protocolV1MaximumCatalogEntries,
  stashContentType,
} from "@/features/sync/contracts";
import { objectKey, SyncService } from "@/features/sync/sync-service";
import type {
  ObjectMetadata,
  ObjectStore,
  TextObject,
  TransferGrant,
} from "@/platform/storage/object-store";
import {
  CatalogPreconditionError,
  PlatformError,
} from "@/shared/errors/platform-error";

const secret = "test-signing-secret-that-is-long-enough";

describe("SyncService", () => {
  test("plans, commits, lists, and downloads an immutable stash", async () => {
    const store = new MemoryObjectStore();
    const service = new SyncService(store, new CatalogRepository(store), secret);
    const sha256 = "b".repeat(64);
    const input = {
      contentType: stashContentType,
      sha256,
      sizeBytes: 128,
      stashId: "trace-01",
    };

    const plan = await service.createPlan(input);
    expect(plan.state).toBe("upload_required");
    expect(plan.upload?.method).toBe("PUT");
    expect(plan.object.key).toBe(objectKey(sha256));

    store.seedObject(plan.object.key, 128);
    const committed = await service.commitPlan({ receipt: plan.receipt });
    expect(committed.requiresFullVerification).toBe(true);
    expect(committed.stash.sha256).toBe(sha256);
    expect(committed.stash.storageVerification).toBe("server-sha256");
    expect("etag" in committed.stash).toBe(false);
    expect("objectKey" in committed.stash).toBe(false);
    expect(await service.listStashes()).toHaveLength(1);

    const repeatedPlan = await service.createPlan(input);
    expect(repeatedPlan.state).toBe("already_committed");
    expect(repeatedPlan.upload).toBeNull();

    const download = await service.createDownload({ stashId: "trace-01" });
    expect(download.mustVerifySha256).toBe(true);
    expect(download.expected).toEqual({ sha256, sizeBytes: 128 });
    expect(download.grant.method).toBe("GET");
  });

  test("rejects rebinding a stash ID to different content", async () => {
    const store = new MemoryObjectStore();
    const service = new SyncService(store, new CatalogRepository(store), secret);
    const first = await service.createPlan({
      contentType: stashContentType,
      sha256: "c".repeat(64),
      sizeBytes: 10,
      stashId: "same-id",
    });
    store.seedObject(first.object.key, 10);
    await service.commitPlan({ receipt: first.receipt });

    await expect(
      service.createPlan({
        contentType: stashContentType,
        sha256: "d".repeat(64),
        sizeBytes: 10,
        stashId: "same-id",
      }),
    ).rejects.toBeInstanceOf(PlatformError);
  });

  test("will not commit a missing or incorrectly sized upload", async () => {
    const store = new MemoryObjectStore();
    const service = new SyncService(store, new CatalogRepository(store), secret);
    const plan = await service.createPlan({
      contentType: stashContentType,
      sha256: "e".repeat(64),
      sizeBytes: 25,
      stashId: "incomplete",
    });

    await expect(service.commitPlan({ receipt: plan.receipt })).rejects.toMatchObject({
      code: "upload_incomplete",
    });

    store.seedObject(plan.object.key, 24);
    await expect(service.commitPlan({ receipt: plan.receipt })).rejects.toMatchObject({
      code: "integrity_mismatch",
    });
  });

  test("will not trust same-size bytes with the wrong verified SHA-256", async () => {
    const store = new MemoryObjectStore();
    const service = new SyncService(store, new CatalogRepository(store), secret);
    const sha256 = "f".repeat(64);
    const input = {
      contentType: stashContentType,
      sha256,
      sizeBytes: 4,
      stashId: "poisoned",
    };
    const plan = await service.createPlan(input);
    store.seedObject(plan.object.key, 4, "0".repeat(64));

    await expect(service.commitPlan({ receipt: plan.receipt })).rejects.toMatchObject({
      code: "integrity_mismatch",
    });
    await expect(service.createPlan(input)).rejects.toMatchObject({
      code: "integrity_mismatch",
    });
  });

  test("repairs a committed catalog entry whose object is missing", async () => {
    const store = new MemoryObjectStore();
    const service = new SyncService(store, new CatalogRepository(store), secret);
    const input = {
      contentType: stashContentType,
      sha256: "9".repeat(64),
      sizeBytes: 16,
      stashId: "repair-me",
    };
    const first = await service.createPlan(input);
    store.seedObject(first.object.key, input.sizeBytes);
    await service.commitPlan({ receipt: first.receipt });
    store.removeObject(first.object.key);

    const repair = await service.createPlan(input);
    expect(repair.state).toBe("upload_required");
    expect(repair.upload?.method).toBe("PUT");
    store.seedObject(repair.object.key, input.sizeBytes);
    await expect(
      service.commitPlan({ receipt: repair.receipt }),
    ).resolves.toMatchObject({ stash: { stashId: input.stashId } });
    await expect(service.createPlan(input)).resolves.toMatchObject({
      state: "already_committed",
    });
  });

  test("fails closed when an ETag-only adapter loses a committed object", async () => {
    const store = new MemoryObjectStore("presence-size-etag");
    const service = new SyncService(store, new CatalogRepository(store), secret);
    const input = {
      contentType: stashContentType,
      sha256: "6".repeat(64),
      sizeBytes: 16,
      stashId: "blob-repair-needs-verification",
    };
    const plan = await service.createPlan(input);
    store.seedObject(plan.object.key, input.sizeBytes);
    await service.commitPlan({ receipt: plan.receipt });
    store.removeObject(plan.object.key);

    await expect(service.createPlan(input)).rejects.toMatchObject({
      code: "committed_object_missing",
      status: 409,
    });
    await expect(
      service.createDownload({ stashId: input.stashId }),
    ).rejects.toMatchObject({ code: "committed_object_missing", status: 409 });
  });

  test("does not issue a download grant for missing or corrupted object bytes", async () => {
    const store = new MemoryObjectStore();
    const service = new SyncService(store, new CatalogRepository(store), secret);
    const input = {
      contentType: stashContentType,
      sha256: "8".repeat(64),
      sizeBytes: 16,
      stashId: "unavailable-download",
    };
    const plan = await service.createPlan(input);
    store.seedObject(plan.object.key, input.sizeBytes);
    await service.commitPlan({ receipt: plan.receipt });

    store.removeObject(plan.object.key);
    await expect(
      service.createDownload({ stashId: input.stashId }),
    ).rejects.toMatchObject({ code: "committed_object_missing", status: 409 });

    store.seedObject(plan.object.key, input.sizeBytes, "7".repeat(64));
    await expect(
      service.createDownload({ stashId: input.stashId }),
    ).rejects.toMatchObject({ code: "integrity_mismatch", status: 422 });
  });

  test("rejects changed ETag evidence and unexpected object metadata", async () => {
    const store = new MemoryObjectStore("presence-size-etag");
    const service = new SyncService(store, new CatalogRepository(store), secret);
    const input = {
      contentType: stashContentType,
      sha256: "7".repeat(64),
      sizeBytes: 16,
      stashId: "changed-evidence",
    };
    const plan = await service.createPlan(input);
    store.seedObject(plan.object.key, input.sizeBytes);
    await service.commitPlan({ receipt: plan.receipt });

    store.changeObject(plan.object.key, { etag: "replacement-etag" });
    await expect(service.createPlan(input)).rejects.toMatchObject({
      code: "integrity_mismatch",
      status: 422,
    });
    await expect(service.commitPlan({ receipt: plan.receipt })).rejects.toMatchObject({
      code: "integrity_mismatch",
      status: 422,
    });
    await expect(
      service.createDownload({ stashId: input.stashId }),
    ).rejects.toMatchObject({ code: "integrity_mismatch", status: 422 });

    store.changeObject(plan.object.key, {
      contentType: "application/octet-stream",
    });
    await expect(service.createPlan(input)).rejects.toMatchObject({
      code: "integrity_mismatch",
      status: 422,
    });
  });

  test("rejects a new plan before issuing an upload grant when the catalog is full", async () => {
    const store = new MemoryObjectStore();
    const entries = Object.fromEntries(
      Array.from({ length: protocolV1MaximumCatalogEntries }, (_, index) => {
        const stashId = `stash-${index}`;
        const sha256 = index.toString(16).padStart(64, "0");
        return [
          stashId,
          {
            committedAt: "2026-07-15T22:15:00.000Z",
            contentType: stashContentType,
            etag: sha256,
            objectKey: objectKey(sha256),
            sha256,
            sizeBytes: 1,
            stashId,
            storageVerification: "server-sha256",
          },
        ];
      }),
    );
    store.seedCatalog(
      JSON.stringify({
        revision: protocolV1MaximumCatalogEntries,
        schemaVersion: 1,
        stashes: entries,
        updatedAt: "2026-07-15T22:15:00.000Z",
      }),
    );
    const service = new SyncService(store, new CatalogRepository(store), secret);

    await expect(
      service.createPlan({
        contentType: stashContentType,
        sha256: "a".repeat(64),
        sizeBytes: 1,
        stashId: "overflow",
      }),
    ).rejects.toMatchObject({
      code: "catalog_capacity_reached",
      status: 409,
    });
    expect(store.uploadGrantsIssued).toBe(0);
  });
});

class MemoryObjectStore implements ObjectStore {
  readonly driver = "local" as const;
  private readonly objects = new Map<string, ObjectMetadata>();
  private readonly texts = new Map<string, TextObject>();
  private etag = 0;
  uploadGrantsIssued = 0;

  constructor(
    readonly verification: ObjectStore["verification"] = "server-sha256",
  ) {}

  seedObject(key: string, sizeBytes: number, verifiedSha256 = hashFromKey(key)): void {
    this.objects.set(key, {
      contentType: stashContentType,
      etag: `object-${this.etag += 1}`,
      key,
      sizeBytes,
      uploadedAt: new Date().toISOString(),
      verifiedSha256,
    });
  }

  removeObject(key: string): void {
    this.objects.delete(key);
  }

  changeObject(key: string, changes: Partial<ObjectMetadata>): void {
    const current = this.objects.get(key);
    if (!current) throw new Error(`Missing test object: ${key}`);
    this.objects.set(key, { ...current, ...changes });
  }

  async inspect(key: string): Promise<ObjectMetadata | null> {
    return this.objects.get(key) ?? null;
  }

  async issueUploadGrant(): Promise<TransferGrant> {
    this.uploadGrantsIssued += 1;
    return {
      expiresAt: new Date(Date.now() + 60_000).toISOString(),
      headers: { "content-type": "application/vnd.filecheap.stash" },
      method: "PUT",
      url: "http://127.0.0.1/upload",
    };
  }

  async issueDownloadGrant(): Promise<TransferGrant> {
    return {
      expiresAt: new Date(Date.now() + 60_000).toISOString(),
      headers: {},
      method: "GET",
      url: "http://127.0.0.1/download",
    };
  }

  async readText(key: string): Promise<TextObject | null> {
    return this.texts.get(key) ?? null;
  }

  seedCatalog(body: string): void {
    this.texts.set("v1/workspaces/default/catalog/v1.json", {
      body,
      etag: "catalog-seed",
    });
  }

  async writeText(input: {
    body: string;
    expectedEtag?: string;
    key: string;
  }): Promise<{ etag: string }> {
    const current = this.texts.get(input.key);
    if (
      (input.expectedEtag && current?.etag !== input.expectedEtag) ||
      (!input.expectedEtag && current)
    ) {
      throw new CatalogPreconditionError();
    }
    const nextEtag = `catalog-${this.etag += 1}`;
    this.texts.set(input.key, { body: input.body, etag: nextEtag });
    return { etag: nextEtag };
  }
}

function hashFromKey(key: string): string {
  const matched = key.match(/([a-f0-9]{64})\.fcheap$/);
  if (!matched) throw new Error(`Object key does not contain a SHA-256: ${key}`);
  return matched[1];
}
