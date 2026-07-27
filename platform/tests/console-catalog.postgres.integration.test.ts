import { afterAll, beforeAll, describe, expect, test } from "bun:test";

import {
  consoleArtifactListResponseSchema,
  consoleRunListResponseSchema,
} from "@/features/console/catalog/contracts";
import { DrizzleConsoleCatalogRepository } from "@/platform/database/console-catalog-repository";
import {
  artifactRuns,
  artifacts,
  consoleUsers,
} from "@/platform/database/schema";
import {
  openPostgresTestDatabase,
  truncatePostgresTestData,
} from "./postgres-test-database";

const databaseUrl = process.env.FILECHEAP_POSTGRES_TEST_URL;
const now = new Date("2026-07-26T18:00:00.000Z");
const ownerId = "acc_catalog_postgres_owner";
const otherOwnerId = "acc_catalog_postgres_other";

describe.skipIf(!databaseUrl)("console catalog PostgreSQL repository", () => {
  let harness: ReturnType<typeof openPostgresTestDatabase>;
  let repository: DrizzleConsoleCatalogRepository;

  beforeAll(async () => {
    harness = openPostgresTestDatabase();
    repository = new DrizzleConsoleCatalogRepository(harness.database);
    await truncatePostgresTestData(harness);
    await seedCatalog(harness.database);
  });

  afterAll(async () => {
    await truncatePostgresTestData(harness);
    await harness.pool.end();
  });

  test("traverses artifact ties in both directions without overlap", async () => {
    const first = await repository.listArtifacts(
      { direction: "next", limit: 10 },
      ownerId,
      now,
    );
    expect(first.artifacts).toHaveLength(10);
    expect(first.filteredTotal).toBe(61);
    expect(first.overview.recordedCount).toBe(61);
    expect(first.pageInfo.hasNextPage).toBe(true);
    expect(first.pageInfo.hasPreviousPage).toBe(false);

    const second = await repository.listArtifacts(
      {
        cursor: requiredCursor(first.pageInfo.endCursor),
        direction: "next",
        limit: 10,
      },
      ownerId,
      now,
    );
    const firstIds = first.artifacts.map((item) => item.artifact.artifactId);
    const secondIds = second.artifacts.map((item) => item.artifact.artifactId);
    expect(secondIds).toHaveLength(10);
    expect(secondIds.some((id) => firstIds.includes(id))).toBe(false);

    const previous = await repository.listArtifacts(
      {
        cursor: requiredCursor(second.pageInfo.startCursor),
        direction: "previous",
        limit: 10,
      },
      ownerId,
      now,
    );
    expect(previous.artifacts.map((item) => item.artifact.artifactId)).toEqual(
      firstIds,
    );
    expect(previous.pageInfo.hasPreviousPage).toBe(false);
    expect(previous.pageInfo.hasNextPage).toBe(true);

    consoleArtifactListResponseSchema.parse({
      ...first,
      version: "filecheap-console-artifacts/1",
    });
  });

  test("keeps full-catalog aggregates on an empty stale-cursor page", async () => {
    let page = await repository.listArtifacts(
      { direction: "next", limit: 25 },
      ownerId,
      now,
    );
    while (page.pageInfo.hasNextPage) {
      page = await repository.listArtifacts(
        {
          cursor: requiredCursor(page.pageInfo.endCursor),
          direction: "next",
          limit: 25,
        },
        ownerId,
        now,
      );
    }
    const empty = await repository.listArtifacts(
      {
        cursor: requiredCursor(page.pageInfo.endCursor),
        direction: "next",
        limit: 25,
      },
      ownerId,
      now,
    );

    expect(empty.artifacts).toEqual([]);
    expect(empty.filteredTotal).toBe(61);
    expect(empty.overview.recordedCount).toBe(61);
    expect(empty.facets.producers.reduce((sum, facet) => sum + facet.count, 0))
      .toBe(61);
  });

  test("enforces owner, retention, filters, and literal LIKE escaping", async () => {
    const glyphrun = await repository.listArtifacts(
      { direction: "next", limit: 50, producer: "glyphrun" },
      ownerId,
      now,
    );
    expect(glyphrun.filteredTotal).toBe(30);
    expect(glyphrun.artifacts.every(
      (item) => item.artifact.producer.tool === "glyphrun",
    )).toBe(true);

    const literalPercent = await repository.listArtifacts(
      { direction: "next", limit: 50, q: "%" },
      ownerId,
      now,
    );
    expect(literalPercent.filteredTotal).toBe(0);
    expect(literalPercent.artifacts).toEqual([]);
    expect(glyphrun.artifacts.some(
      (item) => item.artifact.artifactId.includes("other"),
    )).toBe(false);
  });

  test("orders runs by effective timestamp when started_at is null", async () => {
    const first = await repository.listRuns(
      { direction: "next", limit: 9 },
      ownerId,
      now,
    );
    expect(first.runs).toHaveLength(9);
    expect(first.filteredTotal).toBe(43);
    expect(first.runs.some((item) => item.run.startedAt === undefined)).toBe(true);

    const second = await repository.listRuns(
      {
        cursor: requiredCursor(first.pageInfo.endCursor),
        direction: "next",
        limit: 9,
      },
      ownerId,
      now,
    );
    const previous = await repository.listRuns(
      {
        cursor: requiredCursor(second.pageInfo.startCursor),
        direction: "previous",
        limit: 9,
      },
      ownerId,
      now,
    );
    expect(previous.runs.map((item) => item.artifactId)).toEqual(
      first.runs.map((item) => item.artifactId),
    );

    consoleRunListResponseSchema.parse({
      ...first,
      version: "filecheap-console-runs/1",
    });
  });

  test("applies run facets and the measured effective-sort index", async () => {
    const failed = await repository.listRuns(
      { direction: "next", limit: 50, status: "failed" },
      ownerId,
      now,
    );
    expect(failed.filteredTotal).toBe(10);
    expect(failed.runs.every((item) => item.run.status === "failed")).toBe(true);

    const indexResult = await harness.pool.query<{ indexdef: string }>(`
      SELECT indexdef
      FROM pg_indexes
      WHERE schemaname = 'public'
        AND tablename = 'artifact_runs'
        AND indexname = 'artifact_runs_owner_sort_index'
    `);
    expect(indexResult.rows[0]?.indexdef).toContain(
      "COALESCE(started_at, created_at)",
    );
  });
});

type TestDatabase = ReturnType<typeof openPostgresTestDatabase>["database"];

async function seedCatalog(database: TestDatabase): Promise<void> {
  await database.insert(consoleUsers).values([
    {
      createdAt: now,
      email: "catalog-owner@example.invalid",
      id: ownerId,
      updatedAt: now,
    },
    {
      createdAt: now,
      email: "catalog-other@example.invalid",
      id: otherOwnerId,
      updatedAt: now,
    },
  ]);

  const artifactRows = Array.from({ length: 66 }, (_, offset) => {
    const ordinal = offset + 1;
    const ownerAccountId = ordinal > 63 ? otherOwnerId : ownerId;
    const createdAt = new Date(
      now.getTime() - 2 * 24 * 60 * 60 * 1_000 - Math.floor(offset / 3) * 1_000,
    );
    const excluded = ordinal === 62 || ordinal === 63;
    return {
      artifactId: artifactId(ordinal, ownerAccountId === otherOwnerId),
      committedAt: createdAt,
      contentType: "application/zstd",
      createdAt,
      expiresAt: ordinal === 62
        ? new Date(now.getTime() - 60 * 60 * 1_000)
        : null,
      kind: ordinal % 2 === 0 ? "run.bundle" : "log.bundle",
      ownerAccountId,
      planExpiresAt: new Date(now.getTime() + 15 * 60 * 1_000),
      planToken: `00000000-0000-4000-8000-${String(ordinal).padStart(12, "0")}`,
      producer: {
        entrypoint: "runs/output",
        native_id: `native:${ordinal}`,
        native_schema: "urn:filecheap:test:run:v1",
        tool: ordinal % 2 === 0 ? "glyphrun" : "cairntrace",
        version: "1.0.0",
      },
      sha256: ordinal.toString(16).padStart(64, "0"),
      sizeBytes: 100 + ordinal,
      state: excluded && ordinal === 63 ? "deleted" : "committed",
      verification: "server-sha256",
    };
  });
  await database.insert(artifacts).values(artifactRows);

  const runRows = artifactRows.slice(0, 43).map((artifact, offset) => {
    const ordinal = offset + 1;
    const startedAt = ordinal % 4 === 0
      ? null
      : new Date(artifact.createdAt.getTime() - 2_000);
    const status = ordinal % 4 === 0
      ? "failed"
      : ordinal % 4 === 1
        ? "passed"
        : ordinal % 4 === 2
          ? "running"
          : "queued";
    return {
      artifactCount: 0,
      artifactId: artifact.artifactId,
      backend: "local",
      createdAt: artifact.createdAt,
      detectorName: ordinal % 2 === 0 ? "glyphrun-run" : "cairntrace-run",
      detectorVersion: "1.0.0",
      durationMs: 2_000,
      endedAt: artifact.createdAt,
      environment: "integration",
      errorKind: null,
      evidence: [],
      exitCode: status === "failed" ? 1 : 0,
      health: status === "failed" ? "degraded" : "ok",
      healthChanged: 0,
      healthDeclared: 0,
      healthEmpty: 0,
      healthMissing: 0,
      healthPresent: 0,
      healthReasons: [],
      nativeRunId: `native:${ordinal}`,
      nativeSchema: "urn:filecheap:test:run:v1",
      outcomeCount: 0,
      outcomes: [],
      ownerAccountId: artifact.ownerAccountId,
      producerTool: ordinal % 2 === 0 ? "glyphrun" : "cairntrace",
      runIndexSha256: (1_000 + ordinal).toString(16).padStart(64, "0"),
      schemaVersion: 1,
      seriesKey: `series_${String(ordinal % 5).padStart(16, "0")}`,
      sourceSha256: artifact.sha256,
      specName: `spec ${ordinal % 3}`,
      startedAt,
      status,
      stepCount: 0,
      updatedAt: artifact.createdAt,
    };
  });
  await database.insert(artifactRuns).values(runRows);
}

function artifactId(ordinal: number, otherOwner: boolean): string {
  const scope = otherOwner ? "other" : "owner";
  return `art_catalog_${scope}_${String(ordinal).padStart(16, "0")}`;
}

function requiredCursor(value: string | null): string {
  if (!value) throw new Error("Expected a catalog cursor");
  return value;
}
