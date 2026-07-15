import { describe, expect, test } from "bun:test";

import { CatalogRepository } from "@/features/catalog/catalog";
import { objectKey, SyncService } from "@/features/sync/sync-service";
import type {
  ObjectMetadata,
  ObjectStore,
  TextObject,
  TransferGrant,
} from "@/platform/storage/object-store";
import { PlatformError } from "@/shared/errors/platform-error";

const secret = "test-signing-secret-that-is-long-enough";

describe("SyncService", () => {
  test("plans, commits, lists, and downloads an immutable stash", async () => {
    const store = new MemoryObjectStore();
    const service = new SyncService(store, new CatalogRepository(store), secret);
    const sha256 = "b".repeat(64);
    const input = {
      contentType: "application/vnd.filecheap.stash",
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
      contentType: "application/vnd.filecheap.stash",
      sha256: "c".repeat(64),
      sizeBytes: 10,
      stashId: "same-id",
    });
    store.seedObject(first.object.key, 10);
    await service.commitPlan({ receipt: first.receipt });

    await expect(
      service.createPlan({
        contentType: "application/vnd.filecheap.stash",
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
      contentType: "application/vnd.filecheap.stash",
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
});

class MemoryObjectStore implements ObjectStore {
  readonly driver = "local" as const;
  private readonly objects = new Map<string, ObjectMetadata>();
  private readonly texts = new Map<string, TextObject>();
  private etag = 0;

  seedObject(key: string, sizeBytes: number): void {
    this.objects.set(key, {
      contentType: "application/vnd.filecheap.stash",
      etag: `object-${this.etag += 1}`,
      key,
      sizeBytes,
      uploadedAt: new Date().toISOString(),
    });
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
    const nextEtag = `catalog-${this.etag += 1}`;
    this.texts.set(input.key, { body: input.body, etag: nextEtag });
    return { etag: nextEtag };
  }
}
