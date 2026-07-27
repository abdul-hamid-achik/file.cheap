import type { ArtifactSummary } from "@/features/artifacts/contracts";
import type {
  ConsoleArtifactListQuery,
  ConsoleArtifactListResponse,
  ConsoleCatalogFacetValue,
  ConsoleCatalogPageInfo,
  ConsoleRunListQuery,
  ConsoleRunListResponse,
} from "@/features/console/catalog/contracts";
import {
  decodeConsoleCatalogCursor,
  encodeConsoleCatalogCursor,
  type ConsoleCatalogCursor,
  type ConsoleCatalogScope,
} from "@/features/console/catalog/cursor";
import type { ConsoleCatalogRepository } from "@/features/console/catalog/repository";
import type { RunSummary } from "@/features/runs/contracts";

const expiringSoonMilliseconds = 24 * 60 * 60 * 1_000;

export type InMemoryConsoleArtifact = Readonly<{
  createdAt: Date;
  ownerAccountId: string;
  summary: ArtifactSummary;
}>;

export type InMemoryConsoleRun = Readonly<{
  expiresAt: Date | null;
  ownerAccountId: string;
  summary: RunSummary;
}>;

type Positioned<Row> = Readonly<{
  cursor: ConsoleCatalogCursor;
  row: Row;
}>;

export class InMemoryConsoleCatalogRepository implements ConsoleCatalogRepository {
  constructor(
    private readonly artifacts: readonly InMemoryConsoleArtifact[] = [],
    private readonly runs: readonly InMemoryConsoleRun[] = [],
  ) {}

  async listArtifacts(
    query: ConsoleArtifactListQuery,
    ownerAccountId: string,
    now: Date,
  ): Promise<Omit<ConsoleArtifactListResponse, "version">> {
    const retained = this.artifacts.filter((record) =>
      record.ownerAccountId === ownerAccountId &&
      record.summary.artifact.state === "committed" &&
      isRetained(record.summary.artifact.expiresAt, now));
    const filtered = retained.filter((record) => artifactMatches(record.summary, query));
    const positioned = filtered.map((record): Positioned<ArtifactSummary> => ({
      cursor: {
        id: record.summary.artifact.artifactId,
        time: record.createdAt,
      },
      row: record.summary,
    }));
    const page = paginate(positioned, "artifacts", query);
    const horizon = new Date(now.getTime() + expiringSoonMilliseconds);

    return {
      artifacts: page.rows,
      facets: {
        kinds: facets(retained.map((record) => record.summary.artifact.kind)),
        producers: facets(retained.map((record) => record.summary.artifact.producer.tool)),
      },
      filteredTotal: filtered.length,
      overview: {
        expiringSoonCount: retained.filter((record) => {
          const expiresAt = record.summary.artifact.expiresAt;
          return Boolean(expiresAt && new Date(expiresAt) > now && new Date(expiresAt) <= horizon);
        }).length,
        recordedCount: retained.length,
        totalBytes: retained.reduce((total, record) => total + record.summary.artifact.sizeBytes, 0),
        transferableCount: retained.length,
        verifiedCount: retained.filter((record) => record.summary.artifact.verification === "server-sha256").length,
      },
      pageInfo: page.pageInfo,
    };
  }

  async listRuns(
    query: ConsoleRunListQuery,
    ownerAccountId: string,
    now: Date,
  ): Promise<Omit<ConsoleRunListResponse, "version">> {
    const retained = this.runs.filter((record) =>
      record.ownerAccountId === ownerAccountId &&
      (record.expiresAt === null || record.expiresAt > now));
    const filtered = retained.filter((record) => runMatches(record.summary, query));
    const positioned = filtered.map((record): Positioned<RunSummary> => ({
      cursor: {
        id: record.summary.artifactId,
        time: new Date(record.summary.run.startedAt ?? record.summary.createdAt),
      },
      row: record.summary,
    }));
    const page = paginate(positioned, "runs", query);

    return {
      facets: {
        health: facets(retained.map((record) => record.summary.health.state)),
        producers: facets(retained.map((record) => record.summary.producer.tool)),
        statuses: facets(retained.map((record) => record.summary.run.status)),
      },
      filteredTotal: filtered.length,
      overview: {
        activeCount: retained.filter((record) => ["queued", "running"].includes(record.summary.run.status)).length,
        healthyCount: retained.filter((record) => record.summary.health.state === "ok").length,
        indexedEvidenceCount: retained.reduce((total, record) => total + record.summary.evidence.length, 0),
        passedCount: retained.filter((record) => record.summary.run.status === "passed").length,
        recordedCount: retained.length,
      },
      pageInfo: page.pageInfo,
      runs: page.rows,
    };
  }
}

function paginate<Row>(
  positioned: readonly Positioned<Row>[],
  scope: ConsoleCatalogScope,
  query: Readonly<{
    cursor?: string;
    direction: "next" | "previous";
    limit: number;
  }>,
): { pageInfo: ConsoleCatalogPageInfo; rows: Row[] } {
  const cursor = query.cursor
    ? decodeConsoleCatalogCursor(query.cursor, scope)
    : undefined;
  const ordered = [...positioned].sort((left, right) =>
    compareNewestFirst(left.cursor, right.cursor));
  const candidates = cursor
    ? ordered.filter((record) => {
        const order = compareNewestFirst(record.cursor, cursor);
        return query.direction === "previous" ? order < 0 : order > 0;
      })
    : ordered;
  if (query.direction === "previous") candidates.reverse();
  const hasExtra = candidates.length > query.limit;
  const selected = candidates.slice(0, query.limit);
  if (query.direction === "previous") selected.reverse();
  const first = selected.at(0);
  const last = selected.at(-1);
  return {
    pageInfo: {
      endCursor: last ? encodeConsoleCatalogCursor(scope, last.cursor) : null,
      hasNextPage: query.direction === "previous" ? Boolean(cursor) : hasExtra,
      hasPreviousPage: query.direction === "previous" ? hasExtra : Boolean(cursor),
      startCursor: first ? encodeConsoleCatalogCursor(scope, first.cursor) : null,
    },
    rows: selected.map((record) => record.row),
  };
}

function compareNewestFirst(
  left: ConsoleCatalogCursor,
  right: ConsoleCatalogCursor,
): number {
  const byTime = right.time.getTime() - left.time.getTime();
  return byTime || right.id.localeCompare(left.id);
}

function artifactMatches(
  summary: ArtifactSummary,
  query: ConsoleArtifactListQuery,
): boolean {
  const artifact = summary.artifact;
  const needle = query.q?.toLocaleLowerCase();
  const searchable = [
    artifact.artifactId,
    artifact.kind,
    artifact.producer.tool,
    artifact.producer.native_id,
    artifact.producer.entrypoint,
  ];
  return (!query.kind || artifact.kind === query.kind) &&
    (!query.producer || artifact.producer.tool === query.producer) &&
    (!needle || searchable.some((value) => value?.toLocaleLowerCase().includes(needle)));
}

function runMatches(summary: RunSummary, query: ConsoleRunListQuery): boolean {
  const sortTime = new Date(summary.run.startedAt ?? summary.createdAt);
  const needle = query.q?.toLocaleLowerCase();
  const searchable = [
    summary.artifactId,
    summary.producer.tool,
    summary.run.nativeId,
    summary.run.seriesKey,
    summary.run.specName,
    summary.run.environment,
    summary.run.backend,
  ];
  return (!query.status || summary.run.status === query.status) &&
    (!query.health || summary.health.state === query.health) &&
    (!query.producer || summary.producer.tool === query.producer) &&
    (!query.from || sortTime >= new Date(query.from)) &&
    (!query.to || sortTime <= new Date(query.to)) &&
    (!needle || searchable.some((value) => value?.toLocaleLowerCase().includes(needle)));
}

function facets(values: readonly string[]): ConsoleCatalogFacetValue[] {
  const counts = new Map<string, number>();
  for (const value of values) counts.set(value, (counts.get(value) ?? 0) + 1);
  return [...counts.entries()]
    .map(([value, count]) => ({ count, value }))
    .sort((left, right) => right.count - left.count || left.value.localeCompare(right.value));
}

function isRetained(expiresAt: string | null, now: Date): boolean {
  return expiresAt === null || new Date(expiresAt) > now;
}
