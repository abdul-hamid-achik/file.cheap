import type { ArtifactPlanInput } from "@/features/artifacts/contracts";
import type { ArtifactRecord, ArtifactRepository, RetainableArtifactState } from "@/platform/database/repository";

export class InMemoryArtifactRepository implements ArtifactRepository {
  private readonly records = new Map<string, ArtifactRecord>();
  async createPlan(input: ArtifactPlanInput, values: { artifactId: string; objectKey: string; planExpiresAt: Date; planToken: string }): Promise<ArtifactRecord> {
    const existing = this.records.get(values.artifactId);
    if (existing) return existing;
    const record: ArtifactRecord = { artifactId: values.artifactId, contentType: input.contentType, committedAt: null, deletingAt: null, expiresAt: input.expiresAt ? new Date(input.expiresAt) : null, kind: input.kind, objectContentType: input.contentType, objectEtag: null, objectKey: values.objectKey, objectSha256: input.sha256, objectSizeBytes: input.sizeBytes, planExpiresAt: values.planExpiresAt, planToken: values.planToken, producer: input.producer, sha256: input.sha256, sizeBytes: input.sizeBytes, state: "planned" };
    this.records.set(record.artifactId, record);
    return record;
  }
  async find(artifactId: string): Promise<ArtifactRecord | null> { return this.records.get(artifactId) ?? null; }
  async findByPlanToken(planToken: string): Promise<ArtifactRecord | null> { return [...this.records.values()].find((record) => record.planToken === planToken) ?? null; }
  async list(limit: number, after?: string): Promise<ArtifactRecord[]> { return [...this.records.values()].sort((left, right) => right.artifactId.localeCompare(left.artifactId)).filter((record) => record.state === "committed" && (!after || record.artifactId < after)).slice(0, limit); }
  async markCommitted(artifactId: string, etag: string, now: Date): Promise<ArtifactRecord | null> { const record = this.records.get(artifactId); if (!record) return null; const retained = record.expiresAt === null || record.expiresAt > now; if (record.state === "planned" && record.planExpiresAt > now && retained) { record.state = "committed"; record.committedAt = now; record.objectEtag = etag; } return record.state === "committed" && retained ? record : null; }
  async renewPlan(artifactId: string, planToken: string, values: { now: Date; planExpiresAt: Date }): Promise<ArtifactRecord | null> { const record = this.records.get(artifactId); if (record?.state === "planned" && record.planToken === planToken && record.planExpiresAt <= values.now && (record.expiresAt === null || record.expiresAt > values.now)) record.planExpiresAt = values.planExpiresAt; return record ?? null; }
  async restartDeletedPlan(artifactId: string, values: { planExpiresAt: Date; planToken: string }): Promise<ArtifactRecord | null> { const record = this.records.get(artifactId); if (record?.state === "deleted" && record.committedAt === null) { record.state = "planned"; record.planExpiresAt = values.planExpiresAt; record.planToken = values.planToken; record.deletingAt = null; } return record ?? null; }
  async claimForDeletion(artifactId: string, expectedState: RetainableArtifactState, now: Date): Promise<boolean> { const record = this.records.get(artifactId); const expired = expectedState === "planned" ? record?.planExpiresAt !== undefined && record.planExpiresAt <= now : record?.expiresAt !== null && record?.expiresAt !== undefined && record.expiresAt <= now; if (!record || record.state !== expectedState || !expired) return false; record.state = "deleting"; record.deletingAt = now; return true; }
  async reclaimDeletion(artifactId: string, staleBefore: Date, now: Date): Promise<boolean> { const record = this.records.get(artifactId); if (!record || record.state !== "deleting" || record.deletingAt === null || record.deletingAt > staleBefore) return false; record.deletingAt = now; return true; }
  async markDeleted(artifactId: string): Promise<void> { const record = this.records.get(artifactId); if (record) record.state = "deleted"; }
  async restoreAfterDeletionFailure(artifactId: string, state: RetainableArtifactState): Promise<void> { const record = this.records.get(artifactId); if (record?.state === "deleting") { record.state = state; record.deletingAt = null; } }
  async retentionCandidates(now: Date, staleBefore: Date): Promise<ArtifactRecord[]> { return [...this.records.values()].filter((record) => (record.state === "planned" && record.planExpiresAt <= now) || (record.state === "committed" && record.expiresAt !== null && record.expiresAt <= now) || (record.state === "deleting" && record.deletingAt !== null && record.deletingAt <= staleBefore)).sort((left, right) => left.artifactId.localeCompare(right.artifactId)).slice(0, 50); }
}
