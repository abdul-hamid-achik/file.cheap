"use client";

import { useMemo, useRef, useState } from "react";

import type { RunSummary } from "@/features/runs/contracts";

import { RunCards } from "./RunCards";
import { RunDetail } from "./RunDetail";
import { RunTable } from "./RunTable";
import { defaultRunFilters, filterRuns, type RunFilters } from "./run-presentation";
import styles from "./runs.module.css";

interface RunBrowserProps {
  runs: readonly RunSummary[];
}

/** Interactive client leaf. Data ownership, fetches, and mutation stay outside this component. */
export function RunBrowser({ runs }: RunBrowserProps) {
  const [filters, setFilters] = useState<RunFilters>(defaultRunFilters);
  const [selected, setSelected] = useState<RunSummary | null>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const filteredRuns = useMemo(() => filterRuns(runs, filters), [filters, runs]);
  const selectedRun = selected && filteredRuns.some((run) => run.artifactId === selected.artifactId) ? selected : null;
  const producers = useMemo<string[]>(
    () => [...new Set(runs.map((run) => run.producer.tool))].sort(),
    [runs],
  );

  function updateFilter<Key extends keyof RunFilters>(key: Key, value: RunFilters[Key]) {
    setFilters((current) => ({ ...current, [key]: value }));
  }

  function selectRun(run: RunSummary, trigger: HTMLButtonElement) {
    triggerRef.current = trigger;
    setSelected(run);
  }

  return (
    <section aria-labelledby="run-browser-title" className={styles.browser}>
      <header className={styles.browserHeader}>
        <div>
          <p className={styles.eyebrow}>Run registry</p>
          <h1 id="run-browser-title">Runs</h1>
          <p>Inspect the recorded status, evidence health, outcomes, and provenance for trusted producer runs.</p>
        </div>
        <p aria-live="polite" className={styles.count}>{filteredRuns.length} of {runs.length} recorded</p>
      </header>
      {runs.length === 0 ? (
        <div className={styles.emptyState}>
          <span aria-hidden="true">◇</span>
          <h2>No runs recorded</h2>
          <p>Trusted producer runs will appear here when they are recorded for this owner.</p>
        </div>
      ) : (
        <>
          <form className={styles.filters} onSubmit={(event) => event.preventDefault()}>
            <label>
              <span>Status</span>
              <select onChange={(event) => updateFilter("status", event.target.value as RunFilters["status"])} value={filters.status}>
                <option value="all">All statuses</option>
                <option value="queued">Queued</option>
                <option value="running">Running</option>
                <option value="passed">Passed</option>
                <option value="failed">Failed</option>
                <option value="errored">Errored</option>
                <option value="cancelled">Cancelled</option>
                <option value="incomplete">Incomplete</option>
                <option value="unknown">Unknown</option>
              </select>
            </label>
            <label>
              <span>Producer</span>
              <select onChange={(event) => updateFilter("producer", event.target.value)} value={filters.producer}>
                <option value="all">All producers</option>
                {producers.map((producer) => <option key={producer} value={producer}>{producer}</option>)}
              </select>
            </label>
            <label>
              <span>Evidence health</span>
              <select onChange={(event) => updateFilter("health", event.target.value as RunFilters["health"])} value={filters.health}>
                <option value="all">All health states</option>
                <option value="ok">Healthy</option>
                <option value="degraded">Degraded</option>
                <option value="incomplete">Incomplete</option>
                <option value="unknown">Unknown</option>
              </select>
            </label>
          </form>
          {filteredRuns.length === 0 ? (
            <div className={styles.filteredEmptyState}>
              <h2>No runs match these filters</h2>
              <p>Choose another status, producer, or evidence health state.</p>
              <button onClick={() => setFilters(defaultRunFilters)} type="button">Clear filters</button>
            </div>
          ) : (
            <>
              <RunTable onSelect={selectRun} runs={filteredRuns} selectedId={selectedRun?.artifactId} />
              <RunCards onSelect={selectRun} runs={filteredRuns} selectedId={selectedRun?.artifactId} />
            </>
          )}
        </>
      )}
      <RunDetail onClose={() => setSelected(null)} returnFocusRef={triggerRef} run={selectedRun} />
    </section>
  );
}
