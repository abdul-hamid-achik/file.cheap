import { createHash, randomUUID } from "node:crypto";

import type { ArtifactDownloadInput, ArtifactDownloadResponse, ArtifactListQuery, ArtifactPlanInput, ArtifactPlanReplayResponse, ArtifactPlanResponse, ArtifactPlanResult, ArtifactSummary } from "@/features/artifacts/contracts";
import type { ArtifactRecord, ArtifactRepository, RetainableArtifactState } from "@/platform/database/repository";
import type { ArtifactObjectStore } from "@/platform/artifacts/object-store";
import { PlatformError } from "@/shared/errors/platform-error";

const grantLifetimeMilliseconds = 15 * 60 * 1000;
const deletionLeaseMilliseconds = 15 * 60 * 1000;

type ArtifactIngestPolicy = Readonly<{
  kinds: readonly string[];
  maxSizeBytes: number;
  nativeSchemas: readonly string[];
  producerTool: string;
}>;

/**
 * Reject an artifact that exceeds the authenticated producer's own quota. The
 * detail names the producer and its exact quota so an operator can act on the
 * response without reading the keyring.
 */
export function assertProducerSizeQuota(
  sizeBytes: number,
  policy: Readonly<{ maxSizeBytes: number; producerTool: string }>,
): void {
  if (sizeBytes > policy.maxSizeBytes) {
    throw new PlatformError({
      code: "producer_quota_exceeded",
      detail: `producer '${policy.producerTool}' allows up to ${policy.maxSizeBytes} bytes; this artifact declares ${sizeBytes} bytes.`,
      status: 413,
      title: "Artifact exceeds the producer quota",
    });
  }
}

export class ArtifactService {
  constructor(private readonly store: ArtifactObjectStore, private readonly repository: ArtifactRepository, private readonly now: () => Date = () => new Date()) {}

  async plan(input: ArtifactPlanInput, signal?: AbortSignal, ownerAccountId?: string): Promise<ArtifactPlanResult> {
    const now = this.now();
    const artifactId = `art_${input.idempotencyKey.replaceAll("-", "")}`;
    const retentionExpiresAt = input.expiresAt
      ? new Date(input.expiresAt)
      : null;
    if (retentionExpiresAt !== null && retentionExpiresAt <= now) {
      throw retentionExpired();
    }
    const planExpiresAt = new Date(
      Math.min(
        now.getTime() + grantLifetimeMilliseconds,
        retentionExpiresAt?.getTime() ?? Number.POSITIVE_INFINITY,
      ),
    );
    const runIndexSha256 = input.runIndex ? digestRunIndex(input.runIndex) : null;
    let record = await this.repository.find(artifactId);
    if (record && !samePlan(record, input, ownerAccountId)) {
      throw new PlatformError({ code: "idempotency_conflict", detail: "The idempotency key is already bound to different artifact metadata or content.", status: 409, title: "Idempotency conflict" });
    }
    if (record?.state === "committed") {
      return committedPlanResult(record);
    }
    if (record?.state === "deleted" && record.committedAt === null) {
      record = await this.repository.restartDeletedPlan(artifactId, { now, planExpiresAt, planToken: randomUUID() });
    }
    if (record?.state === "deleting") {
      throw new PlatformError({ code: "idempotency_reconciling", detail: "The expired upload plan is being reconciled. Retry the same request.", retryAfterSeconds: 2, status: 409, title: "Upload plan reconciling" });
    }
    if (record && record.state !== "planned") {
      throw new PlatformError({ code: "idempotency_unavailable", detail: "The idempotency key identifies an artifact under retention or already deleted.", status: 409, title: "Artifact unavailable" });
    }
    if (!record) {
      record = await this.repository.createPlan(input, { artifactId, objectKey: objectKey(artifactId, input.sha256), ownerAccountId, planExpiresAt, planToken: randomUUID(), runIndexSha256, now });
    } else if (record.planExpiresAt <= now) {
      record = await this.repository.renewPlan(artifactId, record.planToken, { now, planExpiresAt });
    }
    if (!record) {
      throw new PlatformError({ code: "idempotency_reconciling", detail: "The upload plan could not be loaded safely. Retry the same request.", retryAfterSeconds: 2, status: 409, title: "Upload plan reconciling" });
    }
    if (!samePlan(record, input, ownerAccountId)) {
      throw new PlatformError({ code: "idempotency_conflict", detail: "The idempotency key is already bound to different artifact metadata or content.", status: 409, title: "Idempotency conflict" });
    }
    if (record.state === "committed") {
      return committedPlanResult(record);
    }
    const grantNow = this.now();
    if (record.expiresAt !== null && record.expiresAt <= grantNow) {
      throw retentionExpired();
    }
    if (record.state !== "planned" || record.planExpiresAt <= grantNow) {
      throw new PlatformError({ code: "idempotency_reconciling", detail: "The upload plan could not be renewed safely. Retry the same request.", retryAfterSeconds: 2, status: 409, title: "Upload plan reconciling" });
    }
    const upload = await this.store.issueUploadGrant({ contentType: record.contentType, key: record.objectKey, sizeBytes: record.sizeBytes, validUntil: record.planExpiresAt }, signal);
    return plannedPlanResult(record, { ...upload, method: "PUT" });
  }

  async commit(
    receipt: string,
    signal?: AbortSignal,
    ingestPolicy?: ArtifactIngestPolicy,
  ): Promise<ArtifactSummary> {
    const record = await this.repository.findByPlanToken(receipt);
    if (!record) throw invalidReceipt();
    if (
      ingestPolicy &&
      !matchesArtifactPolicy(record, ingestPolicy)
    ) {
      throw invalidReceipt();
    }
    if (record.state === "deleted" || record.state === "deleting") throw invalidReceipt();
    // Re-check the quota here as well as at plan time: a keyring that was
    // tightened after the plan was issued must not be able to commit.
    if (ingestPolicy) assertProducerSizeQuota(record.sizeBytes, ingestPolicy);
    const now = this.now();
    if (
      (record.state === "planned" && record.planExpiresAt <= now) ||
      (record.expiresAt !== null && record.expiresAt <= now)
    ) {
      throw invalidReceipt();
    }
    const object = await this.store.inspect(record.objectKey, signal);
    if (!object) throw new PlatformError({ code: "upload_incomplete", detail: "The direct upload is not present.", status: 409, title: "Upload incomplete" });
    if (object.sizeBytes !== record.sizeBytes || object.contentType !== record.contentType) throw new PlatformError({ code: "integrity_mismatch", detail: "The stored object does not match the planned size or content type.", status: 422, title: "Artifact mismatch" });
    // The HEAD above already bound size and content type, so the streamed
    // digest is bounded by the planned size and never buffers the object.
    const verified = await this.store.verifySha256(record.objectKey, record.sha256, record.sizeBytes, signal);
    if (!verified) throw new PlatformError({ code: "integrity_mismatch", detail: "The stored object does not match the declared SHA-256.", status: 422, title: "Artifact mismatch" });
    const committed = await this.repository.markCommitted(record.artifactId, object.etag, this.now());
    if (!committed) {
      throw new PlatformError({ code: "commit_conflict", detail: "The upload plan expired or entered retention before it could be committed. Retry the plan request with the same idempotency key.", status: 409, title: "Commit conflict" });
    }
    return summary(committed);
  }

  async download(
    input: ArtifactDownloadInput,
    signal?: AbortSignal,
    accessPolicy?: ArtifactIngestPolicy,
    ownerAccountId?: string,
  ): Promise<ArtifactDownloadResponse> {
    const now = this.now();
    const record = await this.requireCommitted(input.artifactId, ownerAccountId);
    if (
      (record.expiresAt !== null && record.expiresAt <= now) ||
      (accessPolicy && !matchesArtifactPolicy(record, accessPolicy))
    ) {
      throw artifactNotFound();
    }
    const object = await this.store.inspect(record.objectKey, signal);
    if (!object) throw new PlatformError({ code: "artifact_object_missing", detail: "The committed artifact object is unavailable.", status: 409, title: "Artifact unavailable" });
    const grantExpiresAt = new Date(
      Math.min(
        now.getTime() + (ownerAccountId ? 60 * 1_000 : grantLifetimeMilliseconds),
        record.expiresAt?.getTime() ?? Number.POSITIVE_INFINITY,
      ),
    );
    const grant = await this.store.issueDownloadGrant({ key: record.objectKey, validUntil: grantExpiresAt }, signal);
    return { ...summary(record), download: { ...grant, method: "GET" } };
  }

  async get(artifactId: string, ownerAccountId?: string): Promise<ArtifactSummary> { return summary(await this.requireCommitted(artifactId, ownerAccountId)); }
  async list(query: ArtifactListQuery, ownerAccountId?: string): Promise<{ artifacts: ArtifactSummary[]; nextCursor: string | null }> {
    const records = await this.repository.list(query.limit + 1, query.after, ownerAccountId);
    const page = records.slice(0, query.limit).map(summary);
    return { artifacts: page, nextCursor: records.length > query.limit ? page.at(-1)?.artifact.artifactId ?? null : null };
  }
  async delete(artifactId: string, ownerAccountId: string): Promise<{ artifactId: string; state: "deleted" }> {
    const record = await this.repository.find(artifactId);
    if (!record || record.ownerAccountId !== ownerAccountId) throw artifactNotFound();
    if (record.state === "deleted") return { artifactId, state: "deleted" };
    if (record.state !== "committed") throw artifactNotFound();
    const claimed = await this.repository.claimForManualDeletion(artifactId, ownerAccountId, this.now());
    if (!claimed) throw artifactNotFound();
    let objectDeleted = false;
    try {
      await this.store.delete(record.objectKey);
      objectDeleted = true;
      await this.repository.markDeleted(record.artifactId, this.now());
      return { artifactId, state: "deleted" };
    } catch (error) {
      if (!objectDeleted) {
        await this.repository.restoreAfterDeletionFailure(record.artifactId, "committed");
      }
      throw error;
    }
  }
  async reconcile(): Promise<{ deleted: number }> {
    let deleted = 0;
    const failures: unknown[] = [];
    const now = this.now();
    const staleBefore = new Date(now.getTime() - deletionLeaseMilliseconds);
    for (const record of await this.repository.retentionCandidates(now, staleBefore)) {
      const originalState = retainableState(record);
      const claimed = record.state === "deleting"
        ? await this.repository.reclaimDeletion(record.artifactId, staleBefore, now)
        : originalState !== null && await this.repository.claimForDeletion(record.artifactId, originalState, now);
      if (!claimed) continue;
      let objectDeleted = false;
      try {
        await this.store.delete(record.objectKey);
        objectDeleted = true;
        await this.repository.markDeleted(record.artifactId, this.now());
      } catch (error) {
        if (!objectDeleted && originalState !== null) {
          try {
            await this.repository.restoreAfterDeletionFailure(record.artifactId, originalState);
          } catch (restoreError) {
            failures.push(
              new AggregateError(
                [error, restoreError],
                "Artifact retention recovery failed",
              ),
            );
            continue;
          }
        }
        failures.push(error);
        continue;
      }
      deleted += 1;
    }
    if (failures.length === 1) {
      throw failures[0];
    }
    if (failures.length > 1) {
      throw new AggregateError(
        failures,
        `Artifact retention failed for ${failures.length} candidates after deleting ${deleted}`,
      );
    }
    return { deleted };
  }

  private async requireCommitted(artifactId: string, ownerAccountId?: string) {
    const record = await this.repository.find(artifactId);
    if (!record || record.state !== "committed" || (ownerAccountId && record.ownerAccountId !== ownerAccountId)) throw artifactNotFound();
    return record;
  }
}

function retainableState(record: ArtifactRecord): RetainableArtifactState | null {
  return record.state === "planned" || record.state === "committed"
    ? record.state
    : null;
}

export function objectKey(artifactId: string, sha256: string): string { return `v1/private/artifacts/${artifactId}/${sha256}`; }

function summary(record: ArtifactRecord): ArtifactSummary {
  return { artifact: { artifactId: record.artifactId, committedAt: record.committedAt?.toISOString() ?? null, contentType: record.contentType, expiresAt: record.expiresAt?.toISOString() ?? null, kind: record.kind, producer: record.producer, sha256: record.sha256, sizeBytes: record.sizeBytes, state: record.state, verification: "server-sha256" }, artifactRef: { $schema: "urn:filecheap.dev:artifact-ref:v1", artifact_id: record.artifactId, kind: record.kind, producer: record.producer, provider: "fcheap-cloud", uri: `fcheap://cloud/vaults/private/artifacts/${record.artifactId}`, version: 1 } };
}

function committedPlanResult(record: ArtifactRecord): ArtifactPlanReplayResponse {
  if (record.state !== "committed" || record.committedAt === null) {
    throw new Error("Committed artifact metadata is incomplete");
  }
  const value = summary(record);
  return {
    artifact: {
      ...value.artifact,
      committedAt: record.committedAt.toISOString(),
      state: "committed",
    },
    artifactRef: value.artifactRef,
  };
}

function plannedPlanResult(
  record: ArtifactRecord,
  upload: ArtifactPlanResponse["upload"],
): ArtifactPlanResponse {
  if (record.state !== "planned" || record.committedAt !== null) {
    throw new Error("Planned artifact metadata is inconsistent");
  }
  const value = summary(record);
  return {
    artifact: {
      ...value.artifact,
      committedAt: null,
      state: "planned",
    },
    artifactRef: value.artifactRef,
    receipt: record.planToken,
    upload,
  };
}

function invalidReceipt(): PlatformError { return new PlatformError({ code: "invalid_receipt", detail: "The upload receipt is invalid or expired.", status: 400, title: "Invalid receipt" }); }
function retentionExpired(): PlatformError { return new PlatformError({ code: "artifact_retention_expired", detail: "The requested artifact retention window has already ended.", status: 422, title: "Artifact retention expired" }); }
function artifactNotFound(): PlatformError { return new PlatformError({ code: "artifact_not_found", detail: "No committed artifact exists for this identifier.", status: 404, title: "Artifact not found" }); }

function matchesArtifactPolicy(
  record: ArtifactRecord,
  policy: ArtifactIngestPolicy,
): boolean {
  return (
    record.producer.tool === policy.producerTool &&
    policy.kinds.includes(record.kind) &&
    Boolean(record.producer.native_schema) &&
    policy.nativeSchemas.includes(record.producer.native_schema!)
  );
}

function samePlan(record: ArtifactRecord, input: ArtifactPlanInput, ownerAccountId?: string): boolean {
  return (
    record.ownerAccountId === (ownerAccountId ?? null) &&
    record.sha256 === input.sha256 &&
    record.objectKey === objectKey(record.artifactId, input.sha256) &&
    record.objectSha256 === input.sha256 &&
    record.objectSizeBytes === input.sizeBytes &&
    record.objectContentType === input.contentType &&
    record.sizeBytes === input.sizeBytes &&
    record.contentType === input.contentType &&
    record.kind === input.kind &&
    record.producer.tool === input.producer.tool &&
    record.producer.version === input.producer.version &&
    record.producer.native_schema === input.producer.native_schema &&
    record.producer.native_id === input.producer.native_id &&
    record.producer.entrypoint === input.producer.entrypoint &&
    record.runIndexSha256 === (input.runIndex ? digestRunIndex(input.runIndex) : null) &&
    (record.expiresAt?.getTime() ?? null) ===
      (input.expiresAt ? new Date(input.expiresAt).getTime() : null)
  );
}

function digestRunIndex(value: NonNullable<ArtifactPlanInput["runIndex"]>): string {
  return createHash("sha256").update(JSON.stringify(value)).digest("hex");
}
