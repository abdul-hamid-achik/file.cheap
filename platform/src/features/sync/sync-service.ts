import type { CloudStash } from "@/features/catalog/catalog";
import { CatalogRepository } from "@/features/catalog/catalog";
import type {
  CommitPlanInput,
  CreateDownloadInput,
  CreatePlanInput,
} from "@/features/sync/contracts";
import type { ObjectStore, TransferGrant } from "@/platform/storage/object-store";
import { PlatformError } from "@/shared/errors/platform-error";
import {
  signPayload,
  verifyPayload,
} from "@/shared/security/signed-token";

const grantLifetimeMilliseconds = 15 * 60 * 1000;

export type SyncPlan = {
  object: {
    key: string;
    sha256: string;
    sizeBytes: number;
  };
  receipt: string;
  state: "already_committed" | "object_present" | "upload_required";
  upload: TransferGrant | null;
  version: "filecheap-sync/1";
};

export class SyncService {
  constructor(
    private readonly store: ObjectStore,
    private readonly catalog: CatalogRepository,
    private readonly signingSecret: string,
    private readonly now: () => Date = () => new Date(),
  ) {}

  async createPlan(input: CreatePlanInput): Promise<SyncPlan> {
    const existingStash = await this.catalog.find(input.stashId);
    if (
      existingStash &&
      (existingStash.sha256 !== input.sha256 ||
        existingStash.sizeBytes !== input.sizeBytes)
    ) {
      throw stashConflict(input.stashId);
    }

    const key = objectKey(input.sha256);
    const object = await this.store.inspect(key);
    if (object && object.sizeBytes !== input.sizeBytes) {
      throw new PlatformError({
        code: "object_conflict",
        detail: "The content-addressed object exists with a different byte size.",
        status: 409,
        title: "Object conflict",
      });
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

    if (existingStash) {
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
        : await this.store.issueUploadGrant({
            contentType: input.contentType,
            key,
            sha256: input.sha256,
            sizeBytes: input.sizeBytes,
            validUntil,
          }),
      version: "filecheap-sync/1",
    };
  }

  async commitPlan(input: CommitPlanInput): Promise<{
    requiresFullVerification: true;
    stash: CloudStash;
    version: "filecheap-sync/1";
  }> {
    const payload = verifyPayload(input.receipt, this.signingSecret);
    if (payload.kind !== "commit") {
      throw new PlatformError({
        code: "invalid_receipt",
        detail: "The supplied receipt cannot commit an upload plan.",
        status: 400,
        title: "Invalid receipt",
      });
    }

    const existingStash = await this.catalog.find(payload.stashId);
    if (existingStash && existingStash.sha256 !== payload.sha256) {
      throw stashConflict(payload.stashId);
    }

    const object = await this.store.inspect(payload.key);
    if (!object) {
      throw new PlatformError({
        code: "upload_incomplete",
        detail: "The archive object is not present. Complete the upload first.",
        status: 409,
        title: "Upload incomplete",
      });
    }
    if (object.sizeBytes !== payload.sizeBytes) {
      throw new PlatformError({
        code: "integrity_mismatch",
        detail: "The stored object size differs from the upload plan.",
        status: 422,
        title: "Integrity mismatch",
      });
    }

    const stash = await this.catalog.commit({
      committedAt: this.now().toISOString(),
      contentType: payload.contentType,
      etag: object.etag,
      objectKey: payload.key,
      sha256: payload.sha256,
      sizeBytes: payload.sizeBytes,
      stashId: payload.stashId,
    });

    return {
      requiresFullVerification: true,
      stash,
      version: "filecheap-sync/1",
    };
  }

  async createDownload(input: CreateDownloadInput): Promise<{
    expected: { sha256: string; sizeBytes: number };
    grant: TransferGrant;
    mustVerifySha256: true;
    stashId: string;
    version: "filecheap-sync/1";
  }> {
    const stash = await this.catalog.find(input.stashId);
    if (!stash) {
      throw new PlatformError({
        code: "stash_not_found",
        detail: `No committed remote stash exists for ${input.stashId}.`,
        status: 404,
        title: "Stash not found",
      });
    }

    const validUntil = new Date(this.now().getTime() + grantLifetimeMilliseconds);
    return {
      expected: { sha256: stash.sha256, sizeBytes: stash.sizeBytes },
      grant: await this.store.issueDownloadGrant({
        key: stash.objectKey,
        validUntil,
      }),
      mustVerifySha256: true,
      stashId: stash.stashId,
      version: "filecheap-sync/1",
    };
  }

  listStashes(): Promise<CloudStash[]> {
    return this.catalog.list();
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
