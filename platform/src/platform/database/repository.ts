import { randomUUID } from "node:crypto";

import { and, asc, desc, eq, gt, isNull, lt, lte, or, sql, type SQL } from "drizzle-orm";

import type { ArtifactPlanInput } from "@/features/artifacts/contracts";
import {
  legacyPlanReceiptLookupSchemeV1,
  planReceiptSchemeV1,
  type IssuedPlanReceipt,
  type PlanReceiptLookupCandidate,
} from "@/features/artifacts/plan-receipts";
import type { RunIndexV1 } from "@/features/runs/index-contract";
import { getDatabase } from "@/platform/database/client";
import { artifactObjects, artifactRuns, artifacts } from "@/platform/database/schema";

export type ArtifactState = "planned" | "committed" | "deleting" | "deleted";
export type RetainableArtifactState = Extract<ArtifactState, "planned" | "committed">;
export type ArtifactPlanReceipt =
  | Readonly<{
      kid: string;
      lookup: string;
      nonce: string;
      scheme: typeof planReceiptSchemeV1;
    }>
  | Readonly<{
      kid: string;
      lookup: string;
      nonce: null;
      scheme: typeof legacyPlanReceiptLookupSchemeV1;
    }>;
export type ArtifactPlanReceiptMatch =
  | Readonly<{
      kid: string;
      lookup: string;
      mode: "lookup";
    }>
  | Readonly<{
      mode: "legacy";
      planToken: string;
    }>;
export type ArtifactRecord = {
  artifactId: string; ownerAccountId: string | null; contentType: string; committedAt: Date | null; expiresAt: Date | null;
  deletingAt: Date | null; kind: string; objectContentType: string; objectEtag: string | null; objectKey: string;
  objectSha256: string; objectSizeBytes: number; planExpiresAt: Date; planReceipt: ArtifactPlanReceipt | null; planToken: string;
  producer: ArtifactPlanInput["producer"]; runIndex: RunIndexV1 | null; runIndexSha256: string | null;
  sha256: string; sizeBytes: number; state: ArtifactState;
};

export interface ArtifactRepository {
  createPlan(input: ArtifactPlanInput, values: { artifactId: string; objectKey: string; ownerAccountId?: string; planExpiresAt: Date; receipt: IssuedPlanReceipt; runIndexSha256: string | null; now: Date }): Promise<ArtifactRecord>;
  find(artifactId: string): Promise<ArtifactRecord | null>;
  findByPlanReceipt(candidates: readonly PlanReceiptLookupCandidate[]): Promise<ArtifactRecord | null>;
  findLegacyByPlanToken(planToken: string): Promise<ArtifactRecord | null>;
  list(limit: number, after?: string, ownerAccountId?: string): Promise<ArtifactRecord[]>;
  markCommitted(artifactId: string, etag: string, now: Date): Promise<ArtifactRecord | null>;
  renewPlan(artifactId: string, expectedReceipt: ArtifactPlanReceiptMatch, values: { now: Date; planExpiresAt: Date }): Promise<ArtifactRecord | null>;
  restartDeletedPlan(artifactId: string, values: { now: Date; planExpiresAt: Date; receipt: IssuedPlanReceipt }): Promise<ArtifactRecord | null>;
  claimForDeletion(artifactId: string, expectedState: RetainableArtifactState, now: Date): Promise<boolean>;
  claimForManualDeletion(artifactId: string, ownerAccountId: string, now: Date): Promise<boolean>;
  reclaimDeletion(artifactId: string, staleBefore: Date, now: Date): Promise<boolean>;
  markDeleted(artifactId: string, now: Date): Promise<void>;
  restoreAfterDeletionFailure(artifactId: string, state: RetainableArtifactState): Promise<void>;
  retentionCandidates(now: Date, staleBefore: Date): Promise<ArtifactRecord[]>;
}

export class DrizzleArtifactRepository implements ArtifactRepository {
  constructor(private readonly db: ReturnType<typeof getDatabase> = getDatabase()) {}

  async createPlan(input: ArtifactPlanInput, values: { artifactId: string; objectKey: string; ownerAccountId?: string; planExpiresAt: Date; receipt: IssuedPlanReceipt; runIndexSha256: string | null; now: Date }): Promise<ArtifactRecord> {
    await this.db.execute(sql`
      WITH inserted_artifact AS (
        INSERT INTO ${artifacts} (
          artifact_id, owner_account_id, kind, producer, sha256, size_bytes, content_type,
          state, verification, plan_token, plan_receipt_scheme,
          plan_receipt_kid, plan_receipt_nonce, plan_receipt_lookup,
          plan_expires_at, expires_at, created_at
        )
        VALUES (
          ${values.artifactId}, ${values.ownerAccountId ?? null}, ${input.kind}, ${JSON.stringify(input.producer)}::jsonb,
          ${input.sha256}, ${input.sizeBytes}, ${input.contentType},
          'planned', 'server-sha256', ${values.receipt.receipt},
          ${values.receipt.receiptScheme}, ${values.receipt.receiptKid},
          ${values.receipt.receiptNonce}, ${values.receipt.receiptLookup},
          ${values.planExpiresAt.toISOString()}::timestamptz,
          ${input.expiresAt ?? null}::timestamptz,
          ${values.now.toISOString()}::timestamptz
        )
        ON CONFLICT (artifact_id) DO NOTHING
        RETURNING artifact_id
      )
      , inserted_object AS (
        INSERT INTO ${artifactObjects} (
          id, artifact_id, ordinal, object_key, sha256, size_bytes, content_type, created_at
        )
        SELECT
          ${randomUUID()}, artifact_id, 0, ${values.objectKey}, ${input.sha256},
          ${input.sizeBytes}, ${input.contentType}, ${values.now.toISOString()}::timestamptz
        FROM inserted_artifact
        ON CONFLICT (artifact_id, ordinal) DO NOTHING
        RETURNING artifact_id
      ), inserted_run AS (
        INSERT INTO ${artifactRuns} (
          artifact_id, owner_account_id, schema_version, run_index_sha256, source_sha256,
          detector_name, detector_version, producer_tool, native_schema, native_run_id,
          series_key, spec_name, status, health, started_at, ended_at, duration_ms,
          environment, backend, exit_code, error_kind, step_count, outcome_count,
          artifact_count, health_declared, health_present, health_empty, health_missing,
          health_changed, health_reasons, outcomes, evidence, created_at, updated_at
        )
        SELECT
          artifact_id, ${values.ownerAccountId ?? null}, 1, ${values.runIndexSha256}, ${input.sha256},
          ${input.runIndex?.detector.name ?? null}, ${input.runIndex?.detector.version ?? null},
          ${input.producer.tool}, ${input.producer.native_schema ?? null},
          ${input.runIndex?.run.nativeId ?? null}, ${input.runIndex?.run.seriesKey ?? null},
          ${input.runIndex?.run.specName ?? null}, ${input.runIndex?.run.status ?? null},
          ${input.runIndex?.health.state ?? null}, ${input.runIndex?.run.startedAt ?? null}::timestamptz,
          ${input.runIndex?.run.endedAt ?? null}::timestamptz, ${input.runIndex?.run.durationMs ?? null},
          ${input.runIndex?.run.environment ?? null}, ${input.runIndex?.run.backend ?? null},
          ${input.runIndex?.run.exitCode ?? null}, ${input.runIndex?.run.errorKind ?? null},
          ${input.runIndex?.counts.steps ?? 0}, ${input.runIndex?.counts.outcomes ?? 0},
          ${input.runIndex?.counts.artifacts ?? 0}, ${input.runIndex?.health.declared ?? 0},
          ${input.runIndex?.health.present ?? 0}, ${input.runIndex?.health.empty ?? 0},
          ${input.runIndex?.health.missing ?? 0}, ${input.runIndex?.health.changed ?? 0},
          ${JSON.stringify(input.runIndex?.health.reasons ?? [])}::jsonb,
          ${JSON.stringify(input.runIndex?.outcomes ?? [])}::jsonb,
          ${JSON.stringify(input.runIndex?.evidence ?? [])}::jsonb,
          ${values.now.toISOString()}::timestamptz, ${values.now.toISOString()}::timestamptz
        FROM inserted_artifact
        WHERE ${input.runIndex !== undefined}
        RETURNING artifact_id
      )
      SELECT artifact_id FROM inserted_artifact
    `);
    const record = await this.find(values.artifactId);
    if (!record) {
      throw new Error("Artifact plan creation did not produce a complete record");
    }
    return record;
  }
  async find(artifactId: string): Promise<ArtifactRecord | null> { const row = (await this.db.select({ artifact: artifacts, object: artifactObjects, run: artifactRuns }).from(artifacts).innerJoin(artifactObjects, eq(artifactObjects.artifactId, artifacts.artifactId)).leftJoin(artifactRuns, eq(artifactRuns.artifactId, artifacts.artifactId)).where(eq(artifacts.artifactId, artifactId)).limit(1))[0]; return row ? mapRow(row.artifact, row.object, row.run) : null; }
  async findByPlanReceipt(candidates: readonly PlanReceiptLookupCandidate[]): Promise<ArtifactRecord | null> {
    if (candidates.length === 0) return null;
    const row = (await this.db
      .select({ artifact: artifacts, object: artifactObjects, run: artifactRuns })
      .from(artifacts)
      .innerJoin(artifactObjects, eq(artifactObjects.artifactId, artifacts.artifactId))
      .leftJoin(artifactRuns, eq(artifactRuns.artifactId, artifacts.artifactId))
      .where(or(...candidates.map((candidate) => and(
        eq(artifacts.planReceiptKid, candidate.receiptKid),
        eq(artifacts.planReceiptLookup, candidate.receiptLookup),
      ))))
      .limit(1))[0];
    return row ? mapRow(row.artifact, row.object, row.run) : null;
  }
  async findLegacyByPlanToken(planToken: string): Promise<ArtifactRecord | null> {
    const row = (await this.db
      .select({ artifact: artifacts, object: artifactObjects, run: artifactRuns })
      .from(artifacts)
      .innerJoin(artifactObjects, eq(artifactObjects.artifactId, artifacts.artifactId))
      .leftJoin(artifactRuns, eq(artifactRuns.artifactId, artifacts.artifactId))
      .where(and(
        eq(artifacts.planToken, planToken),
        isNull(artifacts.planReceiptScheme),
        isNull(artifacts.planReceiptKid),
        isNull(artifacts.planReceiptNonce),
        isNull(artifacts.planReceiptLookup),
      ))
      .limit(1))[0];
    return row ? mapRow(row.artifact, row.object, row.run) : null;
  }
  async list(limit: number, after?: string, ownerAccountId?: string): Promise<ArtifactRecord[]> { const rows = await this.db.select({ artifact: artifacts, object: artifactObjects, run: artifactRuns }).from(artifacts).innerJoin(artifactObjects, eq(artifactObjects.artifactId, artifacts.artifactId)).leftJoin(artifactRuns, eq(artifactRuns.artifactId, artifacts.artifactId)).where(and(eq(artifacts.state, "committed"), ...(ownerAccountId ? [eq(artifacts.ownerAccountId, ownerAccountId)] : []), ...(after ? [lt(artifacts.artifactId, after)] : []))).orderBy(desc(artifacts.artifactId)).limit(limit); return rows.map((row) => mapRow(row.artifact, row.object, row.run)); }
  async markCommitted(artifactId: string, etag: string, now: Date): Promise<ArtifactRecord | null> {
    const retained = or(isNull(artifacts.expiresAt), gt(artifacts.expiresAt, now));
    const result = await this.db.update(artifacts).set({ state: "committed", committedAt: now }).where(and(eq(artifacts.artifactId, artifactId), eq(artifacts.state, "planned"), gt(artifacts.planExpiresAt, now), retained)).returning({ artifactId: artifacts.artifactId });
    if (result.length === 0) {
      const current = await this.find(artifactId);
      return current?.state === "committed" &&
        current.planExpiresAt > now &&
        (current.expiresAt === null || current.expiresAt > now)
        ? current
        : null;
    }
    await this.db.update(artifactObjects).set({ etag }).where(eq(artifactObjects.artifactId, artifactId));
    const current = await this.find(artifactId);
    return current?.state === "committed" ? current : null;
  }
  async renewPlan(artifactId: string, expectedReceipt: ArtifactPlanReceiptMatch, values: { now: Date; planExpiresAt: Date }): Promise<ArtifactRecord | null> {
    await this.db.update(artifacts).set({ planExpiresAt: values.planExpiresAt }).where(and(eq(artifacts.artifactId, artifactId), eq(artifacts.state, "planned"), receiptMatchCondition(expectedReceipt), lte(artifacts.planExpiresAt, values.now), or(isNull(artifacts.expiresAt), gt(artifacts.expiresAt, values.now))));
    return this.find(artifactId);
  }
  async restartDeletedPlan(artifactId: string, values: { now: Date; planExpiresAt: Date; receipt: IssuedPlanReceipt }): Promise<ArtifactRecord | null> {
    await this.db.update(artifacts).set({
      state: "planned",
      planExpiresAt: values.planExpiresAt,
      planToken: values.receipt.receipt,
      planReceiptScheme: values.receipt.receiptScheme,
      planReceiptKid: values.receipt.receiptKid,
      planReceiptNonce: values.receipt.receiptNonce,
      planReceiptLookup: values.receipt.receiptLookup,
      deletingAt: null,
      deletedAt: null,
    }).where(and(eq(artifacts.artifactId, artifactId), eq(artifacts.state, "deleted"), isNull(artifacts.committedAt)));
    return this.find(artifactId);
  }
  async claimForDeletion(artifactId: string, expectedState: RetainableArtifactState, now: Date): Promise<boolean> {
    const expiry = expectedState === "planned" ? lte(artifacts.planExpiresAt, now) : lte(artifacts.expiresAt, now);
    const result = await this.db.update(artifacts).set({ state: "deleting", deletingAt: now }).where(and(eq(artifacts.artifactId, artifactId), eq(artifacts.state, expectedState), expiry)).returning({ artifactId: artifacts.artifactId });
    return result.length === 1;
  }
  async claimForManualDeletion(artifactId: string, ownerAccountId: string, now: Date): Promise<boolean> {
    const result = await this.db.update(artifacts).set({ state: "deleting", deletingAt: now }).where(and(eq(artifacts.artifactId, artifactId), eq(artifacts.ownerAccountId, ownerAccountId), eq(artifacts.state, "committed"))).returning({ artifactId: artifacts.artifactId });
    return result.length === 1;
  }
  async reclaimDeletion(artifactId: string, staleBefore: Date, now: Date): Promise<boolean> {
    const result = await this.db.update(artifacts).set({ deletingAt: now }).where(and(eq(artifacts.artifactId, artifactId), eq(artifacts.state, "deleting"), lte(artifacts.deletingAt, staleBefore))).returning({ artifactId: artifacts.artifactId });
    return result.length === 1;
  }
  async markDeleted(artifactId: string, now: Date): Promise<void> { await this.db.update(artifacts).set({ state: "deleted", deletedAt: now }).where(and(eq(artifacts.artifactId, artifactId), eq(artifacts.state, "deleting"))); }
  async restoreAfterDeletionFailure(artifactId: string, state: RetainableArtifactState): Promise<void> { await this.db.update(artifacts).set({ state, deletingAt: null }).where(and(eq(artifacts.artifactId, artifactId), eq(artifacts.state, "deleting"))); }
  async retentionCandidates(now: Date, staleBefore: Date): Promise<ArtifactRecord[]> {
    const rows = await this.db.select({ artifact: artifacts, object: artifactObjects }).from(artifacts).innerJoin(artifactObjects, eq(artifactObjects.artifactId, artifacts.artifactId)).where(or(
      and(eq(artifacts.state, "planned"), lte(artifacts.planExpiresAt, now)),
      and(eq(artifacts.state, "committed"), lte(artifacts.expiresAt, now)),
      and(eq(artifacts.state, "deleting"), lte(artifacts.deletingAt, staleBefore)),
    )).orderBy(asc(artifacts.artifactId)).limit(50);
    return rows.map((row) => mapRow(row.artifact, row.object, null));
  }
}

function mapRow(artifact: typeof artifacts.$inferSelect, object: typeof artifactObjects.$inferSelect, run: typeof artifactRuns.$inferSelect | null): ArtifactRecord { return { artifactId: artifact.artifactId, ownerAccountId: artifact.ownerAccountId, contentType: artifact.contentType, committedAt: artifact.committedAt, deletingAt: artifact.deletingAt, expiresAt: artifact.expiresAt, kind: artifact.kind, objectContentType: object.contentType, objectEtag: object.etag, objectKey: object.objectKey, objectSha256: object.sha256, objectSizeBytes: object.sizeBytes, planExpiresAt: artifact.planExpiresAt, planReceipt: mapPlanReceipt(artifact), planToken: artifact.planToken, producer: artifact.producer as ArtifactPlanInput["producer"], runIndex: run ? mapRunIndex(run) : null, runIndexSha256: run?.runIndexSha256 ?? null, sha256: artifact.sha256, sizeBytes: artifact.sizeBytes, state: artifact.state as ArtifactState }; }

function mapPlanReceipt(artifact: typeof artifacts.$inferSelect): ArtifactPlanReceipt | null {
  if (
    artifact.planReceiptScheme === null &&
    artifact.planReceiptKid === null &&
    artifact.planReceiptNonce === null &&
    artifact.planReceiptLookup === null
  ) return null;
  if (
    artifact.planReceiptScheme === planReceiptSchemeV1 &&
    artifact.planReceiptKid !== null &&
    artifact.planReceiptNonce !== null &&
    artifact.planReceiptLookup !== null
  ) {
    return { kid: artifact.planReceiptKid, lookup: artifact.planReceiptLookup, nonce: artifact.planReceiptNonce, scheme: planReceiptSchemeV1 };
  }
  if (
    artifact.planReceiptScheme === legacyPlanReceiptLookupSchemeV1 &&
    artifact.planReceiptKid !== null &&
    artifact.planReceiptNonce === null &&
    artifact.planReceiptLookup !== null
  ) {
    return { kid: artifact.planReceiptKid, lookup: artifact.planReceiptLookup, nonce: null, scheme: legacyPlanReceiptLookupSchemeV1 };
  }
  throw new Error("Artifact plan receipt metadata is inconsistent");
}

function receiptMatchCondition(match: ArtifactPlanReceiptMatch): SQL {
  if (match.mode === "lookup") {
    return and(
      eq(artifacts.planReceiptKid, match.kid),
      eq(artifacts.planReceiptLookup, match.lookup),
    )!;
  }
  return and(
    eq(artifacts.planToken, match.planToken),
    isNull(artifacts.planReceiptScheme),
    isNull(artifacts.planReceiptKid),
    isNull(artifacts.planReceiptNonce),
    isNull(artifacts.planReceiptLookup),
  )!;
}

function mapRunIndex(run: typeof artifactRuns.$inferSelect): RunIndexV1 {
  return {
    $schema: "urn:filecheap.dev:run-index:v1",
    counts: { artifacts: run.artifactCount, outcomes: run.outcomeCount, steps: run.stepCount },
    detector: { name: run.detectorName as RunIndexV1["detector"]["name"], version: run.detectorVersion },
    evidence: run.evidence as RunIndexV1["evidence"],
    health: { changed: run.healthChanged, declared: run.healthDeclared, empty: run.healthEmpty, missing: run.healthMissing, present: run.healthPresent, reasons: run.healthReasons as RunIndexV1["health"]["reasons"], state: run.health as RunIndexV1["health"]["state"] },
    outcomes: run.outcomes as RunIndexV1["outcomes"],
    run: { ...(run.backend ? { backend: run.backend } : {}), ...(run.durationMs !== null ? { durationMs: run.durationMs } : {}), ...(run.endedAt ? { endedAt: run.endedAt.toISOString() } : {}), ...(run.environment ? { environment: run.environment } : {}), ...(run.errorKind ? { errorKind: run.errorKind } : {}), ...(run.exitCode !== null ? { exitCode: run.exitCode } : {}), nativeId: run.nativeRunId, seriesKey: run.seriesKey, ...(run.specName ? { specName: run.specName } : {}), ...(run.startedAt ? { startedAt: run.startedAt.toISOString() } : {}), status: run.status as RunIndexV1["run"]["status"] },
    version: 1,
  };
}
