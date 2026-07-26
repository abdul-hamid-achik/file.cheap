import type { ArtifactSummary } from "@/features/artifacts/contracts";

import { ArtifactBrowser } from "./ArtifactBrowser";
import {
  deriveArtifactDashboardMetrics,
  toConsoleArtifact,
} from "./artifact-dashboard-data";
import { formatArtifactBytes } from "./artifact-types";
import styles from "./console.module.css";

interface ArtifactDashboardProps {
  artifacts: readonly ArtifactSummary[];
  now?: Date;
}

/**
 * Server component: derive stable, owner-scoped presentation data before the
 * interactive catalog hydrates. Fetching and authorization remain with the
 * route that owns this component.
 */
export function ArtifactDashboard({
  artifacts,
  now = new Date(),
}: ArtifactDashboardProps) {
  const rows = artifacts.map(toConsoleArtifact);
  const metrics = deriveArtifactDashboardMetrics(rows, now);

  return (
    <div className={styles.dashboard}>
      <dl aria-label="Artifact summary" className={styles.metrics}>
        <Metric label="Recorded artifacts" value={String(metrics.totalCount)} />
        <Metric label="Verified SHA-256" value={String(metrics.verifiedCount)} />
        <Metric label="Cloud transfers ready" value={String(metrics.cloudReady)} />
        <Metric
          label="Expiring within 24 hours"
          tone={metrics.expiringSoon > 0 ? "attention" : "quiet"}
          value={String(metrics.expiringSoon)}
        />
        <Metric label="Recorded bytes" value={formatArtifactBytes(metrics.totalBytes)} />
      </dl>
      <ArtifactBrowser artifacts={rows} />
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
