import type {
  ConsoleArtifactListQuery,
  ConsoleArtifactListResponse,
} from "@/features/console/catalog/contracts";

import { ArtifactBrowser } from "./ArtifactBrowser";
import { toConsoleArtifact } from "./artifact-dashboard-data";
import { type ArtifactGroupBy, formatArtifactBytes } from "./artifact-types";
import styles from "./console.module.css";

interface ArtifactDashboardProps {
  catalog: ConsoleArtifactListResponse;
  groupBy: ArtifactGroupBy;
  page: number;
  query: ConsoleArtifactListQuery;
}

/**
 * Server component: derive stable, owner-scoped presentation data before the
 * interactive catalog hydrates. Fetching and authorization remain with the
 * route that owns this component.
 */
export function ArtifactDashboard({
  catalog,
  groupBy,
  page,
  query,
}: ArtifactDashboardProps) {
  const rows = catalog.artifacts.map(toConsoleArtifact);
  const metrics = catalog.overview;

  return (
    <div className={styles.dashboard}>
      <dl aria-label="Artifact summary" className={styles.metrics}>
        <Metric label="Recorded artifacts" value={String(metrics.recordedCount)} />
        <Metric label="Verified SHA-256" value={String(metrics.verifiedCount)} />
        <Metric label="Transfer eligible" value={String(metrics.transferableCount)} />
        <Metric
          label="Expiring within 24 hours"
          tone={metrics.expiringSoonCount > 0 ? "attention" : "quiet"}
          value={String(metrics.expiringSoonCount)}
        />
        <Metric label="Recorded bytes" value={formatArtifactBytes(metrics.totalBytes)} />
      </dl>
      <ArtifactBrowser
        artifacts={rows}
        facets={catalog.facets}
        filteredTotal={catalog.filteredTotal}
        initialGroupBy={groupBy}
        initialQuery={query}
        key={JSON.stringify([query.cursor, query.direction, query.kind, query.limit, query.producer, query.q, groupBy])}
        page={page}
        pageInfo={catalog.pageInfo}
        recordedTotal={catalog.overview.recordedCount}
      />
    </div>
  );
}

function Metric({
  label,
  tone = "default",
  value,
}: {
  label: string;
  tone?: "attention" | "default" | "quiet";
  value: string;
}) {
  const className =
    tone === "attention"
      ? `${styles.metric} ${styles.metricAttention}`
      : tone === "quiet"
        ? `${styles.metric} ${styles.metricQuiet}`
        : styles.metric;
  return (
    <div className={className}>
      <dt>{label}</dt>
      <dd>{value}</dd>
    </div>
  );
}

export function ArtifactDashboardLoading() {
  return (
    <div aria-busy="true" aria-label="Loading artifact dashboard" className={styles.dashboard}>
      <div className={styles.metrics}>
        {Array.from({ length: 5 }, (_, index) => <div className={styles.metricSkeleton} key={index} />)}
      </div>
      <div className={styles.loadingRows}>
        {Array.from({ length: 6 }, (_, index) => <div className={styles.loadingRow} key={index} />)}
      </div>
    </div>
  );
}
