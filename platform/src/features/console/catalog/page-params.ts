import {
  consoleArtifactListQuerySchema,
  consoleRunListQuerySchema,
  type ConsoleArtifactListQuery,
  type ConsoleRunListQuery,
} from "@/features/console/catalog/contracts";
import { decodeConsoleCatalogCursor } from "@/features/console/catalog/cursor";
import type { ArtifactGroupBy } from "@/features/console/ui/artifact-types";

export type ConsolePageSearchParams = Record<
  string,
  string | string[] | undefined
>;

export type ConsoleArtifactPageState = Readonly<{
  groupBy: ArtifactGroupBy;
  page: number;
  query: ConsoleArtifactListQuery;
}>;

export type ConsoleRunPageState = Readonly<{
  page: number;
  query: ConsoleRunListQuery;
}>;

export function artifactPageState(
  searchParams: ConsolePageSearchParams,
): ConsoleArtifactPageState {
  const parsed = consoleArtifactListQuerySchema.safeParse({
    cursor: first(searchParams.cursor),
    direction: first(searchParams.direction),
    kind: first(searchParams.kind),
    limit: numberParam(searchParams.limit),
    producer: first(searchParams.producer),
    q: first(searchParams.q),
  });
  const query = parsed.success
    ? validCursor(parsed.data, "artifacts")
    : consoleArtifactListQuerySchema.parse({});
  return {
    groupBy: first(searchParams.groupBy) === "kind" ? "kind" : "producer",
    page: query.cursor ? pageNumber(searchParams.page) : 1,
    query,
  };
}

export function runPageState(
  searchParams: ConsolePageSearchParams,
): ConsoleRunPageState {
  const parsed = consoleRunListQuerySchema.safeParse({
    cursor: first(searchParams.cursor),
    direction: first(searchParams.direction),
    from: first(searchParams.from),
    health: first(searchParams.health),
    limit: numberParam(searchParams.limit),
    producer: first(searchParams.producer),
    q: first(searchParams.q),
    status: first(searchParams.status),
    to: first(searchParams.to),
  });
  const query = parsed.success
    ? validCursor(parsed.data, "runs")
    : consoleRunListQuerySchema.parse({});
  return { page: query.cursor ? pageNumber(searchParams.page) : 1, query };
}

function validCursor<Query extends ConsoleArtifactListQuery | ConsoleRunListQuery>(
  query: Query,
  scope: "artifacts" | "runs",
): Query {
  if (!query.cursor) return query;
  try {
    decodeConsoleCatalogCursor(query.cursor, scope);
    return query;
  } catch {
    return { ...query, cursor: undefined, direction: "next" };
  }
}

function first(value: string | string[] | undefined): string | undefined {
  return Array.isArray(value) ? value[0] : value;
}

function numberParam(value: string | string[] | undefined): number | undefined {
  const raw = first(value);
  return raw === undefined ? undefined : Number(raw);
}

function pageNumber(value: string | string[] | undefined): number {
  const parsed = Number(first(value));
  return Number.isSafeInteger(parsed) && parsed >= 1 && parsed <= 100_000
    ? parsed
    : 1;
}
