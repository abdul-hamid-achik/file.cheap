import type { ArtifactPlanInput } from "@/features/artifacts/contracts";
import type {
  IssuedPlanReceipt,
  PlanReceiptLookupCandidate,
} from "@/features/artifacts/plan-receipts";
import type {
  ArtifactPlanReceiptMatch,
  ArtifactRecord,
  ArtifactRepository,
  RetainableArtifactState,
} from "@/platform/database/repository";

export class InMemoryArtifactRepository implements ArtifactRepository {
  private readonly records = new Map<string, ArtifactRecord>();
  async createPlan(input: ArtifactPlanInput, values: { artifactId: string; objectKey: string; ownerAccountId?: string; planExpiresAt: Date; receipt: IssuedPlanReceipt; runIndexSha256?: string | null }): Promise<ArtifactRecord> {
    const existing = this.records.get(values.artifactId);
    if (existing) return existing;
    const record: ArtifactRecord = { artifactId: values.artifactId, ownerAccountId: values.ownerAccountId ?? null, contentType: input.contentType, committedAt: null, deletingAt: null, expiresAt: input.expiresAt ? new Date(input.expiresAt) : null, kind: input.kind, objectContentType: input.contentType, objectEtag: null, objectKey: values.objectKey, objectSha256: input.sha256, objectSizeBytes: input.sizeBytes, planExpiresAt: values.planExpiresAt, planReceipt: { kid: values.receipt.receiptKid, lookup: values.receipt.receiptLookup, nonce: values.receipt.receiptNonce, scheme: values.receipt.receiptScheme }, planToken: values.receipt.receipt, producer: input.producer, runIndex: input.runIndex ?? null, runIndexSha256: values.runIndexSha256 ?? null, sha256: input.sha256, sizeBytes: input.sizeBytes, state: "planned" };
    this.records.set(record.artifactId, record);
    return record;
  }
  async find(artifactId: string): Promise<ArtifactRecord | null> { return this.records.get(artifactId) ?? null; }
  async findByPlanReceipt(candidates: readonly PlanReceiptLookupCandidate[]): Promise<ArtifactRecord | null> { return [...this.records.values()].find((record) => matchesLookupCandidate(record, candidates)) ?? null; }
  async findLegacyByPlanToken(planToken: string): Promise<ArtifactRecord | null> { return [...this.records.values()].find((record) => record.planReceipt === null && record.planToken === planToken) ?? null; }
  async list(limit: number, after?: string, ownerAccountId?: string): Promise<ArtifactRecord[]> { return [...this.records.values()].sort((left, right) => right.artifactId.localeCompare(left.artifactId)).filter((record) => record.state === "committed" && (!ownerAccountId || record.ownerAccountId === ownerAccountId) && (!after || record.artifactId < after)).slice(0, limit); }
  async markCommitted(artifactId: string, etag: string, now: Date): Promise<ArtifactRecord | null> { const record = this.records.get(artifactId); if (!record) return null; const retained = record.expiresAt === null || record.expiresAt > now; if (record.state === "planned" && record.planExpiresAt > now && retained) { record.state = "committed"; record.committedAt = now; record.objectEtag = etag; } return record.state === "committed" && record.planExpiresAt > now && retained ? record : null; }
  async renewPlan(artifactId: string, expectedReceipt: ArtifactPlanReceiptMatch, values: { now: Date; planExpiresAt: Date }): Promise<ArtifactRecord | null> { const record = this.records.get(artifactId); if (record?.state === "planned" && matchesReceipt(record, expectedReceipt) && record.planExpiresAt <= values.now && (record.expiresAt === null || record.expiresAt > values.now)) record.planExpiresAt = values.planExpiresAt; return record ?? null; }
  async restartDeletedPlan(artifactId: string, values: { planExpiresAt: Date; receipt: IssuedPlanReceipt }): Promise<ArtifactRecord | null> { const record = this.records.get(artifactId); if (record?.state === "deleted" && record.committedAt === null) { record.state = "planned"; record.planExpiresAt = values.planExpiresAt; record.planToken = values.receipt.receipt; record.planReceipt = { kid: values.receipt.receiptKid, lookup: values.receipt.receiptLookup, nonce: values.receipt.receiptNonce, scheme: values.receipt.receiptScheme }; record.deletingAt = null; } return record ?? null; }
  async claimForDeletion(artifactId: string, expectedState: RetainableArtifactState, now: Date): Promise<boolean> { const record = this.records.get(artifactId); const expired = expectedState === "planned" ? record?.planExpiresAt !== undefined && record.planExpiresAt <= now : record?.expiresAt !== null && record?.expiresAt !== undefined && record.expiresAt <= now; if (!record || record.state !== expectedState || !expired) return false; record.state = "deleting"; record.deletingAt = now; return true; }
  async claimForManualDeletion(artifactId: string, ownerAccountId: string, now: Date): Promise<boolean> { const record = this.records.get(artifactId); if (!record || record.state !== "committed" || record.ownerAccountId !== ownerAccountId) return false; record.state = "deleting"; record.deletingAt = now; return true; }
  async reclaimDeletion(artifactId: string, staleBefore: Date, now: Date): Promise<boolean> { const record = this.records.get(artifactId); if (!record || record.state !== "deleting" || record.deletingAt === null || record.deletingAt > staleBefore) return false; record.deletingAt = now; return true; }
  async markDeleted(artifactId: string): Promise<void> { const record = this.records.get(artifactId); if (record) record.state = "deleted"; }
  async restoreAfterDeletionFailure(artifactId: string, state: RetainableArtifactState): Promise<void> { const record = this.records.get(artifactId); if (record?.state === "deleting") { record.state = state; record.deletingAt = null; } }
  async retentionCandidates(now: Date, staleBefore: Date): Promise<ArtifactRecord[]> { return [...this.records.values()].filter((record) => (record.state === "planned" && record.planExpiresAt <= now) || (record.state === "committed" && record.expiresAt !== null && record.expiresAt <= now) || (record.state === "deleting" && record.deletingAt !== null && record.deletingAt <= staleBefore)).sort((left, right) => left.artifactId.localeCompare(right.artifactId)).slice(0, 50); }
}

function matchesReceipt(
  record: ArtifactRecord,
  expected: ArtifactPlanReceiptMatch,
): boolean {
  if (expected.mode === "legacy") {
    return record.planReceipt === null && record.planToken === expected.planToken;
  }
  return record.planReceipt !== null &&
    record.planReceipt.kid === expected.kid &&
    record.planReceipt.lookup === expected.lookup;
}

function matchesLookupCandidate(
  record: ArtifactRecord,
  candidates: readonly PlanReceiptLookupCandidate[],
): boolean {
  const receipt = record.planReceipt;
  return receipt !== null && candidates.some(
    (candidate) =>
      candidate.receiptKid === receipt.kid &&
      candidate.receiptLookup === receipt.lookup,
  );
}
