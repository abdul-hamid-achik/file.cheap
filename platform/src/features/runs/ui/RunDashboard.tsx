import type {
  ConsoleRunListQuery,
  ConsoleRunListResponse,
} from "@/features/console/catalog/contracts";

import { RunBrowser } from "./RunBrowser";
import styles from "./runs.module.css";

interface RunDashboardProps {
  catalog: ConsoleRunListResponse;
  page: number;
  query: ConsoleRunListQuery;
}

/** Server component backed by exact owner-scoped catalog aggregates. */
export function RunDashboard({ catalog, page, query }: RunDashboardProps) {
  const metrics = catalog.overview;

  return (
    <div className={styles.dashboard}>
      <dl aria-label="Run summary" className={styles.metrics}>
        <Metric label="Recorded runs" value={String(metrics.recordedCount)} />
        <Metric label="Active runs" tone={metrics.activeCount > 0 ? "attention" : "quiet"} value={String(metrics.activeCount)} />
        <Metric label="Passed" value={String(metrics.passedCount)} />
        <Metric label="Healthy evidence" value={String(metrics.healthyCount)} />
        <Metric label="Indexed evidence" value={String(metrics.indexedEvidenceCount)} />
      </dl>
      <RunBrowser
        facets={catalog.facets}
        filteredTotal={catalog.filteredTotal}
        initialQuery={query}
        key={JSON.stringify([query.cursor, query.direction, query.from, query.health, query.limit, query.producer, query.q, query.status, query.to])}
        page={page}
        pageInfo={catalog.pageInfo}
        recordedTotal={catalog.overview.recordedCount}
        runs={catalog.runs}
      />
    </div>
  );
}

function Metric({ label, tone = "default", value }: { label: string; tone?: "attention" | "default" | "quiet"; value: string }) {
  const className = tone === "attention"
    ? `${styles.metric} ${styles.metricAttention}`
    : tone === "quiet"
      ? `${styles.metric} ${styles.metricQuiet}`
      : styles.metric;
  return <div className={className}><dt>{label}</dt><dd>{value}</dd></div>;
}

export function RunDashboardLoading() {
  return (
    <div aria-busy="true" aria-label="Loading run dashboard" className={styles.dashboard}>
      <div className={styles.metrics}>
        {Array.from({ length: 5 }, (_, index) => <div className={styles.metricSkeleton} key={index} />)}
      </div>
      <div className={styles.loadingRows}>
        {Array.from({ length: 6 }, (_, index) => <div className={styles.loadingRow} key={index} />)}
      </div>
    </div>
  );
}
