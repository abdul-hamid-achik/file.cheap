import { describe, expect, test } from "bun:test";
import { type SQL } from "drizzle-orm";
import { PgDialect } from "drizzle-orm/pg-core";

import {
  consoleArtifactListQuerySchema,
  consoleRunListQuerySchema,
} from "@/features/console/catalog/contracts";
import {
  decodeConsoleCatalogCursor,
  encodeConsoleCatalogCursor,
} from "@/features/console/catalog/cursor";
import {
  type ConsoleCatalogDatabase,
  DrizzleConsoleCatalogRepository,
} from "@/platform/database/console-catalog-repository";

const now = new Date("2026-07-26T18:00:00.000Z");
const owner = "acc_owner123";

describe("DrizzleConsoleCatalogRepository", () => {
  test("returns a stale artifact cursor page from one consistent statement", async () => {
    const database = new RecordingDatabase({
      expiringSoonCount: "0",
      filteredTotal: "4",
      hasExtra: false,
      items: [],
      kindFacets: [{ count: 4, value: "glyphrun.run" }],
      producerFacets: [{ count: 4, value: "glyphrun" }],
      recordedCount: "4",
      totalBytes: "4096",
      verifiedCount: "4",
    });
    const repository = new DrizzleConsoleCatalogRepository(database);
    const cursor = encodeConsoleCatalogCursor("artifacts", {
      id: "art_0000000000000001",
      time: new Date("2026-07-25T18:00:00.000Z"),
    });

    const result = await repository.listArtifacts(
      consoleArtifactListQuerySchema.parse({ cursor, limit: 25 }),
      owner,
      now,
    );

    expect(database.queries).toHaveLength(1);
    expect(database.queryText()).toContain("with page_candidates as");
    expect(database.queryText()).toContain('as "filteredtotal"');
    expect(result.artifacts).toEqual([]);
    expect(result.filteredTotal).toBe(4);
    expect(result.overview.recordedCount).toBe(4);
    expect(result.pageInfo).toEqual({
      endCursor: null,
      hasNextPage: false,
      hasPreviousPage: true,
      startCursor: null,
    });
  });

  test("maps a run with a null started_at and cursors by its effective sort time", async () => {
    const createdAt = "2026-07-26T17:30:00.000Z";
    const artifactId = "art_0000000000000002";
    const database = new RecordingDatabase({
      activeCount: "0",
      filteredTotal: "1",
      hasExtra: false,
      healthFacets: [{ count: 1, value: "ok" }],
      healthyCount: "1",
      indexedEvidenceCount: "0",
      items: [{
        artifact: {
          artifactId,
          contentType: "application/zstd",
          kind: "glyphrun.run",
          producer: {
            native_id: "run-2",
            native_schema: "urn:glyphrun.dev:run:v1",
            tool: "glyphrun",
          },
          sizeBytes: 512,
        },
        run: {
          artifactCount: 0,
          backend: null,
          createdAt,
          detectorName: "glyphrun-run",
          detectorVersion: "1",
          durationMs: null,
          endedAt: null,
          environment: null,
          errorKind: null,
          evidence: [],
          exitCode: null,
          health: "ok",
          healthChanged: 0,
          healthDeclared: 0,
          healthEmpty: 0,
          healthMissing: 0,
          healthPresent: 0,
          healthReasons: [],
          nativeRunId: "run-2",
          outcomeCount: 0,
          outcomes: [],
          runIndexSha256: "b".repeat(64),
          seriesKey: "series-key-0000000002",
          sourceSha256: "a".repeat(64),
          specName: null,
          startedAt: null,
          status: "passed",
          stepCount: 0,
          updatedAt: createdAt,
        },
        sortTime: createdAt,
      }],
      passedCount: "1",
      producerFacets: [{ count: 1, value: "glyphrun" }],
      recordedCount: "1",
      statusFacets: [{ count: 1, value: "passed" }],
    });
    const repository = new DrizzleConsoleCatalogRepository(database);

    const result = await repository.listRuns(
      consoleRunListQuerySchema.parse({ limit: 25 }),
      owner,
      now,
    );

    expect(database.queries).toHaveLength(1);
    expect(database.queryText()).toContain("coalesce(");
    expect(result.runs[0]?.run.startedAt).toBeUndefined();
    expect(result.runs[0]?.artifactId).toBe(artifactId);
    expect(decodeConsoleCatalogCursor(
      result.pageInfo.endCursor!,
      "runs",
    )).toEqual({ id: artifactId, time: new Date(createdAt) });
  });
});

class RecordingDatabase implements ConsoleCatalogDatabase {
  readonly queries: SQL[] = [];

  constructor(private readonly row: Record<string, unknown>) {}

  execute(query: SQL): PromiseLike<{ rows: unknown[] }> {
    this.queries.push(query);
    return Promise.resolve({ rows: [this.row] });
  }

  queryText(): string {
    const query = this.queries[0];
    if (!query) throw new Error("No query was recorded");
    return new PgDialect().sqlToQuery(query).sql.toLowerCase();
  }
}
