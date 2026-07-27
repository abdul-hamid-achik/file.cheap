import { describe, expect, test } from "bun:test";

import type { RunSummary } from "@/features/runs/contracts";

import {
  defaultRunFilters,
  deriveRunDashboardMetrics,
  filterRuns,
  formatRunDuration,
  getNextRunDetailTab,
  runEvidenceCountLabel,
} from "./run-presentation";

const baseRun: RunSummary = {
  artifactId: "art_0123456789abcdef",
  counts: { artifacts: 2, outcomes: 1, steps: 4 },
  createdAt: "2026-07-01T12:00:00.000Z",
  detector: { name: "glyphrun-run", version: "1.0.0" },
  evidence: [{
    inspectability: "metadata-only",
    integrity: "verified",
    medium: "structured-text",
    path: "report.json",
    presence: "present",
    role: "report",
    sensitivity: "metadata-safe",
  }],
  health: { changed: 0, declared: 2, empty: 0, missing: 0, present: 1, reasons: [], state: "degraded" },
  outcomes: [{ id: "assertion:one", status: "passed" }],
  producer: {
    native_id: "job:one",
    native_schema: "urn:example:run",
    tool: "glyphrun",
    version: "1.0.0",
  },
  run: {
    durationMs: 12_300,
    nativeId: "job:one",
    seriesKey: "series_0123456789abcdef",
    specName: "sign in",
    status: "passed",
  },
  runIndexSha256: "b".repeat(64),
  source: {
    contentType: "application/json",
    kind: "run-index",
    sha256: "a".repeat(64),
    sizeBytes: 128,
  },
  updatedAt: "2026-07-01T12:00:12.300Z",
};

describe("run dashboard presentation", () => {
  test("derives dashboard totals from metadata-only run records", () => {
    const running: RunSummary = {
      ...baseRun,
      artifactId: "art_abcdef0123456789",
      evidence: [],
      health: { ...baseRun.health, state: "ok" },
      run: { ...baseRun.run, nativeId: "job:two", status: "running" },
    };

    expect(deriveRunDashboardMetrics([baseRun, running])).toEqual({
      activeCount: 1,
      evidenceCount: 1,
      healthyCount: 1,
      passedCount: 1,
      totalCount: 2,
    });
  });

  test("filters by status, producer, and health without changing records", () => {
    const healthy: RunSummary = {
      ...baseRun,
      artifactId: "art_abcdef0123456789",
      health: { ...baseRun.health, state: "ok" },
    };

    expect(filterRuns([baseRun, healthy], defaultRunFilters)).toHaveLength(2);
    expect(filterRuns([baseRun, healthy], { health: "ok", producer: "glyphrun", query: "sign in", status: "passed" })).toEqual([healthy]);
    expect(filterRuns([baseRun], { ...defaultRunFilters, query: "job:one" })).toEqual([baseRun]);
    expect(baseRun.health.state).toBe("degraded");
  });

  test("formats index counts and durations without exposing byte values", () => {
    expect(runEvidenceCountLabel(baseRun)).toBe("1 indexed of 2 declared");
    expect(formatRunDuration(undefined)).toBe("Not recorded");
    expect(formatRunDuration(12_300)).toBe("12 s");
  });

  test("moves run detail tabs with the standard horizontal keyboard pattern", () => {
    expect(getNextRunDetailTab("summary", "ArrowLeft")).toBe("provenance");
    expect(getNextRunDetailTab("summary", "ArrowRight")).toBe("outcomes");
    expect(getNextRunDetailTab("evidence", "Home")).toBe("summary");
    expect(getNextRunDetailTab("outcomes", "End")).toBe("provenance");
    expect(getNextRunDetailTab("summary", "Enter")).toBeNull();
  });
});
