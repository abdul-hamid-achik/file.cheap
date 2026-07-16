import {
  CatalogRepository,
  type CloudStash,
} from "@/features/catalog/catalog";
import {
  stashContentType,
  type CommitPlanResponse,
  type CommitPlanInput,
  type CreateDownloadInput,
  type CreatePlanInput,
  type DownloadPlan,
  type StashSummary,
  type SyncPlan,
} from "@/features/sync/contracts";
import {
  throwIfStorageOperationAborted,
  type ObjectMetadata,
  type ObjectStore,
} from "@/platform/storage/object-store";
import { PlatformError } from "@/shared/errors/platform-error";
import {
  signPayload,
  verifyPayload,
} from "@/shared/security/signed-token";

const grantLifetimeMilliseconds = 15 * 60 * 1000;

export class SyncService {
  constructor(
    private readonly store: ObjectStore,
    private readonly catalog: CatalogRepository,
    private readonly signingSecret: string,
    private readonly now: () => Date = () => new Date(),
  ) {}

  async createPlan(
    input: CreatePlanInput,
    signal?: AbortSignal,
  ): Promise<SyncPlan> {
    throwIfStorageOperationAborted(signal);
    const existingStash = await this.catalog.findForPlan(input.stashId, signal);
    if (
      existingStash &&
      (existingStash.sha256 !== input.sha256 ||
        existingStash.sizeBytes !== input.sizeBytes)
    ) {
      throw stashConflict(input.stashId);
    }

    const key = objectKey(input.sha256);
    const object = await this.store.inspect(key, signal);
    if (
      existingStash &&
      !object &&
      this.store.verification === "presence-size-etag"
    ) {
      throw committedObjectMissing(existingStash.stashId);
    }
    if (object) {
      assertStoredObjectMatches(
        object,
        {
          contentType: input.contentType,
          etag: existingStash?.etag,
          key,
          sha256: input.sha256,
          sizeBytes: input.sizeBytes,
        },
        this.store.verification,
      );
    }

    const validUntil = new Date(this.now().getTime() + grantLifetimeMilliseconds);
    const receipt = signPayload(
      {
        contentType: input.contentType,
        exp: validUntil.getTime(),
        key,
        kind: "commit",
        sha256: input.sha256,
        sizeBytes: input.sizeBytes,
        stashId: input.stashId,
      },
      this.signingSecret,
    );

    if (existingStash && object) {
      return {
        object: { key, sha256: input.sha256, sizeBytes: input.sizeBytes },
        receipt,
        state: "already_committed",
        upload: null,
        version: "filecheap-sync/1",
      };
    }

    return {
      object: { key, sha256: input.sha256, sizeBytes: input.sizeBytes },
      receipt,
      state: object ? "object_present" : "upload_required",
      upload: object
        ? null
        : await this.store.issueUploadGrant(
            {
              contentType: input.contentType,
              key,
              sha256: input.sha256,
              sizeBytes: input.sizeBytes,
              validUntil,
            },
            signal,
          ),
      version: "filecheap-sync/1",
    };
  }

  async commitPlan(
    input: CommitPlanInput,
    signal?: AbortSignal,
  ): Promise<CommitPlanResponse> {
    throwIfStorageOperationAborted(signal);
    const payload = verifyPayload(input.receipt, this.signingSecret);
    if (payload.kind !== "commit") {
      throw new PlatformError({
        code: "invalid_receipt",
        detail: "The supplied receipt cannot commit an upload plan.",
        status: 400,
        title: "Invalid receipt",
      });
    }

    const existingStash = await this.catalog.find(payload.stashId, signal);
    if (
      existingStash &&
      (existingStash.contentType !== payload.contentType ||
        existingStash.objectKey !== payload.key ||
        existingStash.sha256 !== payload.sha256 ||
        existingStash.sizeBytes !== payload.sizeBytes)
    ) {
      throw stashConflict(payload.stashId);
    }

    const object = await this.store.inspect(payload.key, signal);
    if (!object) {
      throw new PlatformError({
        code: "upload_incomplete",
        detail: "The archive object is not present. Complete the upload first.",
        status: 409,
        title: "Upload incomplete",
      });
    }
    assertStoredObjectMatches(
      object,
      {
        contentType: payload.contentType,
        etag: existingStash?.etag,
        key: payload.key,
        sha256: payload.sha256,
        sizeBytes: payload.sizeBytes,
      },
      this.store.verification,
    );

    const stash = await this.catalog.commit(
      {
        committedAt: this.now().toISOString(),
        contentType: stashContentType,
        etag: existingStash?.etag ?? object.etag,
        objectKey: payload.key,
        sha256: payload.sha256,
        sizeBytes: payload.sizeBytes,
        stashId: payload.stashId,
        storageVerification: this.store.verification,
      },
      signal,
    );

    return {
      requiresFullVerification: true,
      stash: stashSummary(stash),
      version: "filecheap-sync/1",
    };
  }

  async createDownload(
    input: CreateDownloadInput,
    signal?: AbortSignal,
  ): Promise<DownloadPlan> {
    throwIfStorageOperationAborted(signal);
    const stash = await this.catalog.find(input.stashId, signal);
    if (!stash) {
      throw new PlatformError({
        code: "stash_not_found",
        detail: `No committed remote stash exists for ${input.stashId}.`,
        status: 404,
        title: "Stash not found",
      });
    }

    const object = await this.store.inspect(stash.objectKey, signal);
    if (!object) {
      throw committedObjectMissing(stash.stashId);
    }
    assertStoredObjectMatches(
      object,
      {
        contentType: stash.contentType,
        etag: stash.etag,
        key: stash.objectKey,
        sha256: stash.sha256,
        sizeBytes: stash.sizeBytes,
      },
      this.store.verification,
    );

    const validUntil = new Date(this.now().getTime() + grantLifetimeMilliseconds);
    const grant = await this.store.issueDownloadGrant(
      {
        key: stash.objectKey,
        validUntil,
      },
      signal,
    );
    if (grant.method !== "GET") {
      throw new Error("The storage adapter returned a non-GET download grant");
    }
    const downloadGrant = { ...grant, method: "GET" as const };
    return {
      expected: { sha256: stash.sha256, sizeBytes: stash.sizeBytes },
      grant: downloadGrant,
      mustVerifySha256: true,
      stashId: stash.stashId,
      version: "filecheap-sync/1",
    };
  }

  async listStashes(signal?: AbortSignal): Promise<StashSummary[]> {
    return (await this.catalog.list(signal)).map(stashSummary);
  }
}

export function objectKey(sha256: string): string {
  return `v1/workspaces/default/objects/sha256/${sha256.slice(0, 2)}/${sha256}.fcheap`;
}

function stashConflict(stashId: string): PlatformError {
  return new PlatformError({
    code: "stash_conflict",
    detail: `Stash ${stashId} is already committed to different content.`,
    status: 409,
    title: "Stash conflict",
  });
}

function stashSummary(stash: CloudStash): StashSummary {
  return {
    committedAt: stash.committedAt,
    contentType: stash.contentType,
    sha256: stash.sha256,
    sizeBytes: stash.sizeBytes,
    stashId: stash.stashId,
    storageVerification: stash.storageVerification,
  };
}

function assertStoredObjectMatches(
  object: ObjectMetadata,
  expected: {
    contentType: string;
    etag?: string;
    key: string;
    sha256: string;
    sizeBytes: number;
  },
  verification: ObjectStore["verification"],
): void {
  if (object.key !== expected.key || object.contentType !== expected.contentType) {
    throw integrityMismatch(
      "The stored object identity or content type differs from the upload plan.",
    );
  }
  if (object.sizeBytes !== expected.sizeBytes) {
    throw integrityMismatch("The stored object size differs from the upload plan.");
  }
  if (
    verification === "presence-size-etag" &&
    expected.etag &&
    object.etag !== expected.etag
  ) {
    throw integrityMismatch(
      "The stored object ETag differs from the committed catalog evidence.",
    );
  }
  if (
    verification === "server-sha256" &&
    object.verifiedSha256 !== expected.sha256
  ) {
    throw integrityMismatch(
      "The stored object SHA-256 differs from the upload plan.",
    );
  }
}

function committedObjectMissing(stashId: string): PlatformError {
  return new PlatformError({
    code: "committed_object_missing",
    detail: `The committed archive object for ${stashId} is missing. Automatic repair is not safe for this storage adapter.`,
    status: 409,
    title: "Committed object missing",
  });
}

function integrityMismatch(detail: string): PlatformError {
  return new PlatformError({
    code: "integrity_mismatch",
    detail,
    status: 422,
    title: "Integrity mismatch",
  });
}
