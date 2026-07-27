"use client";

import type { Route } from "next";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import {
  useEffect,
  useMemo,
  useRef,
  useState,
  useTransition,
} from "react";

import type {
  ConsoleArtifactListQuery,
  ConsoleArtifactListResponse,
  ConsoleCatalogPageInfo,
} from "@/features/console/catalog/contracts";

import { ArtifactCards } from "./ArtifactCards";
import { ArtifactDetail } from "./ArtifactDetail";
import { ArtifactTable } from "./ArtifactTable";
import { CollectionPager } from "./CollectionPager";
import {
  catalogDrawerCloseMode,
  catalogDrawerHref,
  resolveCatalogDrawer,
} from "./catalog-drawer-url";
import {
  groupConsoleArtifacts,
  type ArtifactGroupBy,
  type ConsoleArtifact,
} from "./artifact-types";
import styles from "./console.module.css";

interface ArtifactBrowserProps {
  artifacts: readonly ConsoleArtifact[];
  description?: string;
  facets: ConsoleArtifactListResponse["facets"];
  filteredTotal: number;
  initialGroupBy: ArtifactGroupBy;
  initialQuery: ConsoleArtifactListQuery;
  page: number;
  pageInfo: ConsoleCatalogPageInfo;
  recordedTotal: number;
  title?: string;
}

/**
 * Interactive cursor catalog. Search and facets are persisted in the URL, but
 * filtering, totals, ownership, retention, and pagination stay server-side.
 */
export function ArtifactBrowser({
  artifacts,
  description = "Inspect provenance, availability, and verification before restoring or transferring bytes.",
  facets,
  filteredTotal,
  initialGroupBy,
  initialQuery,
  page,
  pageInfo,
  recordedTotal,
  title = "Artifacts",
}: ArtifactBrowserProps) {
  const pathname = usePathname();
  const router = useRouter();
  const searchParams = useSearchParams();
  const [isPending, startTransition] = useTransition();
  const [groupBy, setGroupBy] = useState<ArtifactGroupBy>(initialGroupBy);
  const [kind, setKind] = useState(initialQuery.kind ?? "all");
  const [pageSize, setPageSize] = useState(initialQuery.limit);
  const [producer, setProducer] = useState(initialQuery.producer ?? "all");
  const [query, setQuery] = useState(initialQuery.q ?? "");
  const createdDrawerHrefRef = useRef<string | null>(null);
  const invalidDrawerCleanupRef = useRef<string | null>(null);
  const previouslyOpenDrawerRef = useRef(false);
  const resultsRef = useRef<HTMLDivElement>(null);
  const returnFocusRef = useRef<HTMLElement>(null);

  const serverSignature = artifactFilterSignature({
    groupBy: initialGroupBy,
    kind: initialQuery.kind ?? "all",
    pageSize: initialQuery.limit,
    producer: initialQuery.producer ?? "all",
    query: initialQuery.q ?? "",
  });
  const localSignature = artifactFilterSignature({
    groupBy,
    kind,
    pageSize,
    producer,
    query,
  });
  const filtersSettled = localSignature === serverSignature;
  const groups = useMemo(
    () => groupConsoleArtifacts(artifacts, groupBy),
    [artifacts, groupBy],
  );
  const activeFilters = artifactActiveFilters({ kind, producer, query });
  const currentSearchParams = new URLSearchParams(searchParams.toString());
  const drawer = resolveCatalogDrawer(
    currentSearchParams,
    "artifact",
    artifacts,
    (artifact) => artifact.id,
  );
  const selectedArtifact = drawer.item;
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
      "artifact",
    );
    startTransition(() => router.replace(cleanHref as Route, { scroll: false }));
  }, [currentHref, drawer.shouldClean, filtersSettled, pathname, router, searchParams]);

  useEffect(() => {
    const wasOpen = previouslyOpenDrawerRef.current;
    previouslyOpenDrawerRef.current = selectedArtifact !== null;
    if (!wasOpen || selectedArtifact) return;
    const frame = window.requestAnimationFrame(() => focusDrawerReturnTarget(
      returnFocusRef,
      resultsRef,
      "artifact-browser-title",
    ));
    return () => window.cancelAnimationFrame(frame);
  }, [selectedArtifact]);

  useEffect(() => {
    if (filtersSettled) return;
    const timeout = window.setTimeout(() => {
      const params = new URLSearchParams(searchParams.toString());
      setOptionalParam(params, "q", query.trim());
      setOptionalParam(params, "producer", producer === "all" ? "" : producer);
      setOptionalParam(params, "kind", kind === "all" ? "" : kind);
      setOptionalParam(params, "groupBy", groupBy === "producer" ? "" : groupBy);
      params.set("limit", String(pageSize));
      params.delete("cursor");
      params.delete("direction");
      params.delete("page");
      params.delete("artifact");
      startTransition(() => router.replace(catalogHref(pathname, params) as Route, { scroll: false }));
    }, 250);
    return () => window.clearTimeout(timeout);
  }, [filtersSettled, groupBy, kind, pageSize, pathname, producer, query, router, searchParams]);

  function selectArtifact(artifact: ConsoleArtifact, trigger: HTMLButtonElement) {
    returnFocusRef.current = trigger;
    const href = catalogDrawerHref(pathname, currentSearchParams, "artifact", artifact.id);
    createdDrawerHrefRef.current = href;
    startTransition(() => router.push(href as Route, { scroll: false }));
  }

  function closeDetail() {
    focusDrawerReturnTarget(returnFocusRef, resultsRef, "artifact-browser-title", false);
    const cleanHref = catalogDrawerHref(pathname, currentSearchParams, "artifact");
    if (catalogDrawerCloseMode(currentHref, createdDrawerHrefRef.current) === "back") {
      router.back();
      return;
    }
    startTransition(() => router.replace(cleanHref as Route, { scroll: false }));
  }

  function clearFilters() {
    setKind("all");
    setProducer("all");
    setQuery("");
  }

  function movePage(direction: "next" | "previous") {
    const cursor = direction === "next" ? pageInfo.endCursor : pageInfo.startCursor;
    if (!cursor || !filtersSettled) return;
    const params = new URLSearchParams(searchParams.toString());
    params.set("cursor", cursor);
    params.set("direction", direction);
    params.set("page", String(Math.max(1, page + (direction === "next" ? 1 : -1))));
    params.delete("artifact");
    startTransition(() => router.push(catalogHref(pathname, params) as Route, { scroll: false }));
  }

  function returnToFirstPage() {
    const params = new URLSearchParams(searchParams.toString());
    params.delete("cursor");
    params.delete("direction");
    params.delete("page");
    params.delete("artifact");
    startTransition(() => router.push(catalogHref(pathname, params) as Route, { scroll: false }));
  }

  const busy = isPending || !filtersSettled;

  return (
    <section aria-busy={busy} aria-labelledby="artifact-browser-title" className={styles.browser}>
      <header className={styles.browserHeader}>
        <div>
          <p className={styles.eyebrow}>Private artifact registry</p>
          <h1 id="artifact-browser-title" tabIndex={-1}>{title}</h1>
          <p>{description}</p>
        </div>
        <p aria-live="polite" className={styles.count}>{busy ? "Updating catalog…" : `${artifacts.length} on this page · ${filteredTotal} matching`}</p>
      </header>
      {recordedTotal === 0 ? (
        <div className={styles.emptyState}>
          <span aria-hidden="true">◫</span>
          <h2>No committed artifacts</h2>
          <p>When a trusted producer commits a reference, its evidence will appear here.</p>
        </div>
      ) : (
        <>
          <form aria-label="Artifact filters" className={styles.filters} onSubmit={(event) => event.preventDefault()}>
            <label className={styles.searchField} htmlFor="artifact-search">
              <span>Search evidence</span>
              <input
                id="artifact-search"
                onChange={(event) => setQuery(event.target.value)}
                placeholder="Artifact ID, run, kind, producer"
                type="search"
                value={query}
              />
            </label>
            <label>
              <span>Producer</span>
              <select onChange={(event) => setProducer(event.target.value)} value={producer}>
                <option value="all">All producers</option>
                {facets.producers.map((facet) => <option key={facet.value} value={facet.value}>{facet.value} ({facet.count})</option>)}
              </select>
            </label>
            <label>
              <span>Kind</span>
              <select onChange={(event) => setKind(event.target.value)} value={kind}>
                <option value="all">All kinds</option>
                {facets.kinds.map((facet) => <option key={facet.value} value={facet.value}>{facet.value} ({facet.count})</option>)}
              </select>
            </label>
            <label>
              <span>Group by</span>
              <select onChange={(event) => setGroupBy(event.target.value as ArtifactGroupBy)} value={groupBy}>
                <option value="producer">Producer</option>
                <option value="kind">Kind</option>
              </select>
            </label>
          </form>
          {activeFilters.length > 0 ? (
            <div aria-label="Active artifact filters" className={styles.appliedFilterNotice}>
              <p><strong>Active filters</strong> {activeFilters.join(" · ")}</p>
              <button disabled={busy} onClick={clearFilters} type="button">Clear filters</button>
            </div>
          ) : null}
          {filteredTotal === 0 ? (
            <div className={styles.filteredEmptyState}>
              <h2>No artifacts match these filters</h2>
              <p>Try another producer, kind, or identifier.</p>
              <button onClick={clearFilters} type="button">Clear filters</button>
            </div>
          ) : (
            <div
              aria-label="Artifact results"
              className={styles.resultsRegion}
              ref={resultsRef}
              tabIndex={-1}
            >
              {groups.length === 0 ? (
                <div className={styles.filteredEmptyState}>
                  <h2>This cursor page is no longer available</h2>
                  <p>The catalog changed while you were browsing. Return to the beginning of these filtered results.</p>
                  <button disabled={busy} onClick={returnToFirstPage} type="button">Return to first page</button>
                </div>
              ) : groups.map((group) => (
                <section aria-labelledby={`artifact-group-${group.id}`} className={styles.artifactGroup} key={group.id}>
                  <header className={styles.groupHeader}>
                    <h2 id={`artifact-group-${group.id}`}>{group.label}</h2>
                    <p>{group.artifacts.length} on this page</p>
                  </header>
                  <ArtifactTable artifacts={group.artifacts} labelledBy={`artifact-group-${group.id}`} onSelect={selectArtifact} selectedId={selectedArtifact?.id} />
                  <ArtifactCards artifacts={group.artifacts} onSelect={selectArtifact} selectedId={selectedArtifact?.id} />
                </section>
              ))}
              <CollectionPager
                busy={busy}
                currentPage={page}
                hasNextPage={pageInfo.hasNextPage}
                hasPreviousPage={pageInfo.hasPreviousPage}
                itemLabel="artifacts"
                onNextPage={() => movePage("next")}
                onPageSizeChange={setPageSize}
                onPreviousPage={() => movePage("previous")}
                pageSize={pageSize}
                totalItems={filteredTotal}
                visibleItems={artifacts.length}
              />
            </div>
          )}
        </>
      )}
      <ArtifactDetail artifact={selectedArtifact} onClose={closeDetail} returnFocusRef={returnFocusRef} />
    </section>
  );
}

function artifactFilterSignature(value: {
  groupBy: ArtifactGroupBy;
  kind: string;
  pageSize: number;
  producer: string;
  query: string;
}): string {
  return JSON.stringify([
    value.query.trim(),
    value.producer,
    value.kind,
    value.groupBy,
    value.pageSize,
  ]);
}

function setOptionalParam(params: URLSearchParams, key: string, value: string) {
  if (value) params.set(key, value);
  else params.delete(key);
}

function artifactActiveFilters(value: {
  kind: string;
  producer: string;
  query: string;
}): string[] {
  return [
    value.query.trim() ? `Search: ${value.query.trim()}` : null,
    value.producer === "all" ? null : `Producer: ${value.producer}`,
    value.kind === "all" ? null : `Kind: ${value.kind}`,
  ].filter((label): label is string => label !== null);
}

function catalogHref(pathname: string, params: URLSearchParams): string {
  const query = params.toString();
  return query ? `${pathname}?${query}` : pathname;
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
