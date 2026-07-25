import { randomUUID } from "node:crypto";

import { and, asc, desc, eq, gt, isNull, lt, lte, or, sql } from "drizzle-orm";

import type { ArtifactPlanInput } from "@/features/artifacts/contracts";
import { getDatabase } from "@/platform/database/client";
import { artifactObjects, artifacts } from "@/platform/database/schema";

export type ArtifactState = "planned" | "committed" | "deleting" | "deleted";
export type RetainableArtifactState = Extract<ArtifactState, "planned" | "committed">;
export type ArtifactRecord = {
  artifactId: string; contentType: string; committedAt: Date | null; expiresAt: Date | null;
  deletingAt: Date | null; kind: string; objectContentType: string; objectEtag: string | null; objectKey: string;
  objectSha256: string; objectSizeBytes: number; planExpiresAt: Date; planToken: string;
  producer: ArtifactPlanInput["producer"]; sha256: string; sizeBytes: number; state: ArtifactState;
};

export interface ArtifactRepository {
  createPlan(input: ArtifactPlanInput, values: { artifactId: string; objectKey: string; planExpiresAt: Date; planToken: string; now: Date }): Promise<ArtifactRecord>;
  find(artifactId: string): Promise<ArtifactRecord | null>;
  findByPlanToken(planToken: string): Promise<ArtifactRecord | null>;
  list(limit: number, after?: string): Promise<ArtifactRecord[]>;
  markCommitted(artifactId: string, etag: string, now: Date): Promise<ArtifactRecord | null>;
  renewPlan(artifactId: string, planToken: string, values: { now: Date; planExpiresAt: Date }): Promise<ArtifactRecord | null>;
  restartDeletedPlan(artifactId: string, values: { now: Date; planExpiresAt: Date; planToken: string }): Promise<ArtifactRecord | null>;
  claimForDeletion(artifactId: string, expectedState: RetainableArtifactState, now: Date): Promise<boolean>;
  reclaimDeletion(artifactId: string, staleBefore: Date, now: Date): Promise<boolean>;
  markDeleted(artifactId: string, now: Date): Promise<void>;
  restoreAfterDeletionFailure(artifactId: string, state: RetainableArtifactState): Promise<void>;
  retentionCandidates(now: Date, staleBefore: Date): Promise<ArtifactRecord[]>;
}

export class DrizzleArtifactRepository implements ArtifactRepository {
  private readonly db = getDatabase();
  async createPlan(input: ArtifactPlanInput, values: { artifactId: string; objectKey: string; planExpiresAt: Date; planToken: string; now: Date }): Promise<ArtifactRecord> {
    await this.db.execute(sql`
      WITH inserted_artifact AS (
        INSERT INTO ${artifacts} (
          artifact_id, kind, producer, sha256, size_bytes, content_type,
          state, verification, plan_token, plan_expires_at, expires_at, created_at
        )
        VALUES (
          ${values.artifactId}, ${input.kind}, ${JSON.stringify(input.producer)}::jsonb,
          ${input.sha256}, ${input.sizeBytes}, ${input.contentType},
          'planned', 'server-sha256', ${values.planToken},
          ${values.planExpiresAt.toISOString()}::timestamptz,
          ${input.expiresAt ?? null}::timestamptz,
          ${values.now.toISOString()}::timestamptz
        )
        ON CONFLICT (artifact_id) DO NOTHING
        RETURNING artifact_id
      )
      INSERT INTO ${artifactObjects} (
        id, artifact_id, ordinal, object_key, sha256, size_bytes, content_type, created_at
      )
      SELECT
        ${randomUUID()}, artifact_id, 0, ${values.objectKey}, ${input.sha256},
        ${input.sizeBytes}, ${input.contentType}, ${values.now.toISOString()}::timestamptz
      FROM inserted_artifact
      ON CONFLICT (artifact_id, ordinal) DO NOTHING
    `);
    const record = await this.find(values.artifactId);
    if (!record) {
      throw new Error("Artifact plan creation did not produce a complete record");
    }
    return record;
  }
  async find(artifactId: string): Promise<ArtifactRecord | null> { const row = (await this.db.select({ artifact: artifacts, object: artifactObjects }).from(artifacts).innerJoin(artifactObjects, eq(artifactObjects.artifactId, artifacts.artifactId)).where(eq(artifacts.artifactId, artifactId)).limit(1))[0]; return row ? mapRow(row.artifact, row.object) : null; }
  async findByPlanToken(planToken: string): Promise<ArtifactRecord | null> { const row = (await this.db.select({ artifact: artifacts, object: artifactObjects }).from(artifacts).innerJoin(artifactObjects, eq(artifactObjects.artifactId, artifacts.artifactId)).where(eq(artifacts.planToken, planToken)).limit(1))[0]; return row ? mapRow(row.artifact, row.object) : null; }
  async list(limit: number, after?: string): Promise<ArtifactRecord[]> { const rows = await this.db.select({ artifact: artifacts, object: artifactObjects }).from(artifacts).innerJoin(artifactObjects, eq(artifactObjects.artifactId, artifacts.artifactId)).where(and(eq(artifacts.state, "committed"), ...(after ? [lt(artifacts.artifactId, after)] : []))).orderBy(desc(artifacts.artifactId)).limit(limit); return rows.map((row) => mapRow(row.artifact, row.object)); }
  async markCommitted(artifactId: string, etag: string, now: Date): Promise<ArtifactRecord | null> {
    const retained = or(isNull(artifacts.expiresAt), gt(artifacts.expiresAt, now));
    const result = await this.db.update(artifacts).set({ state: "committed", committedAt: now }).where(and(eq(artifacts.artifactId, artifactId), eq(artifacts.state, "planned"), gt(artifacts.planExpiresAt, now), retained)).returning({ artifactId: artifacts.artifactId });
    if (result.length === 0) {
      const current = await this.find(artifactId);
      return current?.state === "committed" &&
        (current.expiresAt === null || current.expiresAt > now)
        ? current
        : null;
    }
    await this.db.update(artifactObjects).set({ etag }).where(eq(artifactObjects.artifactId, artifactId));
    const current = await this.find(artifactId);
    return current?.state === "committed" ? current : null;
  }
  async renewPlan(artifactId: string, planToken: string, values: { now: Date; planExpiresAt: Date }): Promise<ArtifactRecord | null> {
    await this.db.update(artifacts).set({ planExpiresAt: values.planExpiresAt }).where(and(eq(artifacts.artifactId, artifactId), eq(artifacts.state, "planned"), eq(artifacts.planToken, planToken), lte(artifacts.planExpiresAt, values.now), or(isNull(artifacts.expiresAt), gt(artifacts.expiresAt, values.now))));
    return this.find(artifactId);
  }
  async restartDeletedPlan(artifactId: string, values: { now: Date; planExpiresAt: Date; planToken: string }): Promise<ArtifactRecord | null> {
    await this.db.update(artifacts).set({ state: "planned", planExpiresAt: values.planExpiresAt, planToken: values.planToken, deletingAt: null, deletedAt: null }).where(and(eq(artifacts.artifactId, artifactId), eq(artifacts.state, "deleted"), isNull(artifacts.committedAt)));
    return this.find(artifactId);
  }
  async claimForDeletion(artifactId: string, expectedState: RetainableArtifactState, now: Date): Promise<boolean> {
    const expiry = expectedState === "planned" ? lte(artifacts.planExpiresAt, now) : lte(artifacts.expiresAt, now);
    const result = await this.db.update(artifacts).set({ state: "deleting", deletingAt: now }).where(and(eq(artifacts.artifactId, artifactId), eq(artifacts.state, expectedState), expiry)).returning({ artifactId: artifacts.artifactId });
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
    return rows.map((row) => mapRow(row.artifact, row.object));
  }
}

function mapRow(artifact: typeof artifacts.$inferSelect, object: typeof artifactObjects.$inferSelect): ArtifactRecord { return { artifactId: artifact.artifactId, contentType: artifact.contentType, committedAt: artifact.committedAt, deletingAt: artifact.deletingAt, expiresAt: artifact.expiresAt, kind: artifact.kind, objectContentType: object.contentType, objectEtag: object.etag, objectKey: object.objectKey, objectSha256: object.sha256, objectSizeBytes: object.sizeBytes, planExpiresAt: artifact.planExpiresAt, planToken: artifact.planToken, producer: artifact.producer as ArtifactPlanInput["producer"], sha256: artifact.sha256, sizeBytes: artifact.sizeBytes, state: artifact.state as ArtifactState }; }
