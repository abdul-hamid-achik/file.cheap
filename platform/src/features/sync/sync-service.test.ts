import { describe, expect, test } from "bun:test";

import { CatalogRepository } from "@/features/catalog/catalog";
import { stashContentType } from "@/features/sync/contracts";
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
    ).rejects.toMatchObject({ code: "object_not_found", status: 404 });

    store.seedObject(plan.object.key, input.sizeBytes, "7".repeat(64));
    await expect(
      service.createDownload({ stashId: input.stashId }),
    ).rejects.toMatchObject({ code: "integrity_mismatch", status: 422 });
  });
});

class MemoryObjectStore implements ObjectStore {
  readonly driver = "local" as const;
  readonly verification = "server-sha256" as const;
  private readonly objects = new Map<string, ObjectMetadata>();
  private readonly texts = new Map<string, TextObject>();
  private etag = 0;

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

  async inspect(key: string): Promise<ObjectMetadata | null> {
    return this.objects.get(key) ?? null;
  }

  async issueUploadGrant(): Promise<TransferGrant> {
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
