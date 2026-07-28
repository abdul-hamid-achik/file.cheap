"use client";

import type { Route } from "next";
import Link from "next/link";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useEffect, useRef, useState, useTransition } from "react";

import type {
  ConsoleCatalogPageInfo,
  ConsoleRunListQuery,
  ConsoleRunListResponse,
} from "@/features/console/catalog/contracts";
import { CollectionPager } from "@/features/console/ui/CollectionPager";
import {
  catalogDrawerCloseMode,
  catalogDrawerHref,
  resolveCatalogDrawer,
} from "@/features/console/ui/catalog-drawer-url";
import type { RunSummary } from "@/features/runs/contracts";

import { RunCards } from "./RunCards";
import { RunDetail } from "./RunDetail";
import { RunTable } from "./RunTable";
import { type RunFilters } from "./run-presentation";
import styles from "./runs.module.css";

interface RunBrowserProps {
  facets: ConsoleRunListResponse["facets"];
  filteredTotal: number;
  initialQuery: ConsoleRunListQuery;
  page: number;
  pageInfo: ConsoleCatalogPageInfo;
  recordedTotal: number;
  runs: readonly RunSummary[];
}

/** URL-persisted client controls over a server-filtered, owner-scoped catalog. */
export function RunBrowser({
  facets,
  filteredTotal,
  initialQuery,
  page,
  pageInfo,
  recordedTotal,
  runs,
}: RunBrowserProps) {
  const pathname = usePathname();
  const router = useRouter();
  const searchParams = useSearchParams();
  const [isPending, startTransition] = useTransition();
  const [filters, setFilters] = useState<RunFilters>(() => filtersFromQuery(initialQuery));
  const [pageSize, setPageSize] = useState(initialQuery.limit);
  const createdDrawerHrefRef = useRef<string | null>(null);
  const invalidDrawerCleanupRef = useRef<string | null>(null);
  const previouslyOpenDrawerRef = useRef(false);
  const resultsRef = useRef<HTMLDivElement>(null);
  const returnFocusRef = useRef<HTMLElement>(null);
  const serverSignature = runFilterSignature(filtersFromQuery(initialQuery), initialQuery.limit);
  const localSignature = runFilterSignature(filters, pageSize);
  const filtersSettled = serverSignature === localSignature;
  const hasDateWindow = Boolean(initialQuery.from || initialQuery.to);
  const activeFilters = runActiveFilters(filters);
  const currentSearchParams = new URLSearchParams(searchParams.toString());
  const drawer = resolveCatalogDrawer(
    currentSearchParams,
    "run",
    runs,
    (run) => run.artifactId,
  );
  const selectedRun = drawer.item;
  const currentHref = catalogHref(pathname, currentSearchParams);

  useEffect(() => {
    if (!initialQuery.cursor) return;
    const frame = window.requestAnimationFrame(() => {
      const results = resultsRef.current;
      if (!results) return;
      results.focus({ preventScroll: true });
      results.scrollIntoView({
        behavior: window.matchMedia("(prefers-reduced-motion: reduce)").matches
          ? "auto"
          : "smooth",
        block: "start",
      });
    });
    return () => window.cancelAnimationFrame(frame);
  }, [initialQuery.cursor]);

  useEffect(() => {
    if (!drawer.shouldClean || !filtersSettled) {
      invalidDrawerCleanupRef.current = null;
      return;
    }
    if (invalidDrawerCleanupRef.current === currentHref) return;
    invalidDrawerCleanupRef.current = currentHref;
    const cleanHref = catalogDrawerHref(
      pathname,
      new URLSearchParams(searchParams.toString()),
      "run",
    );
    startTransition(() => router.replace(cleanHref as Route, { scroll: false }));
  }, [currentHref, drawer.shouldClean, filtersSettled, pathname, router, searchParams]);

  useEffect(() => {
    const wasOpen = previouslyOpenDrawerRef.current;
    previouslyOpenDrawerRef.current = selectedRun !== null;
    if (!wasOpen || selectedRun) return;
    const frame = window.requestAnimationFrame(() => focusDrawerReturnTarget(
      returnFocusRef,
      resultsRef,
      "run-browser-title",
    ));
    return () => window.cancelAnimationFrame(frame);
  }, [selectedRun]);

  useEffect(() => {
    if (filtersSettled) return;
    const timeout = window.setTimeout(() => {
      const params = new URLSearchParams(searchParams.toString());
      setOptionalParam(params, "q", filters.query.trim());
      setOptionalParam(params, "status", filters.status === "all" ? "" : filters.status);
      setOptionalParam(params, "producer", filters.producer === "all" ? "" : filters.producer);
      setOptionalParam(params, "health", filters.health === "all" ? "" : filters.health);
      params.set("limit", String(pageSize));
      params.delete("cursor");
      params.delete("direction");
      params.delete("page");
      params.delete("run");
      startTransition(() => router.replace(catalogHref(pathname, params) as Route, { scroll: false }));
    }, 250);
    return () => window.clearTimeout(timeout);
  }, [filters, filtersSettled, pageSize, pathname, router, searchParams]);

  function updateFilter<Key extends keyof RunFilters>(key: Key, value: RunFilters[Key]) {
    setFilters((current) => ({ ...current, [key]: value }));
  }

  function selectRun(run: RunSummary, trigger: HTMLButtonElement) {
    returnFocusRef.current = trigger;
    const href = catalogDrawerHref(pathname, currentSearchParams, "run", run.artifactId);
    createdDrawerHrefRef.current = href;
    startTransition(() => router.push(href as Route, { scroll: false }));
  }

  function closeDetail() {
    focusDrawerReturnTarget(returnFocusRef, resultsRef, "run-browser-title", false);
    const cleanHref = catalogDrawerHref(pathname, currentSearchParams, "run");
    if (catalogDrawerCloseMode(currentHref, createdDrawerHrefRef.current) === "back") {
      router.back();
      return;
    }
    startTransition(() => router.replace(cleanHref as Route, { scroll: false }));
  }

  function clearFilters() {
    setFilters({ health: "all", producer: "all", query: "", status: "all" });
    const params = new URLSearchParams(searchParams.toString());
    for (const key of ["q", "status", "producer", "health", "from", "to", "cursor", "direction", "page", "run"]) {
      params.delete(key);
    }
    params.set("limit", String(pageSize));
    startTransition(() => router.replace(catalogHref(pathname, params) as Route, { scroll: false }));
  }

  function clearDateWindow() {
    const params = new URLSearchParams(searchParams.toString());
    for (const key of ["from", "to", "cursor", "direction", "page", "run"]) params.delete(key);
    startTransition(() => router.replace(catalogHref(pathname, params) as Route, { scroll: false }));
  }

  function movePage(direction: "next" | "previous") {
    const cursor = direction === "next" ? pageInfo.endCursor : pageInfo.startCursor;
    if (!cursor || !filtersSettled) return;
    const params = new URLSearchParams(searchParams.toString());
    params.set("cursor", cursor);
    params.set("direction", direction);
    params.set("page", String(Math.max(1, page + (direction === "next" ? 1 : -1))));
    params.delete("run");
    startTransition(() => router.push(catalogHref(pathname, params) as Route, { scroll: false }));
  }

  function returnToFirstPage() {
    const params = new URLSearchParams(searchParams.toString());
    params.delete("cursor");
    params.delete("direction");
    params.delete("page");
    params.delete("run");
    startTransition(() => router.push(catalogHref(pathname, params) as Route, { scroll: false }));
  }

  const busy = isPending || !filtersSettled;

  return (
    <section aria-busy={busy} aria-labelledby="run-browser-title" className={styles.browser}>
      <header className={styles.browserHeader}>
        <div>
          <p className={styles.eyebrow}>Run registry</p>
          <h1 id="run-browser-title" tabIndex={-1}>Runs</h1>
          <p>Inspect the recorded status, evidence health, outcomes, and provenance for trusted producer runs.</p>
        </div>
        <p aria-live="polite" className={styles.count}>{busy ? "Updating catalog…" : `${runs.length} on this page · ${filteredTotal} matching`}</p>
      </header>
      {recordedTotal === 0 ? (
        <div className={styles.emptyState}>
          <span aria-hidden="true">◇</span>
          <h2>No runs recorded</h2>
          <p>Artifacts without a metadata-only RunIndexV1 remain available under Artifacts, but they are not added to the run registry.</p>
          <Link href="/integrations/run-index">Learn how to publish a run index</Link>
        </div>
      ) : (
        <>
          <form aria-label="Run filters" className={styles.filters} onSubmit={(event) => event.preventDefault()}>
            <label className={styles.searchField} htmlFor="run-search">
              <span>Search runs</span>
              <input
                id="run-search"
                onChange={(event) => updateFilter("query", event.target.value)}
                placeholder="Run, spec, environment, producer"
                type="search"
                value={filters.query}
              />
            </label>
            <label>
              <span>Status</span>
              <select onChange={(event) => updateFilter("status", event.target.value as RunFilters["status"])} value={filters.status}>
                <option value="all">All statuses</option>
                <option value="queued">{facetLabel("Queued", facets.statuses, "queued")}</option>
                <option value="running">{facetLabel("Running", facets.statuses, "running")}</option>
                <option value="passed">{facetLabel("Passed", facets.statuses, "passed")}</option>
                <option value="failed">{facetLabel("Failed", facets.statuses, "failed")}</option>
                <option value="errored">{facetLabel("Errored", facets.statuses, "errored")}</option>
                <option value="cancelled">{facetLabel("Cancelled", facets.statuses, "cancelled")}</option>
                <option value="incomplete">{facetLabel("Incomplete", facets.statuses, "incomplete")}</option>
                <option value="unknown">{facetLabel("Unknown", facets.statuses, "unknown")}</option>
              </select>
            </label>
            <label>
              <span>Producer</span>
              <select onChange={(event) => updateFilter("producer", event.target.value)} value={filters.producer}>
                <option value="all">All producers</option>
                {facets.producers.map((facet) => <option key={facet.value} value={facet.value}>{facet.value} ({facet.count})</option>)}
              </select>
            </label>
            <label>
              <span>Evidence health</span>
              <select onChange={(event) => updateFilter("health", event.target.value as RunFilters["health"])} value={filters.health}>
                <option value="all">All health states</option>
                <option value="ok">{facetLabel("Healthy", facets.health, "ok")}</option>
                <option value="degraded">{facetLabel("Degraded", facets.health, "degraded")}</option>
                <option value="incomplete">{facetLabel("Incomplete", facets.health, "incomplete")}</option>
                <option value="unknown">{facetLabel("Unknown", facets.health, "unknown")}</option>
              </select>
            </label>
          </form>
          {activeFilters.length > 0 ? (
            <div aria-label="Active run filters" className={styles.appliedFilterNotice}>
              <p><strong>Active filters</strong> {activeFilters.join(" · ")}</p>
              <button disabled={busy} onClick={clearFilters} type="button">Clear all filters</button>
            </div>
          ) : null}
          {hasDateWindow ? (
            <div className={styles.appliedFilterNotice} role="status">
              <p><strong>Date window</strong> {dateWindowLabel(initialQuery.from, initialQuery.to)}</p>
              <button disabled={busy} onClick={clearDateWindow} type="button">Clear date window</button>
            </div>
          ) : null}
          {filteredTotal === 0 ? (
            <div className={styles.filteredEmptyState}>
              <h2>No runs match these filters</h2>
              <p>Choose another status, producer, or evidence health state.</p>
              <button onClick={clearFilters} type="button">Clear filters</button>
            </div>
          ) : (
            <div
              aria-label="Run results"
              className={styles.resultsRegion}
              ref={resultsRef}
              tabIndex={-1}
            >
              {runs.length === 0 ? (
                <div className={styles.filteredEmptyState}>
                  <h2>This cursor page is no longer available</h2>
                  <p>The run catalog changed while you were browsing. Return to the beginning of these filtered results.</p>
                  <button disabled={busy} onClick={returnToFirstPage} type="button">Return to first page</button>
                </div>
              ) : (
                <>
                  <RunTable onSelect={selectRun} runs={runs} selectedId={selectedRun?.artifactId} />
                  <RunCards onSelect={selectRun} runs={runs} selectedId={selectedRun?.artifactId} />
                </>
              )}
              <CollectionPager
                busy={busy}
                currentPage={page}
                hasNextPage={pageInfo.hasNextPage}
                hasPreviousPage={pageInfo.hasPreviousPage}
                itemLabel="runs"
                onNextPage={() => movePage("next")}
                onPageSizeChange={setPageSize}
                onPreviousPage={() => movePage("previous")}
                pageSize={pageSize}
                totalItems={filteredTotal}
                visibleItems={runs.length}
              />
            </div>
          )}
        </>
      )}
      <RunDetail onClose={closeDetail} returnFocusRef={returnFocusRef} run={selectedRun} />
    </section>
  );
}

function filtersFromQuery(query: ConsoleRunListQuery): RunFilters {
  return {
    health: query.health ?? "all",
    producer: query.producer ?? "all",
    query: query.q ?? "",
    status: query.status ?? "all",
  };
}

function runFilterSignature(filters: RunFilters, pageSize: number): string {
  return JSON.stringify([
    filters.query.trim(),
    filters.status,
    filters.producer,
    filters.health,
    pageSize,
  ]);
}

function runActiveFilters(filters: RunFilters): string[] {
  return [
    filters.query.trim() ? `Search: ${filters.query.trim()}` : null,
    filters.status === "all" ? null : `Status: ${filters.status}`,
    filters.producer === "all" ? null : `Producer: ${filters.producer}`,
    filters.health === "all" ? null : `Evidence: ${filters.health}`,
  ].filter((label): label is string => label !== null);
}

function setOptionalParam(params: URLSearchParams, key: string, value: string) {
  if (value) params.set(key, value);
  else params.delete(key);
}

function catalogHref(pathname: string, params: URLSearchParams): string {
  const query = params.toString();
  return query ? `${pathname}?${query}` : pathname;
}

function dateWindowLabel(from?: string, to?: string): string {
  if (from && to) return `${dateBoundary(from)} to ${dateBoundary(to)}`;
  if (from) return `from ${dateBoundary(from)}`;
  return to ? `through ${dateBoundary(to)}` : "all recorded dates";
}

function dateBoundary(value: string): string {
  return new Date(value).toISOString().replace("T", " ").replace(/\.\d{3}Z$/u, " UTC");
}

function facetLabel(
  label: string,
  facets: readonly { count: number; value: string }[],
  value: string,
): string {
  const count = facets.find((facet) => facet.value === value)?.count;
  return count === undefined ? label : `${label} (${count})`;
}

function focusDrawerReturnTarget(
  returnFocusRef: { current: HTMLElement | null },
  resultsRef: { current: HTMLElement | null },
  headingId: string,
  focus = true,
): HTMLElement | null {
  const trigger = returnFocusRef.current;
  const target = trigger?.isConnected
    ? trigger
    : resultsRef.current ?? document.getElementById(headingId);
  returnFocusRef.current = target;
  if (focus) target?.focus({ preventScroll: true });
  return target;
}
