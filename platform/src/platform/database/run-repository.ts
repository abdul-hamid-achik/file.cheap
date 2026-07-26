import { Buffer } from "node:buffer";

import { and, desc, eq, gte, lt, lte, or, sql } from "drizzle-orm";

import type { ArtifactPlanInput } from "@/features/artifacts/contracts";
import { runCursorSchema, type RunListQuery, type RunSummary } from "@/features/runs/contracts";
import type { RunRepository } from "@/features/runs/repository";
import { getDatabase } from "@/platform/database/client";
import { artifactRuns, artifacts } from "@/platform/database/schema";
import { PlatformError } from "@/shared/errors/platform-error";

type RunRow = { artifact: typeof artifacts.$inferSelect; run: typeof artifactRuns.$inferSelect };

export class DrizzleRunRepository implements RunRepository {
  private readonly db = getDatabase();

  async find(artifactId: string, ownerAccountId: string): Promise<RunSummary | null> {
    const row = (await this.db.select({ artifact: artifacts, run: artifactRuns })
      .from(artifactRuns)
      .innerJoin(artifacts, eq(artifacts.artifactId, artifactRuns.artifactId))
      .where(and(
        eq(artifactRuns.artifactId, artifactId),
        eq(artifactRuns.ownerAccountId, ownerAccountId),
        eq(artifacts.ownerAccountId, ownerAccountId),
        eq(artifacts.state, "committed"),
      )).limit(1))[0];
    return row ? mapRun(row) : null;
  }

  async list(query: RunListQuery, ownerAccountId: string): Promise<{ nextCursor: string | null; runs: RunSummary[] }> {
    const sortTime = sql<Date>`coalesce(${artifactRuns.startedAt}, ${artifactRuns.createdAt})`;
    const cursor = query.after ? decodeCursor(query.after) : undefined;
    const search = query.q ? `%${escapeLike(query.q)}%` : undefined;
    const rows = await this.db.select({ artifact: artifacts, run: artifactRuns })
      .from(artifactRuns)
      .innerJoin(artifacts, eq(artifacts.artifactId, artifactRuns.artifactId))
      .where(and(
        eq(artifactRuns.ownerAccountId, ownerAccountId),
        eq(artifacts.ownerAccountId, ownerAccountId),
        eq(artifacts.state, "committed"),
        query.status ? eq(artifactRuns.status, query.status) : undefined,
        query.health ? eq(artifactRuns.health, query.health) : undefined,
        query.producer ? eq(artifactRuns.producerTool, query.producer) : undefined,
        query.from ? gte(sortTime, new Date(query.from)) : undefined,
        query.to ? lte(sortTime, new Date(query.to)) : undefined,
        search ? or(
          sql`${artifactRuns.nativeRunId} ilike ${search} escape '\\'`,
          sql`${artifactRuns.seriesKey} ilike ${search} escape '\\'`,
          sql`${artifactRuns.specName} ilike ${search} escape '\\'`,
        ) : undefined,
        cursor ? or(
          lt(sortTime, cursor.time),
          and(eq(sortTime, cursor.time), lt(artifactRuns.artifactId, cursor.artifactId)),
        ) : undefined,
      ))
      .orderBy(desc(sortTime), desc(artifactRuns.artifactId))
      .limit(query.limit + 1);
    const page = rows.slice(0, query.limit);
    return {
      nextCursor: rows.length > query.limit && page.length > 0 ? encodeCursor(page.at(-1)!) : null,
      runs: page.map(mapRun),
    };
  }
}

function mapRun({ artifact, run }: RunRow): RunSummary {
  const producer = artifact.producer as ArtifactPlanInput["producer"];
  if (!producer.native_id || !producer.native_schema) {
    throw new Error("Indexed run producer identity is incomplete");
  }
  return {
    artifactId: artifact.artifactId,
    counts: { artifacts: run.artifactCount, outcomes: run.outcomeCount, steps: run.stepCount },
    createdAt: run.createdAt.toISOString(),
    detector: { name: run.detectorName as RunSummary["detector"]["name"], version: run.detectorVersion },
    evidence: run.evidence as RunSummary["evidence"],
    health: {
      changed: run.healthChanged,
      declared: run.healthDeclared,
      empty: run.healthEmpty,
      missing: run.healthMissing,
      present: run.healthPresent,
      reasons: run.healthReasons as RunSummary["health"]["reasons"],
      state: run.health as RunSummary["health"]["state"],
    },
    outcomes: run.outcomes as RunSummary["outcomes"],
    producer: { ...producer, native_id: producer.native_id, native_schema: producer.native_schema },
    run: {
      ...(run.backend ? { backend: run.backend } : {}),
      ...(run.durationMs !== null ? { durationMs: run.durationMs } : {}),
      ...(run.endedAt ? { endedAt: run.endedAt.toISOString() } : {}),
      ...(run.environment ? { environment: run.environment } : {}),
      ...(run.errorKind ? { errorKind: run.errorKind } : {}),
      ...(run.exitCode !== null ? { exitCode: run.exitCode } : {}),
      nativeId: run.nativeRunId,
      seriesKey: run.seriesKey,
      ...(run.specName ? { specName: run.specName } : {}),
      ...(run.startedAt ? { startedAt: run.startedAt.toISOString() } : {}),
      status: run.status as RunSummary["run"]["status"],
    },
    runIndexSha256: run.runIndexSha256,
    source: { contentType: artifact.contentType, kind: artifact.kind, sha256: run.sourceSha256, sizeBytes: artifact.sizeBytes },
    updatedAt: run.updatedAt.toISOString(),
  };
}

function encodeCursor(row: RunRow): string {
  const time = (row.run.startedAt ?? row.run.createdAt).toISOString();
  return Buffer.from(JSON.stringify([time, row.run.artifactId]), "utf8").toString("base64url");
}

function decodeCursor(value: string): { artifactId: string; time: Date } {
  try {
    runCursorSchema.parse(value);
    const decoded = JSON.parse(Buffer.from(value, "base64url").toString("utf8")) as unknown;
    if (!Array.isArray(decoded) || decoded.length !== 2 || typeof decoded[0] !== "string" || typeof decoded[1] !== "string") throw new Error("invalid shape");
    const time = new Date(decoded[0]);
    if (Number.isNaN(time.getTime()) || decoded[1].length < 1) throw new Error("invalid values");
    return { artifactId: decoded[1], time };
  } catch {
    throw new PlatformError({ code: "invalid_cursor", detail: "The run cursor is invalid or expired.", status: 422, title: "Invalid cursor" });
  }
}

function escapeLike(value: string): string { return value.replaceAll("\\", "\\\\").replaceAll("%", "\\%").replaceAll("_", "\\_"); }
