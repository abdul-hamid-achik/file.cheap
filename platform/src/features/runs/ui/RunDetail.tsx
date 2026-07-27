"use client";

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type KeyboardEvent as ReactKeyboardEvent,
  type RefObject,
} from "react";

import type { RunSummary } from "@/features/runs/contracts";

import {
  formatRunDate,
  formatRunDuration,
  getNextRunDetailTab,
  runDetailTabOrder,
  runEvidenceCountLabel,
  runHealthLabel,
  runStatusLabel,
  type RunDetailTab,
} from "./run-presentation";
import styles from "./runs.module.css";

interface RunDetailProps {
  onClose: () => void;
  returnFocusRef?: RefObject<HTMLElement | null>;
  run: RunSummary | null;
}

const tabLabels: Record<RunDetailTab, string> = {
  evidence: "Evidence",
  outcomes: "Outcomes",
  provenance: "Provenance",
  summary: "Summary",
};

/** Metadata-only dialog; it never previews, fetches, or transfers artifact bytes. */
export function RunDetail({ onClose, returnFocusRef, run }: RunDetailProps) {
  const closeButtonRef = useRef<HTMLButtonElement>(null);
  const detailRef = useRef<HTMLElement>(null);
  const tabRefs = useRef<Partial<Record<RunDetailTab, HTMLButtonElement | null>>>({});
  const [copyFeedback, setCopyFeedback] = useState<{ artifactId: string; field?: string; message: string } | null>(null);
  const [tab, setTab] = useState<RunDetailTab>("summary");

  const close = useCallback(() => {
    setCopyFeedback(null);
    setTab("summary");
    onClose();
    returnFocusRef?.current?.focus();
  }, [onClose, returnFocusRef]);

  useEffect(() => {
    if (!run) return;
    closeButtonRef.current?.focus();
    function onKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        close();
        return;
      }
      if (event.key !== "Tab") return;
      const controls = [...(detailRef.current?.querySelectorAll<HTMLElement>('button:not([disabled]):not([tabindex="-1"]), a[href], [tabindex]:not([tabindex="-1"])') ?? [])];
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
  }, [close, run]);

  useEffect(() => {
    if (!run) return;
    const scrollContainer = document.getElementById("main-content");
    const previousBodyOverflow = document.body.style.overflow;
    const previousRootOverflow = document.documentElement.style.overflow;
    const previousScrollContainerOverflow = scrollContainer?.style.overflow ?? "";
    document.body.style.overflow = "hidden";
    document.documentElement.style.overflow = "hidden";
    if (scrollContainer) scrollContainer.style.overflow = "hidden";
    return () => {
      document.body.style.overflow = previousBodyOverflow;
      document.documentElement.style.overflow = previousRootOverflow;
      if (scrollContainer) scrollContainer.style.overflow = previousScrollContainerOverflow;
    };
  }, [run]);

  if (!run) return null;

  const currentCopyFeedback = copyFeedback?.artifactId === run.artifactId ? copyFeedback : null;

  async function copyMetadata(label: string, value: string) {
    if (!run) return;
    try {
      if (!navigator.clipboard) throw new Error("Clipboard access is unavailable.");
      await navigator.clipboard.writeText(value);
      setCopyFeedback({ artifactId: run.artifactId, field: label, message: `${label} copied to clipboard.` });
    } catch {
      setCopyFeedback({ artifactId: run.artifactId, message: `${label} could not be copied. Select the value and copy it manually.` });
    }
  }

  function moveTab(event: ReactKeyboardEvent<HTMLButtonElement>, currentTab: RunDetailTab) {
    const nextTab = getNextRunDetailTab(currentTab, event.key);
    if (!nextTab) return;
    event.preventDefault();
    setTab(nextTab);
    tabRefs.current[nextTab]?.focus();
  }

  const title = run.run.specName ?? run.run.nativeId;
  return (
    <div className={styles.dialogLayer}>
      <button aria-hidden="true" aria-label="Close run details" className={styles.dialogBackdrop} onClick={close} tabIndex={-1} type="button" />
      <aside aria-labelledby="run-detail-title" aria-modal="true" className={styles.detail} id="run-detail" ref={detailRef} role="dialog">
        <header className={styles.detailHeader}>
          <div><p className={styles.eyebrow}>Run record</p><h2 id="run-detail-title">{title}</h2></div>
          <button aria-label={`Close details for ${title}`} className={styles.closeButton} onClick={close} ref={closeButtonRef} type="button">×</button>
        </header>
        <div aria-label="Run details" aria-orientation="horizontal" className={styles.tabList} role="tablist">
          {runDetailTabOrder.map((item) => (
            <button
              aria-controls={`run-panel-${item}`}
              aria-selected={tab === item}
              className={tab === item ? styles.tabCurrent : styles.tab}
              id={`run-tab-${item}`}
              key={item}
              onClick={() => setTab(item)}
              onKeyDown={(event) => moveTab(event, item)}
              ref={(element) => { tabRefs.current[item] = element; }}
              role="tab"
              tabIndex={tab === item ? 0 : -1}
              type="button"
            >
              {tabLabels[item]}
            </button>
          ))}
        </div>
        <div className={styles.detailBody}>
          <section aria-labelledby={`run-tab-${tab}`} id={`run-panel-${tab}`} role="tabpanel" tabIndex={0}>
            {tab === "summary" ? <Summary run={run} /> : null}
            {tab === "outcomes" ? <Outcomes run={run} /> : null}
            {tab === "evidence" ? <Evidence run={run} /> : null}
            {tab === "provenance" ? (
              <Provenance
                copiedField={currentCopyFeedback?.field}
                onCopy={copyMetadata}
                run={run}
              />
            ) : null}
          </section>
          <p aria-live="polite" className={styles.srOnly} role="status">{currentCopyFeedback?.message ?? ""}</p>
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

function Provenance({
  copiedField,
  onCopy,
  run,
}: {
  copiedField?: string;
  onCopy: (label: string, value: string) => Promise<void>;
  run: RunSummary;
}) {
  return (
    <>
      <p className={styles.detailIntro}>Source, detector, and producer identity recorded with this run index.</p>
      <dl className={styles.detailList}>
        <CopyableDetailItem copied={copiedField === "Source artifact"} label="Source artifact" onCopy={onCopy} value={run.artifactId} />
        <DetailItem label="Source kind" mono value={run.source.kind} />
        <DetailItem label="Source content type" mono value={run.source.contentType} />
        <CopyableDetailItem copied={copiedField === "Source SHA-256"} label="Source SHA-256" onCopy={onCopy} value={run.source.sha256} />
        <DetailItem label="Detector" mono value={`${run.detector.name} ${run.detector.version}`} />
        <CopyableDetailItem copied={copiedField === "Run index SHA-256"} label="Run index SHA-256" onCopy={onCopy} value={run.runIndexSha256} />
        <DetailItem label="Tool" mono value={run.producer.tool} />
        <DetailItem label="Version" mono value={run.producer.version ?? "Not recorded"} />
        <CopyableDetailItem copied={copiedField === "Native ID"} label="Native ID" onCopy={onCopy} value={run.producer.native_id} />
        <DetailItem label="Native schema" mono value={run.producer.native_schema} />
        <DetailItem label="Entrypoint" mono value={run.producer.entrypoint ?? "Not recorded"} />
        <CopyableDetailItem copied={copiedField === "Series key"} label="Series key" onCopy={onCopy} value={run.run.seriesKey} />
        <DetailItem label="Recorded" value={formatRunDate(run.createdAt)} />
        <DetailItem label="Last updated" value={formatRunDate(run.updatedAt)} />
      </dl>
    </>
  );
}

function DetailItem({ label, mono = false, value }: { label: string; mono?: boolean; value: string }) {
  return <div><dt>{label}</dt><dd className={mono ? styles.mono : undefined}>{value}</dd></div>;
}

function CopyableDetailItem({
  copied,
  label,
  onCopy,
  value,
}: {
  copied: boolean;
  label: string;
  onCopy: (label: string, value: string) => Promise<void>;
  value: string;
}) {
  return (
    <div>
      <dt>{label}</dt>
      <dd className={`${styles.mono} ${styles.copyableValue}`}>
        <span>{value}</span>
        <button aria-label={`Copy ${label}`} className={styles.copyButton} onClick={() => void onCopy(label, value)} type="button">
          {copied ? "Copied" : "Copy"}
        </button>
      </dd>
    </div>
  );
}

function EmptyPanel({ heading, text }: { heading: string; text: string }) {
  return <div className={styles.panelEmpty}><h3>{heading}</h3><p>{text}</p></div>;
}
