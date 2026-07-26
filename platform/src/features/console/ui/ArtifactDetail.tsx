"use client";

import { useEffect, useRef, useState, type RefObject } from "react";

import type { ConsoleArtifact } from "./artifact-types";
import {
  artifactAvailabilityLabel,
  artifactIntegrityLabel,
  artifactStateLabel,
  formatArtifactBytes,
  formatArtifactDate,
} from "./artifact-types";
import styles from "./console.module.css";

interface ArtifactDetailProps {
  artifact: ConsoleArtifact | null;
  onClose: () => void;
  returnFocusRef?: RefObject<HTMLButtonElement | null>;
}

/** Controlled dialog that requests only short-lived authenticated transfers. */
export function ArtifactDetail({
  artifact,
  onClose,
  returnFocusRef,
}: ArtifactDetailProps) {
  const closeButtonRef = useRef<HTMLButtonElement>(null);
  const detailRef = useRef<HTMLElement>(null);
  const [actionState, setActionState] = useState<"idle" | "downloading" | "deleting">("idle");
  const [actionMessage, setActionMessage] = useState("");

  useEffect(() => {
    if (!artifact) return;
    closeButtonRef.current?.focus();
    function onKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        onClose();
        return;
      }
      if (event.key === "Tab") {
        const controls = [...(detailRef.current?.querySelectorAll<HTMLElement>('button:not([disabled]), a[href]') ?? [])];
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
    }
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [artifact, onClose]);

  if (!artifact) return null;

  function close() {
    setActionMessage("");
    setActionState("idle");
    onClose();
    returnFocusRef?.current?.focus();
  }

  async function download() {
    if (!artifact) return;
    setActionState("downloading");
    setActionMessage("");
    try {
      const response = await fetch("/api/console/artifacts/downloads", {
        body: JSON.stringify({ artifactId: artifact.id }),
        headers: { "content-type": "application/json" },
        method: "POST",
      });
      const payload = await response.json() as { detail?: string; download?: { url?: string } };
      if (!response.ok || !payload.download?.url) throw new Error(payload.detail ?? "A transfer could not be issued.");
      const link = document.createElement("a");
      link.href = payload.download.url;
      link.rel = "noreferrer noopener";
      link.target = "_blank";
      link.click();
      setActionState("idle");
    } catch (error) {
      setActionMessage(error instanceof Error ? error.message : "A transfer could not be issued.");
      setActionState("idle");
    }
  }

  async function remove() {
    if (!artifact) return;
    if (!window.confirm(`Permanently delete ${artifact.label}? The immutable bytes cannot be recovered from file.cheap.`)) return;
    setActionState("deleting");
    setActionMessage("");
    try {
      const response = await fetch(`/api/console/artifacts/${encodeURIComponent(artifact.id)}`, { method: "DELETE" });
      const payload = await response.json() as { detail?: string };
      if (!response.ok) throw new Error(payload.detail ?? "The artifact could not be deleted.");
      window.location.reload();
    } catch (error) {
      setActionMessage(error instanceof Error ? error.message : "The artifact could not be deleted.");
      setActionState("idle");
    }
  }

  return (
    <div className={styles.dialogLayer}>
      <button
        aria-label="Close artifact details"
        aria-hidden="true"
        className={styles.dialogBackdrop}
        onClick={close}
        tabIndex={-1}
        type="button"
      />
      <aside
        aria-describedby="artifact-detail-description"
        aria-labelledby="artifact-detail-title"
        aria-modal="true"
        className={styles.detail}
        id="artifact-detail"
        ref={detailRef}
        role="dialog"
      >
        <header className={styles.detailHeader}>
          <div>
            <p className={styles.eyebrow}>Artifact evidence</p>
            <h2 id="artifact-detail-title">{artifact.label}</h2>
          </div>
          <button
            aria-label={`Close details for ${artifact.label}`}
            className={styles.closeButton}
            onClick={close}
            ref={closeButtonRef}
            type="button"
          >
            ×
          </button>
        </header>
        <div className={styles.detailBody}>
          <p className={styles.detailIntro} id="artifact-detail-description">
            {artifact.description ?? "Review the recorded evidence before requesting a transfer or local restore."}
          </p>
          <dl className={styles.detailList}>
            <div><dt>State</dt><dd>{artifactStateLabel(artifact.state)}</dd></div>
            <div><dt>Availability</dt><dd>{artifactAvailabilityLabel(artifact.availability)}</dd></div>
            <div><dt>Verification</dt><dd>{artifactIntegrityLabel(artifact.integrity)}</dd></div>
            <div><dt>Size</dt><dd>{formatArtifactBytes(artifact.sizeBytes)}</dd></div>
            <div><dt>Created</dt><dd>{formatArtifactDate(artifact.createdAt)}</dd></div>
            <div><dt>Retention</dt><dd>{formatArtifactDate(artifact.expiresAt)}</dd></div>
            <div><dt>Producer</dt><dd className={styles.mono}>{artifact.producer.tool}{artifact.producer.version ? ` ${artifact.producer.version}` : ""}</dd></div>
            {artifact.producer.nativeId ? <div><dt>Native ID</dt><dd className={styles.mono}>{artifact.producer.nativeId}</dd></div> : null}
            {artifact.contentType ? <div><dt>Content type</dt><dd className={styles.mono}>{artifact.contentType}</dd></div> : null}
            {artifact.sha256 ? <div><dt>SHA-256</dt><dd className={`${styles.mono} ${styles.hash}`}>{artifact.sha256}</dd></div> : null}
          </dl>
          <p className={styles.boundaryNote}>
            Artifact bytes are never proxied or previewed by the console. A download action requests a short-lived grant for this exact verified object.
          </p>
          {artifact.availability === "cloud-ready" ? (
            <div className={styles.detailActions}>
              <button disabled={actionState !== "idle"} onClick={download} type="button">
                {actionState === "downloading" ? "Issuing transfer…" : "Download verified transfer"}
              </button>
              <button className={styles.dangerButton} disabled={actionState !== "idle"} onClick={remove} type="button">
                {actionState === "deleting" ? "Deleting…" : "Delete artifact"}
              </button>
            </div>
          ) : null}
          <p aria-live="polite" className={styles.actionMessage}>{actionMessage}</p>
        </div>
      </aside>
    </div>
  );
}
