import { describe, expect, test } from "bun:test";

import type { ArtifactSummary } from "@/features/artifacts/contracts";
import {
  consoleArtifactListQuerySchema,
  consoleArtifactListResponseSchema,
  consoleRunListQuerySchema,
  consoleRunListResponseSchema,
} from "@/features/console/catalog/contracts";
import {
  InMemoryConsoleCatalogRepository,
  type InMemoryConsoleArtifact,
  type InMemoryConsoleRun,
} from "@/features/console/catalog/in-memory-repository";
import { ConsoleCatalogService } from "@/features/console/catalog/service";
import type { RunSummary } from "@/features/runs/contracts";

const now = new Date("2026-07-26T18:00:00.000Z");
const owner = "acc_owner123";
const otherOwner = "acc_other123";

describe("console catalog service", () => {
  test("pages more than 100 retained artifacts chronologically in both directions", async () => {
    const artifacts = [
      ...Array.from({ length: 125 }, (_, index) => artifactRecord(index, owner)),
      artifactRecord(200, owner, { expired: true }),
      ...Array.from({ length: 3 }, (_, index) => artifactRecord(300 + index, otherOwner)),
    ];
    const service = catalog(artifacts);

    const first = await service.listArtifacts(artifactQuery({ limit: 25 }), owner);
    expect(consoleArtifactListResponseSchema.parse(first)).toEqual(first);
    expect(first.artifacts).toHaveLength(25);
    expect(first.filteredTotal).toBe(125);
    expect(first.overview).toMatchObject({
      recordedCount: 125,
      totalBytes: 125 * 100,
      transferableCount: 125,
      verifiedCount: 125,
    });
    expect(first.pageInfo).toMatchObject({
      hasNextPage: true,
      hasPreviousPage: false,
    });
    expect(first.artifacts[0]?.artifact.producer.native_id).toBe("needle-0");

    const second = await service.listArtifacts(artifactQuery({
      cursor: first.pageInfo.endCursor!,
      limit: 25,
    }), owner);
    expect(second.artifacts).toHaveLength(25);
    expect(second.pageInfo).toMatchObject({
      hasNextPage: true,
      hasPreviousPage: true,
    });
    const firstIds = new Set(first.artifacts.map(idOfArtifact));
    expect(second.artifacts.some((artifact) => firstIds.has(idOfArtifact(artifact)))).toBe(false);

    const previous = await service.listArtifacts(artifactQuery({
      cursor: second.pageInfo.startCursor!,
      direction: "previous",
      limit: 25,
    }), owner);
    expect(previous.artifacts.map(idOfArtifact)).toEqual(first.artifacts.map(idOfArtifact));
    expect(previous.pageInfo).toMatchObject({
      hasNextPage: true,
      hasPreviousPage: false,
    });

    const visited = [...first.artifacts, ...second.artifacts];
    let current = second;
    while (current.pageInfo.hasNextPage) {
      current = await service.listArtifacts(artifactQuery({
        cursor: current.pageInfo.endCursor!,
        limit: 25,
      }), owner);
      visited.push(...current.artifacts);
    }
    expect(visited).toHaveLength(125);
    expect(new Set(visited.map(idOfArtifact)).size).toBe(125);
    expect(current.pageInfo).toMatchObject({
      hasNextPage: false,
      hasPreviousPage: true,
    });
  });

  test("uses artifact id as the deterministic tie-breaker for equal creation timestamps", async () => {
    const tiedAt = new Date("2026-07-26T17:00:00.000Z");
    const records = [artifactRecord(1, owner), artifactRecord(3, owner), artifactRecord(2, owner)]
      .map((record) => ({ ...record, createdAt: tiedAt }));
    const service = catalog(records);

    const result = await service.listArtifacts(artifactQuery({}), owner);
    expect(result.artifacts.map(idOfArtifact)).toEqual([
      artifactIdFor(3),
      artifactIdFor(2),
      artifactIdFor(1),
    ]);
  });

  test("scopes artifact totals and facets to the owner and active retention window", async () => {
    const artifacts = [
      artifactRecord(0, owner, { kind: "chalupa.log-chunk", producer: "chalupa" }),
      artifactRecord(1, owner, { kind: "glyphrun.run", producer: "glyphrun" }),
      artifactRecord(2, owner, { expired: true, kind: "glyphrun.run", producer: "glyphrun" }),
      artifactRecord(3, otherOwner, { kind: "cairntrace.run", producer: "cairntrace" }),
      artifactRecord(4, owner, { kind: "glyphrun.run", producer: "glyphrun", state: "planned" }),
    ];
    const service = catalog(artifacts);

    const result = await service.listArtifacts(artifactQuery({ limit: 50 }), owner);
    expect(result.filteredTotal).toBe(2);
    expect(result.overview.recordedCount).toBe(2);
    expect(result.facets.kinds).toEqual([
      { count: 1, value: "chalupa.log-chunk" },
      { count: 1, value: "glyphrun.run" },
    ]);
    expect(result.facets.producers).toEqual([
      { count: 1, value: "chalupa" },
      { count: 1, value: "glyphrun" },
    ]);
  });

  test("applies artifact q, kind, and producer filters before exact totals", async () => {
    const artifacts = Array.from({ length: 125 }, (_, index) => artifactRecord(index, owner, {
      kind: index % 2 === 0 ? "chalupa.log-chunk" : "glyphrun.run",
      producer: index % 2 === 0 ? "chalupa" : "glyphrun",
    }));
    const service = catalog(artifacts);

    const result = await service.listArtifacts(artifactQuery({
      kind: "chalupa.log-chunk",
      producer: "chalupa",
      q: "needle-112",
    }), owner);
    expect(result.filteredTotal).toBe(1);
    expect(result.artifacts[0]?.artifact.producer.native_id).toBe("needle-112");
    // Facets describe the complete retained catalog, not only the active page filter.
    expect(result.facets.producers).toContainEqual({ count: 63, value: "chalupa" });
    expect(result.facets.producers).toContainEqual({ count: 62, value: "glyphrun" });
  });

  test("pages and filters more than 100 retained runs with exact overview metrics", async () => {
    const runs = [
      ...Array.from({ length: 112 }, (_, index) => runRecord(index, owner)),
      runRecord(200, owner, { expired: true }),
      runRecord(300, otherOwner),
    ];
    const service = catalog([], runs);

    const first = await service.listRuns(runQuery({ limit: 25 }), owner);
    expect(consoleRunListResponseSchema.parse(first)).toEqual(first);
    expect(first.runs).toHaveLength(25);
    expect(first.filteredTotal).toBe(112);
    expect(first.overview.recordedCount).toBe(112);
    expect(first.overview.indexedEvidenceCount).toBe(112);
    expect(first.pageInfo.hasNextPage).toBe(true);

    const filtered = await service.listRuns(runQuery({ q: "preview-107" }), owner);
    expect(filtered.filteredTotal).toBe(1);
    expect(filtered.runs[0]?.run.environment).toBe("preview-107");

    const second = await service.listRuns(runQuery({
      cursor: first.pageInfo.endCursor!,
      limit: 25,
    }), owner);
    const previous = await service.listRuns(runQuery({
      cursor: second.pageInfo.startCursor!,
      direction: "previous",
      limit: 25,
    }), owner);
    expect(previous.runs.map((run) => run.artifactId)).toEqual(
      first.runs.map((run) => run.artifactId),
    );
  });

  test("rejects a cursor from the other catalog scope", async () => {
    const service = catalog([artifactRecord(0, owner)], [runRecord(0, owner)]);
    const artifacts = await service.listArtifacts(artifactQuery({}), owner);

    await expect(service.listRuns(runQuery({
      cursor: artifacts.pageInfo.endCursor!,
    }), owner)).rejects.toMatchObject({ code: "invalid_cursor", status: 422 });
  });
});

function catalog(
  artifacts: readonly InMemoryConsoleArtifact[] = [],
  runs: readonly InMemoryConsoleRun[] = [],
): ConsoleCatalogService {
  return new ConsoleCatalogService(
    new InMemoryConsoleCatalogRepository(artifacts, runs),
    () => now,
  );
}

function artifactQuery(
  input: Partial<Parameters<ConsoleCatalogService["listArtifacts"]>[0]>,
) {
  return consoleArtifactListQuerySchema.parse(input);
}

function runQuery(
  input: Partial<Parameters<ConsoleCatalogService["listRuns"]>[0]>,
) {
  return consoleRunListQuerySchema.parse(input);
}

function artifactRecord(
  index: number,
  ownerAccountId: string,
  options: {
    expired?: boolean;
    kind?: string;
    producer?: string;
    state?: ArtifactSummary["artifact"]["state"];
  } = {},
): InMemoryConsoleArtifact {
  const artifactId = artifactIdFor(index);
  const committedAt = new Date(now.getTime() - index * 60_000);
  const producer = options.producer ?? "chalupa";
  const summary: ArtifactSummary = {
    artifact: {
      artifactId,
      committedAt: committedAt.toISOString(),
      contentType: "application/zstd",
      expiresAt: options.expired
        ? new Date(now.getTime() - 1).toISOString()
        : new Date(now.getTime() + 7 * 86_400_000).toISOString(),
      kind: options.kind ?? "chalupa.log-chunk",
      producer: {
        entrypoint: `service/stdout/${index}.zst`,
        native_id: `needle-${index}`,
        native_schema: `urn:${producer}.dev:artifact:v1`,
        tool: producer,
      },
      sha256: index.toString(16).padStart(64, "0").slice(-64),
      sizeBytes: 100,
      state: options.state ?? "committed",
      verification: "server-sha256",
    },
    artifactRef: {
      $schema: "urn:filecheap.dev:artifact-ref:v1",
      artifact_id: artifactId,
      kind: options.kind ?? "chalupa.log-chunk",
      producer: {
        entrypoint: `service/stdout/${index}.zst`,
        native_id: `needle-${index}`,
        native_schema: `urn:${producer}.dev:artifact:v1`,
        tool: producer,
      },
      provider: "fcheap-cloud",
      uri: `fcheap://cloud/vaults/private/artifacts/${artifactId}`,
      version: 1,
    },
  };
  return { createdAt: committedAt, ownerAccountId, summary };
}

function runRecord(
  index: number,
  ownerAccountId: string,
  options: { expired?: boolean } = {},
): InMemoryConsoleRun {
  const artifactId = artifactIdFor(index);
  const startedAt = new Date(now.getTime() - index * 60_000).toISOString();
  const status = index % 3 === 0 ? "passed" : index % 3 === 1 ? "running" : "failed";
  const summary: RunSummary = {
    artifactId,
    counts: { artifacts: 1, outcomes: 1, steps: 1 },
    createdAt: startedAt,
    detector: { name: "glyphrun-run", version: "1" },
    evidence: [{
      inspectability: "metadata-only",
      integrity: "declared",
      medium: "structured-text",
      path: "run.json",
      presence: "present",
      role: "run-overview",
      sensitivity: "metadata-safe",
    }],
    health: {
      changed: 0,
      declared: 1,
      empty: 0,
      missing: 0,
      present: 1,
      reasons: [],
      state: index % 4 === 0 ? "ok" : "degraded",
    },
    outcomes: [{ id: `outcome-${index}`, status: status === "passed" ? "passed" : "failed" }],
    producer: {
      native_id: `run-${index}`,
      native_schema: "urn:glyphrun.dev:run:v1",
      tool: "glyphrun",
    },
    run: {
      environment: `preview-${index}`,
      nativeId: `run-${index}`,
      seriesKey: `series-key-${String(index % 5).padStart(16, "0")}`,
      specName: `spec-${index}`,
      startedAt,
      status,
    },
    runIndexSha256: "b".repeat(64),
    source: {
      contentType: "application/gzip",
      kind: "glyphrun.run",
      sha256: "a".repeat(64),
      sizeBytes: 100,
    },
    updatedAt: startedAt,
  };
  return {
    expiresAt: options.expired ? new Date(now.getTime() - 1) : null,
    ownerAccountId,
    summary,
  };
}

function artifactIdFor(index: number): string {
  return `art_${index.toString().padStart(20, "0")}`;
}

function idOfArtifact(summary: ArtifactSummary): string {
  return summary.artifact.artifactId;
}
