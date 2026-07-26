"use client";

import { useEffect, useRef, useState, type RefObject } from "react";

import type { RunSummary } from "@/features/runs/contracts";

import { formatRunDate, formatRunDuration, runEvidenceCountLabel, runHealthLabel, runStatusLabel } from "./run-presentation";
import styles from "./runs.module.css";

type DetailTab = "summary" | "outcomes" | "evidence" | "provenance";

interface RunDetailProps {
  onClose: () => void;
  returnFocusRef?: RefObject<HTMLButtonElement | null>;
  run: RunSummary | null;
}

const tabs: readonly { id: DetailTab; label: string }[] = [
  { id: "summary", label: "Summary" },
  { id: "outcomes", label: "Outcomes" },
  { id: "evidence", label: "Evidence" },
  { id: "provenance", label: "Provenance" },
];

/** Metadata-only dialog; it never previews, fetches, or transfers artifact bytes. */
export function RunDetail({ onClose, returnFocusRef, run }: RunDetailProps) {
  const closeButtonRef = useRef<HTMLButtonElement>(null);
  const detailRef = useRef<HTMLElement>(null);
  const [tab, setTab] = useState<DetailTab>("summary");

  useEffect(() => {
    if (!run) return;
    closeButtonRef.current?.focus();
    function onKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        onClose();
        return;
      }
      if (event.key !== "Tab") return;
      const controls = [...(detailRef.current?.querySelectorAll<HTMLElement>("button:not([disabled])") ?? [])];
      const first = controls[0];
      const last = controls.at(-1);
      if (!first || !last) return;
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    }
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [onClose, run]);

  if (!run) return null;

  function close() {
    setTab("summary");
    onClose();
    returnFocusRef?.current?.focus();
  }

  const title = run.run.specName ?? run.run.nativeId;
  return (
    <div className={styles.dialogLayer}>
      <button aria-label="Close run details" className={styles.dialogBackdrop} onClick={close} tabIndex={-1} type="button" />
      <aside aria-labelledby="run-detail-title" aria-modal="true" className={styles.detail} id="run-detail" ref={detailRef} role="dialog">
        <header className={styles.detailHeader}>
          <div><p className={styles.eyebrow}>Run record</p><h2 id="run-detail-title">{title}</h2></div>
          <button aria-label={`Close details for ${title}`} className={styles.closeButton} onClick={close} ref={closeButtonRef} type="button">×</button>
        </header>
        <div aria-label="Run details" className={styles.tabList} role="tablist">
          {tabs.map((item) => <button aria-controls={`run-panel-${item.id}`} aria-selected={tab === item.id} className={tab === item.id ? styles.tabCurrent : styles.tab} id={`run-tab-${item.id}`} key={item.id} onClick={() => setTab(item.id)} role="tab" type="button">{item.label}</button>)}
        </div>
        <div className={styles.detailBody}>
          <section aria-labelledby={`run-tab-${tab}`} id={`run-panel-${tab}`} role="tabpanel">
            {tab === "summary" ? <Summary run={run} /> : null}
            {tab === "outcomes" ? <Outcomes run={run} /> : null}
            {tab === "evidence" ? <Evidence run={run} /> : null}
            {tab === "provenance" ? <Provenance run={run} /> : null}
          </section>
        </div>
      </aside>
    </div>
  );
}

function Summary({ run }: { run: RunSummary }) {
  return (
    <>
      <p className={styles.detailIntro}>This run was indexed from a metadata-only artifact. Counts and health reflect the recorded index.</p>
      <dl className={styles.detailList}>
        <DetailItem label="Status" value={runStatusLabel(run.run.status)} />
        <DetailItem label="Evidence health" value={runHealthLabel(run.health.state)} />
        <DetailItem label="Indexed evidence" value={runEvidenceCountLabel(run)} />
        <DetailItem label="Outcomes" value={`${run.outcomes.length} indexed of ${run.counts.outcomes} declared`} />
        <DetailItem label="Steps" value={String(run.counts.steps)} />
        <DetailItem label="Duration" value={formatRunDuration(run.run.durationMs)} />
        <DetailItem label="Started" value={formatRunDate(run.run.startedAt)} />
        <DetailItem label="Ended" value={formatRunDate(run.run.endedAt)} />
        <DetailItem label="Backend" value={run.run.backend ?? "Not recorded"} />
        <DetailItem label="Environment" value={run.run.environment ?? "Not recorded"} />
        <DetailItem label="Exit code" value={run.run.exitCode === undefined ? "Not recorded" : String(run.run.exitCode)} />
        {run.run.errorKind ? <DetailItem label="Error kind" value={run.run.errorKind} /> : null}
      </dl>
      <section aria-labelledby="health-reasons-title" className={styles.reasonsSection}>
        <h3 id="health-reasons-title">Health reasons</h3>
        {run.health.reasons.length > 0 ? <ul className={styles.labels}>{run.health.reasons.map((reason) => <li key={reason}>{reason}</li>)}</ul> : <p>No health reasons recorded.</p>}
      </section>
    </>
  );
}

function Outcomes({ run }: { run: RunSummary }) {
  if (run.outcomes.length === 0) return <EmptyPanel heading="No indexed outcomes" text="This run index contains no outcome metadata." />;
  return (
    <>
      <p className={styles.detailIntro}>{run.outcomes.length} indexed of {run.counts.outcomes} declared. Outcome status is recorded as supplied by the index.</p>
      <ul className={styles.outcomeList}>{run.outcomes.map((outcome) => <li key={outcome.id}><strong>{outcome.id}</strong><span>{outcome.status}</span></li>)}</ul>
    </>
  );
}

function Evidence({ run }: { run: RunSummary }) {
  if (run.evidence.length === 0) return <EmptyPanel heading="No indexed evidence" text="This run index contains no evidence metadata." />;
  return (
    <>
      <p className={styles.detailIntro}>{runEvidenceCountLabel(run)}. Evidence is metadata-only; bytes are never shown or previewed here.</p>
      <ul className={styles.evidenceList}>
        {run.evidence.map((evidence) => (
          <li key={evidence.path}>
            <strong>{evidence.role}</strong><span className={styles.mono}>{evidence.path}</span>
            <dl className={styles.detailList}>
              <DetailItem label="Medium" value={evidence.medium} />
              <DetailItem label="Presence" value={evidence.presence} />
              <DetailItem label="Integrity" value={evidence.integrity} />
              <DetailItem label="Sensitivity" value={evidence.sensitivity} />
              <DetailItem label="Inspectability" value={evidence.inspectability} />
            </dl>
          </li>
        ))}
      </ul>
    </>
  );
}

function Provenance({ run }: { run: RunSummary }) {
  return (
    <>
      <p className={styles.detailIntro}>Source, detector, and producer identity recorded with this run index.</p>
      <dl className={styles.detailList}>
        <DetailItem label="Source artifact" mono value={run.artifactId} />
        <DetailItem label="Source kind" mono value={run.source.kind} />
        <DetailItem label="Source content type" mono value={run.source.contentType} />
        <DetailItem label="Source SHA-256" mono value={run.source.sha256} />
        <DetailItem label="Detector" mono value={`${run.detector.name} ${run.detector.version}`} />
        <DetailItem label="Run index SHA-256" mono value={run.runIndexSha256} />
        <DetailItem label="Tool" mono value={run.producer.tool} />
        <DetailItem label="Version" mono value={run.producer.version ?? "Not recorded"} />
        <DetailItem label="Native ID" mono value={run.producer.native_id} />
        <DetailItem label="Native schema" mono value={run.producer.native_schema} />
        <DetailItem label="Entrypoint" mono value={run.producer.entrypoint ?? "Not recorded"} />
        <DetailItem label="Series key" mono value={run.run.seriesKey} />
        <DetailItem label="Recorded" value={formatRunDate(run.createdAt)} />
        <DetailItem label="Last updated" value={formatRunDate(run.updatedAt)} />
      </dl>
    </>
  );
}

function DetailItem({ label, mono = false, value }: { label: string; mono?: boolean; value: string }) {
  return <div><dt>{label}</dt><dd className={mono ? styles.mono : undefined}>{value}</dd></div>;
}

function EmptyPanel({ heading, text }: { heading: string; text: string }) {
  return <div className={styles.panelEmpty}><h3>{heading}</h3><p>{text}</p></div>;
}
