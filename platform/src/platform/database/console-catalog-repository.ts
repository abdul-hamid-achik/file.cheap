import {
  and,
  eq,
  gt,
  gte,
  isNull,
  lt,
  lte,
  or,
  sql,
  type SQL,
} from "drizzle-orm";

import type { ArtifactPlanInput, ArtifactSummary } from "@/features/artifacts/contracts";
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
  type ConsoleCatalogScope,
} from "@/features/console/catalog/cursor";
import type { ConsoleCatalogRepository } from "@/features/console/catalog/repository";
import type { RunSummary } from "@/features/runs/contracts";
import { getDatabase } from "@/platform/database/client";
import { artifactRuns, artifacts } from "@/platform/database/schema";

const expiringSoonMilliseconds = 24 * 60 * 60 * 1_000;

type TimestampValue = Date | string;

type ArtifactSnapshotRow = {
  artifactId: string;
  committedAt: TimestampValue | null;
  contentType: string;
  expiresAt: TimestampValue | null;
  kind: string;
  producer: unknown;
  sha256: string;
  sizeBytes: unknown;
  sortTime: TimestampValue;
};

type RunSnapshotRow = {
  artifact: {
    artifactId: string;
    contentType: string;
    kind: string;
    producer: unknown;
    sizeBytes: unknown;
  };
  run: {
    artifactCount: unknown;
    backend: string | null;
    createdAt: TimestampValue;
    detectorName: string;
    detectorVersion: string;
    durationMs: unknown;
    endedAt: TimestampValue | null;
    environment: string | null;
    errorKind: string | null;
    evidence: unknown;
    exitCode: unknown;
    health: string;
    healthChanged: unknown;
    healthDeclared: unknown;
    healthEmpty: unknown;
    healthMissing: unknown;
    healthPresent: unknown;
    healthReasons: unknown;
    nativeRunId: string;
    outcomeCount: unknown;
    outcomes: unknown;
    runIndexSha256: string;
    seriesKey: string;
    sourceSha256: string;
    specName: string | null;
    startedAt: TimestampValue | null;
    status: string;
    stepCount: unknown;
    updatedAt: TimestampValue;
  };
  sortTime: TimestampValue;
};

type ArtifactCatalogSnapshot = {
  expiringSoonCount: unknown;
  filteredTotal: unknown;
  hasExtra: boolean;
  items: ArtifactSnapshotRow[];
  kindFacets: { count: unknown; value: string | null }[];
  producerFacets: { count: unknown; value: string | null }[];
  recordedCount: unknown;
  totalBytes: unknown;
  verifiedCount: unknown;
};

type RunCatalogSnapshot = {
  activeCount: unknown;
  filteredTotal: unknown;
  hasExtra: boolean;
  healthFacets: { count: unknown; value: string | null }[];
  healthyCount: unknown;
  indexedEvidenceCount: unknown;
  items: RunSnapshotRow[];
  passedCount: unknown;
  producerFacets: { count: unknown; value: string | null }[];
  recordedCount: unknown;
  statusFacets: { count: unknown; value: string | null }[];
};

export interface ConsoleCatalogDatabase {
  execute(query: SQL): PromiseLike<{ rows: unknown[] }>;
}

export class DrizzleConsoleCatalogRepository implements ConsoleCatalogRepository {
  constructor(private readonly db: ConsoleCatalogDatabase = getDatabase()) {}

  async listArtifacts(
    query: ConsoleArtifactListQuery,
    ownerAccountId: string,
    now: Date,
  ): Promise<Omit<ConsoleArtifactListResponse, "version">> {
    const sortTime = artifacts.createdAt;
    const cursor = query.cursor
      ? decodeConsoleCatalogCursor(query.cursor, "artifacts")
      : undefined;
    const filtered = artifactPredicate(query, ownerAccountId, now);
    const retained = retainedArtifactPredicate(ownerAccountId, now);
    const cursorPredicate = cursor
      ? query.direction === "previous"
        ? or(
            gt(sortTime, cursor.time),
            and(eq(sortTime, cursor.time), gt(artifacts.artifactId, cursor.id)),
          )
        : or(
            lt(sortTime, cursor.time),
            and(eq(sortTime, cursor.time), lt(artifacts.artifactId, cursor.id)),
          )
      : undefined;
    const horizon = new Date(now.getTime() + expiringSoonMilliseconds);
    // neon-http has no callback transactions. Keeping the page and every
    // aggregate in one statement gives the response one PostgreSQL snapshot.
    const snapshotResult = await this.db.execute(sql`
      WITH page_candidates AS (
        SELECT
          ${artifactSnapshotJson()} AS item,
          ${sortTime} AS sort_time,
          ${artifacts.artifactId} AS sort_id
        FROM ${artifacts}
        WHERE ${and(filtered, cursorPredicate) ?? sql`true`}
        ORDER BY ${pageCandidateOrder(query.direction)}
        LIMIT ${query.limit + 1}
      ), page_rows AS (
        SELECT item, sort_time, sort_id
        FROM page_candidates
        ORDER BY ${pageCandidateOrder(query.direction)}
        LIMIT ${query.limit}
      ), overview AS (
        SELECT
          count(*) FILTER (
            WHERE ${artifacts.expiresAt} > ${now}
              AND ${artifacts.expiresAt} <= ${horizon}
          ) AS expiring_soon_count,
          count(*) AS recorded_count,
          coalesce(sum(${artifacts.sizeBytes}), 0) AS total_bytes,
          count(*) FILTER (
            WHERE ${artifacts.verification} = 'server-sha256'
          ) AS verified_count
        FROM ${artifacts}
        WHERE ${retained ?? sql`true`}
      ), producer_facets AS (
        SELECT ${artifacts.producer}->>'tool' AS value, count(*) AS count
        FROM ${artifacts}
        WHERE ${retained ?? sql`true`}
        GROUP BY ${artifacts.producer}->>'tool'
      ), kind_facets AS (
        SELECT ${artifacts.kind} AS value, count(*) AS count
        FROM ${artifacts}
        WHERE ${retained ?? sql`true`}
        GROUP BY ${artifacts.kind}
      )
      SELECT
        coalesce((
          SELECT jsonb_agg(item ORDER BY sort_time DESC, sort_id DESC)
          FROM page_rows
        ), '[]'::jsonb) AS "items",
        ((SELECT count(*) FROM page_candidates) > ${query.limit}) AS "hasExtra",
        (SELECT count(*) FROM ${artifacts} WHERE ${filtered ?? sql`true`})
          AS "filteredTotal",
        overview.expiring_soon_count AS "expiringSoonCount",
        overview.recorded_count AS "recordedCount",
        overview.total_bytes AS "totalBytes",
        overview.verified_count AS "verifiedCount",
        ${facetSnapshot("producer_facets")} AS "producerFacets",
        ${facetSnapshot("kind_facets")} AS "kindFacets"
      FROM overview
    `);
    const snapshot = requiredRow(
      snapshotResult.rows,
      "artifact catalog snapshot",
    ) as ArtifactCatalogSnapshot;
    const selectedRows = snapshot.items.map((item) => ({
      artifact: mapArtifact(item),
      sortTime: timestampValue(item.sortTime, "artifact sort time"),
    }));

    return {
      artifacts: selectedRows.map(({ artifact }) => artifact),
      facets: {
        kinds: mapFacets(snapshot.kindFacets),
        producers: mapFacets(snapshot.producerFacets),
      },
      filteredTotal: integerValue(snapshot.filteredTotal, "artifact total"),
      overview: {
        expiringSoonCount: integerValue(snapshot.expiringSoonCount, "expiring artifact total"),
        recordedCount: integerValue(snapshot.recordedCount, "recorded artifact total"),
        totalBytes: integerValue(snapshot.totalBytes, "artifact bytes"),
        transferableCount: integerValue(snapshot.recordedCount, "transferable artifact total"),
        verifiedCount: integerValue(snapshot.verifiedCount, "verified artifact total"),
      },
      pageInfo: pageInfo(
        "artifacts",
        selectedRows,
        ({ artifact, sortTime: time }) => ({ id: artifact.artifact.artifactId, time }),
        query.direction,
        snapshot.hasExtra,
        Boolean(cursor),
      ),
    };
  }

  async listRuns(
    query: ConsoleRunListQuery,
    ownerAccountId: string,
    now: Date,
  ): Promise<Omit<ConsoleRunListResponse, "version">> {
    const sortTime = sql<Date>`coalesce(${artifactRuns.startedAt}, ${artifactRuns.createdAt})`;
    const cursor = query.cursor
      ? decodeConsoleCatalogCursor(query.cursor, "runs")
      : undefined;
    const filtered = runPredicate(query, ownerAccountId, now, sortTime);
    const retained = retainedRunPredicate(ownerAccountId, now);
    const cursorPredicate = cursor
      ? query.direction === "previous"
        ? or(
            gt(sortTime, cursor.time),
            and(eq(sortTime, cursor.time), gt(artifactRuns.artifactId, cursor.id)),
          )
        : or(
            lt(sortTime, cursor.time),
            and(eq(sortTime, cursor.time), lt(artifactRuns.artifactId, cursor.id)),
          )
      : undefined;
    // This mirrors the artifact catalog's single-statement snapshot boundary.
    const snapshotResult = await this.db.execute(sql`
      WITH page_candidates AS (
        SELECT
          ${runSnapshotJson(sortTime)} AS item,
          ${sortTime} AS sort_time,
          ${artifactRuns.artifactId} AS sort_id
        FROM ${artifactRuns}
        INNER JOIN ${artifacts}
          ON ${artifacts.artifactId} = ${artifactRuns.artifactId}
        WHERE ${and(filtered, cursorPredicate) ?? sql`true`}
        ORDER BY ${pageCandidateOrder(query.direction)}
        LIMIT ${query.limit + 1}
      ), page_rows AS (
        SELECT item, sort_time, sort_id
        FROM page_candidates
        ORDER BY ${pageCandidateOrder(query.direction)}
        LIMIT ${query.limit}
      ), overview AS (
        SELECT
          count(*) FILTER (
            WHERE ${artifactRuns.status} IN ('queued', 'running')
          ) AS active_count,
          count(*) FILTER (
            WHERE ${artifactRuns.health} = 'ok'
          ) AS healthy_count,
          coalesce(sum(jsonb_array_length(${artifactRuns.evidence})), 0)
            AS indexed_evidence_count,
          count(*) FILTER (
            WHERE ${artifactRuns.status} = 'passed'
          ) AS passed_count,
          count(*) AS recorded_count
        FROM ${artifactRuns}
        INNER JOIN ${artifacts}
          ON ${artifacts.artifactId} = ${artifactRuns.artifactId}
        WHERE ${retained ?? sql`true`}
      ), producer_facets AS (
        SELECT ${artifactRuns.producerTool} AS value, count(*) AS count
        FROM ${artifactRuns}
        INNER JOIN ${artifacts}
          ON ${artifacts.artifactId} = ${artifactRuns.artifactId}
        WHERE ${retained ?? sql`true`}
        GROUP BY ${artifactRuns.producerTool}
      ), status_facets AS (
        SELECT ${artifactRuns.status} AS value, count(*) AS count
        FROM ${artifactRuns}
        INNER JOIN ${artifacts}
          ON ${artifacts.artifactId} = ${artifactRuns.artifactId}
        WHERE ${retained ?? sql`true`}
        GROUP BY ${artifactRuns.status}
      ), health_facets AS (
        SELECT ${artifactRuns.health} AS value, count(*) AS count
        FROM ${artifactRuns}
        INNER JOIN ${artifacts}
          ON ${artifacts.artifactId} = ${artifactRuns.artifactId}
        WHERE ${retained ?? sql`true`}
        GROUP BY ${artifactRuns.health}
      )
      SELECT
        coalesce((
          SELECT jsonb_agg(item ORDER BY sort_time DESC, sort_id DESC)
          FROM page_rows
        ), '[]'::jsonb) AS "items",
        ((SELECT count(*) FROM page_candidates) > ${query.limit}) AS "hasExtra",
        (
          SELECT count(*)
          FROM ${artifactRuns}
          INNER JOIN ${artifacts}
            ON ${artifacts.artifactId} = ${artifactRuns.artifactId}
          WHERE ${filtered ?? sql`true`}
        ) AS "filteredTotal",
        overview.active_count AS "activeCount",
        overview.healthy_count AS "healthyCount",
        overview.indexed_evidence_count AS "indexedEvidenceCount",
        overview.passed_count AS "passedCount",
        overview.recorded_count AS "recordedCount",
        ${facetSnapshot("producer_facets")} AS "producerFacets",
        ${facetSnapshot("status_facets")} AS "statusFacets",
        ${facetSnapshot("health_facets")} AS "healthFacets"
      FROM overview
    `);
    const snapshot = requiredRow(
      snapshotResult.rows,
      "run catalog snapshot",
    ) as RunCatalogSnapshot;
    const selectedRows = snapshot.items.map((item) => ({
      run: mapRun(item),
      sortTime: timestampValue(item.sortTime, "run sort time"),
    }));

    return {
      facets: {
        health: mapFacets(snapshot.healthFacets),
        producers: mapFacets(snapshot.producerFacets),
        statuses: mapFacets(snapshot.statusFacets),
      },
      filteredTotal: integerValue(snapshot.filteredTotal, "run total"),
      overview: {
        activeCount: integerValue(snapshot.activeCount, "active run total"),
        healthyCount: integerValue(snapshot.healthyCount, "healthy run total"),
        indexedEvidenceCount: integerValue(snapshot.indexedEvidenceCount, "indexed evidence total"),
        passedCount: integerValue(snapshot.passedCount, "passed run total"),
        recordedCount: integerValue(snapshot.recordedCount, "recorded run total"),
      },
      pageInfo: pageInfo(
        "runs",
        selectedRows,
        ({ run, sortTime: time }) => ({ id: run.artifactId, time }),
        query.direction,
        snapshot.hasExtra,
        Boolean(cursor),
      ),
      runs: selectedRows.map(({ run }) => run),
    };
  }
}

function artifactSnapshotJson(): SQL {
  return sql`jsonb_build_object(
    'artifactId', ${artifacts.artifactId},
    'committedAt', ${artifacts.committedAt},
    'contentType', ${artifacts.contentType},
    'expiresAt', ${artifacts.expiresAt},
    'kind', ${artifacts.kind},
    'producer', ${artifacts.producer},
    'sha256', ${artifacts.sha256},
    'sizeBytes', ${artifacts.sizeBytes},
    'sortTime', ${artifacts.createdAt}
  )`;
}

function runSnapshotJson(sortTime: SQL<Date>): SQL {
  return sql`jsonb_build_object(
    'artifact', jsonb_build_object(
      'artifactId', ${artifacts.artifactId},
      'contentType', ${artifacts.contentType},
      'kind', ${artifacts.kind},
      'producer', ${artifacts.producer},
      'sizeBytes', ${artifacts.sizeBytes}
    ),
    'run', jsonb_build_object(
      'artifactCount', ${artifactRuns.artifactCount},
      'backend', ${artifactRuns.backend},
      'createdAt', ${artifactRuns.createdAt},
      'detectorName', ${artifactRuns.detectorName},
      'detectorVersion', ${artifactRuns.detectorVersion},
      'durationMs', ${artifactRuns.durationMs},
      'endedAt', ${artifactRuns.endedAt},
      'environment', ${artifactRuns.environment},
      'errorKind', ${artifactRuns.errorKind},
      'evidence', ${artifactRuns.evidence},
      'exitCode', ${artifactRuns.exitCode},
      'health', ${artifactRuns.health},
      'healthChanged', ${artifactRuns.healthChanged},
      'healthDeclared', ${artifactRuns.healthDeclared},
      'healthEmpty', ${artifactRuns.healthEmpty},
      'healthMissing', ${artifactRuns.healthMissing},
      'healthPresent', ${artifactRuns.healthPresent},
      'healthReasons', ${artifactRuns.healthReasons},
      'nativeRunId', ${artifactRuns.nativeRunId},
      'outcomeCount', ${artifactRuns.outcomeCount},
      'outcomes', ${artifactRuns.outcomes},
      'runIndexSha256', ${artifactRuns.runIndexSha256},
      'seriesKey', ${artifactRuns.seriesKey},
      'sourceSha256', ${artifactRuns.sourceSha256},
      'specName', ${artifactRuns.specName},
      'startedAt', ${artifactRuns.startedAt},
      'status', ${artifactRuns.status},
      'stepCount', ${artifactRuns.stepCount},
      'updatedAt', ${artifactRuns.updatedAt}
    ),
    'sortTime', ${sortTime}
  )`;
}

function pageCandidateOrder(direction: "next" | "previous"): SQL {
  return direction === "previous"
    ? sql`sort_time ASC, sort_id ASC`
    : sql`sort_time DESC, sort_id DESC`;
}

function facetSnapshot(
  name: "health_facets" | "kind_facets" | "producer_facets" | "status_facets",
): SQL {
  const table = sql.identifier(name);
  return sql`coalesce((
    SELECT jsonb_agg(
      jsonb_build_object('count', count, 'value', value)
      ORDER BY count DESC, value ASC
    )
    FROM ${table}
    WHERE value IS NOT NULL
  ), '[]'::jsonb)`;
}

function retainedArtifactPredicate(ownerAccountId: string, now: Date): SQL | undefined {
  return and(
    eq(artifacts.ownerAccountId, ownerAccountId),
    eq(artifacts.state, "committed"),
    or(isNull(artifacts.expiresAt), gt(artifacts.expiresAt, now)),
  );
}

function artifactPredicate(
  query: ConsoleArtifactListQuery,
  ownerAccountId: string,
  now: Date,
): SQL | undefined {
  const search = query.q ? `%${escapeLike(query.q)}%` : undefined;
  return and(
    retainedArtifactPredicate(ownerAccountId, now),
    query.kind ? eq(artifacts.kind, query.kind) : undefined,
    query.producer
      ? sql`${artifacts.producer}->>'tool' = ${query.producer}`
      : undefined,
    search
      ? or(
          sql`${artifacts.artifactId} ilike ${search} escape '\\'`,
          sql`${artifacts.kind} ilike ${search} escape '\\'`,
          sql`${artifacts.producer}->>'tool' ilike ${search} escape '\\'`,
          sql`${artifacts.producer}->>'native_id' ilike ${search} escape '\\'`,
          sql`${artifacts.producer}->>'entrypoint' ilike ${search} escape '\\'`,
        )
      : undefined,
  );
}

function retainedRunPredicate(ownerAccountId: string, now: Date): SQL | undefined {
  return and(
    eq(artifactRuns.ownerAccountId, ownerAccountId),
    eq(artifacts.ownerAccountId, ownerAccountId),
    eq(artifacts.state, "committed"),
    or(isNull(artifacts.expiresAt), gt(artifacts.expiresAt, now)),
  );
}

function runPredicate(
  query: ConsoleRunListQuery,
  ownerAccountId: string,
  now: Date,
  sortTime: SQL<Date>,
): SQL | undefined {
  const search = query.q ? `%${escapeLike(query.q)}%` : undefined;
  return and(
    retainedRunPredicate(ownerAccountId, now),
    query.status ? eq(artifactRuns.status, query.status) : undefined,
    query.health ? eq(artifactRuns.health, query.health) : undefined,
    query.producer ? eq(artifactRuns.producerTool, query.producer) : undefined,
    query.from ? gte(sortTime, new Date(query.from)) : undefined,
    query.to ? lte(sortTime, new Date(query.to)) : undefined,
    search
      ? or(
          sql`${artifactRuns.artifactId} ilike ${search} escape '\\'`,
          sql`${artifactRuns.producerTool} ilike ${search} escape '\\'`,
          sql`${artifactRuns.nativeRunId} ilike ${search} escape '\\'`,
          sql`${artifactRuns.seriesKey} ilike ${search} escape '\\'`,
          sql`${artifactRuns.specName} ilike ${search} escape '\\'`,
          sql`${artifactRuns.environment} ilike ${search} escape '\\'`,
          sql`${artifactRuns.backend} ilike ${search} escape '\\'`,
        )
      : undefined,
  );
}

function mapArtifact(artifact: ArtifactSnapshotRow): ArtifactSummary {
  const producer = artifact.producer as ArtifactPlanInput["producer"];
  return {
    artifact: {
      artifactId: artifact.artifactId,
      committedAt: timestampIso(artifact.committedAt, "artifact commit time"),
      contentType: artifact.contentType,
      expiresAt: timestampIso(artifact.expiresAt, "artifact expiry time"),
      kind: artifact.kind,
      producer,
      sha256: artifact.sha256,
      sizeBytes: integerValue(artifact.sizeBytes, "artifact size"),
      state: "committed",
      verification: "server-sha256",
    },
    artifactRef: {
      $schema: "urn:filecheap.dev:artifact-ref:v1",
      artifact_id: artifact.artifactId,
      kind: artifact.kind,
      producer,
      provider: "fcheap-cloud",
      uri: `fcheap://cloud/vaults/private/artifacts/${artifact.artifactId}`,
      version: 1,
    },
  };
}

function mapRun({ artifact, run }: RunSnapshotRow): RunSummary {
  const producer = artifact.producer as ArtifactPlanInput["producer"];
  if (!producer.native_id || !producer.native_schema) {
    throw new Error("Indexed run producer identity is incomplete");
  }
  return {
    artifactId: artifact.artifactId,
    counts: {
      artifacts: integerValue(run.artifactCount, "run artifact count"),
      outcomes: integerValue(run.outcomeCount, "run outcome count"),
      steps: integerValue(run.stepCount, "run step count"),
    },
    createdAt: timestampValue(run.createdAt, "run creation time").toISOString(),
    detector: {
      name: run.detectorName as RunSummary["detector"]["name"],
      version: run.detectorVersion,
    },
    evidence: run.evidence as RunSummary["evidence"],
    health: {
      changed: integerValue(run.healthChanged, "changed run evidence count"),
      declared: integerValue(run.healthDeclared, "declared run evidence count"),
      empty: integerValue(run.healthEmpty, "empty run evidence count"),
      missing: integerValue(run.healthMissing, "missing run evidence count"),
      present: integerValue(run.healthPresent, "present run evidence count"),
      reasons: run.healthReasons as RunSummary["health"]["reasons"],
      state: run.health as RunSummary["health"]["state"],
    },
    outcomes: run.outcomes as RunSummary["outcomes"],
    producer: {
      ...producer,
      native_id: producer.native_id,
      native_schema: producer.native_schema,
    },
    run: {
      ...(run.backend ? { backend: run.backend } : {}),
      ...(run.durationMs !== null
        ? { durationMs: integerValue(run.durationMs, "run duration") }
        : {}),
      ...(run.endedAt
        ? { endedAt: timestampValue(run.endedAt, "run end time").toISOString() }
        : {}),
      ...(run.environment ? { environment: run.environment } : {}),
      ...(run.errorKind ? { errorKind: run.errorKind } : {}),
      ...(run.exitCode !== null
        ? { exitCode: signedIntegerValue(run.exitCode, "run exit code") }
        : {}),
      nativeId: run.nativeRunId,
      seriesKey: run.seriesKey,
      ...(run.specName ? { specName: run.specName } : {}),
      ...(run.startedAt
        ? { startedAt: timestampValue(run.startedAt, "run start time").toISOString() }
        : {}),
      status: run.status as RunSummary["run"]["status"],
    },
    runIndexSha256: run.runIndexSha256,
    source: {
      contentType: artifact.contentType,
      kind: artifact.kind,
      sha256: run.sourceSha256,
      sizeBytes: integerValue(artifact.sizeBytes, "run source size"),
    },
    updatedAt: timestampValue(run.updatedAt, "run update time").toISOString(),
  };
}

function pageInfo<Row>(
  scope: ConsoleCatalogScope,
  rows: readonly Row[],
  cursorFor: (row: Row) => { id: string; time: Date },
  direction: "next" | "previous",
  hasExtra: boolean,
  hadCursor: boolean,
): ConsoleCatalogPageInfo {
  const first = rows.at(0);
  const last = rows.at(-1);
  return {
    endCursor: last
      ? encodeConsoleCatalogCursor(scope, cursorFor(last))
      : null,
    hasNextPage: direction === "previous" ? hadCursor : hasExtra,
    hasPreviousPage: direction === "previous" ? hasExtra : hadCursor,
    startCursor: first
      ? encodeConsoleCatalogCursor(scope, cursorFor(first))
      : null,
  };
}

function mapFacets(
  rows: readonly { count: unknown; value: string | null }[],
): ConsoleCatalogFacetValue[] {
  return rows.flatMap((row) => row.value
    ? [{ count: integerValue(row.count, `facet '${row.value}'`), value: row.value }]
    : []);
}

function integerValue(value: unknown, label: string): number {
  const result = typeof value === "number" ? value : Number(value);
  if (!Number.isSafeInteger(result) || result < 0) {
    throw new Error(`${label} is outside the safe integer range`);
  }
  return result;
}

function signedIntegerValue(value: unknown, label: string): number {
  const result = typeof value === "number" ? value : Number(value);
  if (!Number.isSafeInteger(result)) {
    throw new Error(`${label} is outside the safe integer range`);
  }
  return result;
}

function timestampIso(value: TimestampValue | null, label: string): string | null {
  return value === null ? null : timestampValue(value, label).toISOString();
}

function timestampValue(value: TimestampValue, label: string): Date {
  const result = value instanceof Date ? value : new Date(value);
  if (Number.isNaN(result.getTime())) {
    throw new Error(`${label} is invalid`);
  }
  return result;
}

function requiredRow<Row>(rows: readonly Row[], label: string): Row {
  const row = rows[0];
  if (!row) throw new Error(`${label} query returned no row`);
  return row;
}

function escapeLike(value: string): string {
  return value
    .replaceAll("\\", "\\\\")
    .replaceAll("%", "\\%")
    .replaceAll("_", "\\_");
}
