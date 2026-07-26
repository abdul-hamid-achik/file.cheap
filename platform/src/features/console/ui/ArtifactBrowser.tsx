"use client";

import { useMemo, useRef, useState } from "react";

import { ArtifactCards } from "./ArtifactCards";
import { ArtifactDetail } from "./ArtifactDetail";
import { ArtifactTable } from "./ArtifactTable";
import {
  groupConsoleArtifacts,
  type ArtifactGroupBy,
  type ConsoleArtifact,
} from "./artifact-types";
import styles from "./console.module.css";

interface ArtifactBrowserProps {
  artifacts: readonly ConsoleArtifact[];
  description?: string;
  title?: string;
}

/**
 * Small client boundary for selection and an accessible details dialog. The
 * caller owns filtering, paging, auth, data loading, and transfer actions.
 */
export function ArtifactBrowser({
  artifacts,
  description = "Inspect provenance, availability, and verification before restoring or transferring bytes.",
  title = "Artifacts",
}: ArtifactBrowserProps) {
  const [selected, setSelected] = useState<ConsoleArtifact | null>(null);
  const [groupBy, setGroupBy] = useState<ArtifactGroupBy>("producer");
  const [kind, setKind] = useState("all");
  const [producer, setProducer] = useState("all");
  const [query, setQuery] = useState("");
  const triggerRef = useRef<HTMLButtonElement>(null);

  const producers = useMemo<string[]>(
    () => [...new Set(artifacts.map((artifact) => artifact.producer.tool))].sort(),
    [artifacts],
  );
  const kinds = useMemo<string[]>(
    () => [...new Set(artifacts.map((artifact) => artifact.kind))].sort(),
    [artifacts],
  );
  const filteredArtifacts = useMemo(() => {
    const needle = query.trim().toLocaleLowerCase();
    return artifacts.filter((artifact) => {
      const matchesQuery =
        needle === "" ||
        [artifact.id, artifact.kind, artifact.label, artifact.producer.nativeId, artifact.producer.tool]
          .filter((value): value is string => Boolean(value))
          .some((value) => value.toLocaleLowerCase().includes(needle));
      return matchesQuery &&
        (producer === "all" || artifact.producer.tool === producer) &&
        (kind === "all" || artifact.kind === kind);
    });
  }, [artifacts, kind, producer, query]);
  const groups = useMemo(
    () => groupConsoleArtifacts(filteredArtifacts, groupBy),
    [filteredArtifacts, groupBy],
  );
  const selectedArtifact = selected && filteredArtifacts.some((artifact) => artifact.id === selected.id)
    ? selected
    : null;

  function selectArtifact(artifact: ConsoleArtifact, trigger: HTMLButtonElement) {
    triggerRef.current = trigger;
    setSelected(artifact);
  }

  function closeDetail() {
    setSelected(null);
  }

  return (
    <section aria-labelledby="artifact-browser-title" className={styles.browser}>
      <header className={styles.browserHeader}>
        <div>
          <p className={styles.eyebrow}>Private artifact registry</p>
          <h1 id="artifact-browser-title">{title}</h1>
          <p>{description}</p>
        </div>
        <p aria-live="polite" className={styles.count}>{filteredArtifacts.length} of {artifacts.length} recorded</p>
      </header>
      {artifacts.length === 0 ? (
        <div className={styles.emptyState}>
          <span aria-hidden="true">◫</span>
          <h2>No committed artifacts</h2>
          <p>When a trusted producer commits a reference, its evidence will appear here.</p>
        </div>
      ) : (
        <>
          <form className={styles.filters} onSubmit={(event) => event.preventDefault()}>
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
                {producers.map((value: string) => <option key={value} value={value}>{value}</option>)}
              </select>
            </label>
            <label>
              <span>Kind</span>
              <select onChange={(event) => setKind(event.target.value)} value={kind}>
                <option value="all">All kinds</option>
                {kinds.map((value: string) => <option key={value} value={value}>{value}</option>)}
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
          {groups.length === 0 ? (
            <div className={styles.filteredEmptyState}>
              <h2>No artifacts match these filters</h2>
              <p>Try another producer, kind, or identifier.</p>
              <button onClick={() => { setKind("all"); setProducer("all"); setQuery(""); }} type="button">Clear filters</button>
            </div>
          ) : groups.map((group) => (
            <section aria-labelledby={`artifact-group-${group.id}`} className={styles.artifactGroup} key={group.id}>
              <header className={styles.groupHeader}>
                <h2 id={`artifact-group-${group.id}`}>{group.label}</h2>
                <p>{group.artifacts.length} artifact{group.artifacts.length === 1 ? "" : "s"}</p>
              </header>
              <ArtifactTable artifacts={group.artifacts} labelledBy={`artifact-group-${group.id}`} onSelect={selectArtifact} selectedId={selectedArtifact?.id} />
              <ArtifactCards artifacts={group.artifacts} onSelect={selectArtifact} selectedId={selectedArtifact?.id} />
            </section>
          ))}
        </>
      )}
      <ArtifactDetail artifact={selectedArtifact} onClose={closeDetail} returnFocusRef={triggerRef} />
    </section>
  );
}
