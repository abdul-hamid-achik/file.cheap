import type { RunSummary } from "@/features/runs/contracts";

import { RunBrowser } from "./RunBrowser";
import { deriveRunDashboardMetrics } from "./run-presentation";
import styles from "./runs.module.css";

interface RunDashboardProps {
  runs: readonly RunSummary[];
}

/** Server component that derives dashboard totals from the owner-scoped run list. */
export function RunDashboard({ runs }: RunDashboardProps) {
  const metrics = deriveRunDashboardMetrics(runs);

  return (
    <div className={styles.dashboard}>
      <dl aria-label="Run summary" className={styles.metrics}>
        <Metric label="Recorded runs" value={String(metrics.totalCount)} />
        <Metric label="Active runs" tone={metrics.activeCount > 0 ? "attention" : "quiet"} value={String(metrics.activeCount)} />
        <Metric label="Passed" value={String(metrics.passedCount)} />
        <Metric label="Healthy evidence" value={String(metrics.healthyCount)} />
        <Metric label="Indexed evidence" value={String(metrics.evidenceCount)} />
      </dl>
      <RunBrowser runs={runs} />
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
