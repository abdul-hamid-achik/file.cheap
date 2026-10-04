import type { RunHealth, RunStatus, RunSummary } from "@/features/runs/contracts";
import { artifactPullCommand, shellQuote } from "@/features/console/ui/ArtifactDetail";

export const runDetailTabOrder = ["summary", "outcomes", "evidence", "provenance"] as const;

export type RunDetailTab = (typeof runDetailTabOrder)[number];

export interface RunDashboardMetrics {
  activeCount: number;
  evidenceCount: number;
  healthyCount: number;
  passedCount: number;
  totalCount: number;
}

export interface RunFilters {
  health: RunHealth | "all";
  producer: string | "all";
  query: string;
  status: RunStatus | "all";
}

export const defaultRunFilters: RunFilters = {
  health: "all",
  producer: "all",
  query: "",
  status: "all",
};

/** A producer-supplied native ID reduced to a safe, non-hidden local file stem. */
export function runRestoreName(nativeId: string): string {
  const stem = nativeId.replace(/[^A-Za-z0-9._-]+/gu, "_").replace(/^[._-]+/u, "").slice(0, 96);
  return stem === "" ? "run" : stem;
}

export function isGzipRunBundle(contentType: string): boolean {
  return /^application\/(?:x-)?gzip$/iu.test(contentType.split(";")[0]?.trim() ?? "");
}

/**
 * Exact CLI sequence that restores a published run bundle: a verified
 * `fcheap pull` followed by extraction into its own directory. The console
 * never fetches or extracts bytes itself. Only gzip tarballs (what cairntrace
 * and glyphrun publish) get the extraction step; any other content type gets
 * the pull alone.
 */
export function runRestoreCommand(run: RunSummary): string {
  const name = runRestoreName(run.run.nativeId);
  if (!isGzipRunBundle(run.source.contentType)) {
    return artifactPullCommand(run.artifactId, `./${name}.bin`);
  }
  const archive = shellQuote(`./${name}.tar.gz`);
  const directory = shellQuote(`./${name}`);
  return `${artifactPullCommand(run.artifactId, `./${name}.tar.gz`)} && mkdir -p ${directory} && tar -xzf ${archive} -C ${directory}`;
}

export function getNextRunDetailTab(current: RunDetailTab, key: string): RunDetailTab | null {
  if (key === "Home") return runDetailTabOrder[0];
  if (key === "End") return runDetailTabOrder.at(-1) ?? null;
  if (key !== "ArrowLeft" && key !== "ArrowRight") return null;

  const currentIndex = runDetailTabOrder.indexOf(current);
  const offset = key === "ArrowRight" ? 1 : -1;
  const nextIndex = (currentIndex + offset + runDetailTabOrder.length) % runDetailTabOrder.length;
  return runDetailTabOrder[nextIndex] ?? null;
}

export function deriveRunDashboardMetrics(runs: readonly RunSummary[]): RunDashboardMetrics {
  let activeCount = 0;
  let evidenceCount = 0;
  let healthyCount = 0;
  let passedCount = 0;

  for (const run of runs) {
    evidenceCount += run.evidence.length;
    if (run.run.status === "queued" || run.run.status === "running") activeCount += 1;
    if (run.run.status === "passed") passedCount += 1;
    if (run.health.state === "ok") healthyCount += 1;
  }

  return { activeCount, evidenceCount, healthyCount, passedCount, totalCount: runs.length };
}

export function filterRuns(runs: readonly RunSummary[], filters: RunFilters): RunSummary[] {
  const needle = filters.query.trim().toLocaleLowerCase();
  return runs.filter((run) => {
    const matchesQuery = needle === "" || [
      run.artifactId,
      run.producer.tool,
      run.producer.native_id,
      run.run.environment,
      run.run.nativeId,
      run.run.specName,
    ]
      .filter((value): value is string => Boolean(value))
      .some((value) => value.toLocaleLowerCase().includes(needle));
    return matchesQuery &&
      (filters.status === "all" || run.run.status === filters.status) &&
      (filters.producer === "all" || run.producer.tool === filters.producer) &&
      (filters.health === "all" || run.health.state === filters.health);
  });
}

export function runStatusLabel(status: RunStatus): string {
  const labels: Record<RunStatus, string> = {
    cancelled: "Cancelled",
    errored: "Errored",
    failed: "Failed",
    incomplete: "Incomplete",
    passed: "Passed",
    queued: "Queued",
    running: "Running",
    unknown: "Unknown",
  };
  return labels[status];
}

export function runHealthLabel(health: RunHealth): string {
  const labels: Record<RunHealth, string> = {
    degraded: "Degraded evidence",
    incomplete: "Incomplete evidence",
    ok: "Evidence healthy",
    unknown: "Evidence health unknown",
  };
  return labels[health];
}

export function formatRunDate(value: string | undefined): string {
  if (!value) return "Not recorded";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "Date unavailable";
  return new Intl.DateTimeFormat("en-US", {
    dateStyle: "medium",
    timeStyle: "short",
    timeZone: "UTC",
  }).format(date);
}

export function formatRunDuration(durationMs: number | undefined): string {
  if (durationMs === undefined) return "Not recorded";
  if (durationMs < 1_000) return `${durationMs} ms`;
  const seconds = durationMs / 1_000;
  if (seconds < 60) return `${seconds.toFixed(seconds >= 10 ? 0 : 1)} s`;
  const minutes = Math.floor(seconds / 60);
  const remainingSeconds = Math.round(seconds % 60);
  return `${minutes}m ${remainingSeconds}s`;
}

export function runEvidenceCountLabel(run: RunSummary): string {
  return `${run.evidence.length} indexed of ${run.counts.artifacts} declared`;
}
