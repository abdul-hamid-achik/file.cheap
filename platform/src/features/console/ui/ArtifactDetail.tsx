"use client";

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type Ref,
  type RefObject,
} from "react";

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
  returnFocusRef?: RefObject<HTMLElement | null>;
}

/** Controlled dialog that requests only short-lived authenticated transfers. */
export function ArtifactDetail({
  artifact,
  onClose,
  returnFocusRef,
}: ArtifactDetailProps) {
  const closeButtonRef = useRef<HTMLButtonElement>(null);
  const deleteConfirmRef = useRef<HTMLButtonElement>(null);
  const deleteTriggerRef = useRef<HTMLButtonElement>(null);
  const detailRef = useRef<HTMLElement>(null);
  const restoreDeleteFocusRef = useRef(false);
  const [actionState, setActionState] = useState<"idle" | "downloading" | "deleting">("idle");
  const [actionMessage, setActionMessage] = useState("");
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [copyFeedback, setCopyFeedback] = useState<{ artifactId: string; field?: string; message: string } | null>(null);

  const close = useCallback(() => {
    setActionMessage("");
    setActionState("idle");
    setConfirmDelete(false);
    setCopyFeedback(null);
    onClose();
    returnFocusRef?.current?.focus();
  }, [onClose, returnFocusRef]);

  useEffect(() => {
    if (!artifact) return;
    closeButtonRef.current?.focus();
    function onKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        close();
        return;
      }
      if (event.key === "Tab") {
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
    }
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [artifact, close]);

  useEffect(() => {
    if (!artifact) return;
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
  }, [artifact]);

  useEffect(() => {
    if (confirmDelete) {
      deleteConfirmRef.current?.focus();
      return;
    }
    if (!restoreDeleteFocusRef.current) return;
    restoreDeleteFocusRef.current = false;
    deleteTriggerRef.current?.focus();
  }, [confirmDelete]);

  if (!artifact) return null;

  const currentCopyFeedback = copyFeedback?.artifactId === artifact.id ? copyFeedback : null;
  const pullCommand = artifactPullCommand(artifact.id);

  async function copyMetadata(label: string, value: string) {
    if (!artifact) return;
    try {
      if (!navigator.clipboard) throw new Error("Clipboard access is unavailable.");
      await navigator.clipboard.writeText(value);
      setCopyFeedback({ artifactId: artifact.id, field: label, message: `${label} copied to clipboard.` });
    } catch {
      setCopyFeedback({ artifactId: artifact.id, message: `${label} could not be copied. Select the value and copy it manually.` });
    }
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

  function cancelDelete() {
    restoreDeleteFocusRef.current = true;
    setConfirmDelete(false);
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
            <CopyableArtifactValue
              copied={currentCopyFeedback?.field === "Artifact ID"}
              label="Artifact ID"
              onCopy={copyMetadata}
              value={artifact.id}
            />
            <div><dt>State</dt><dd>{artifactStateLabel(artifact.state)}</dd></div>
            <div><dt>Availability</dt><dd>{artifactAvailabilityLabel(artifact.availability)}</dd></div>
            <div><dt>Verification</dt><dd>{artifactIntegrityLabel(artifact.integrity)}</dd></div>
            <div><dt>Size</dt><dd>{formatArtifactBytes(artifact.sizeBytes)}</dd></div>
            <div><dt>Created</dt><dd>{formatArtifactDate(artifact.createdAt)}</dd></div>
            <div><dt>Retention</dt><dd>{formatArtifactDate(artifact.expiresAt)}</dd></div>
            <div><dt>Producer</dt><dd className={styles.mono}>{artifact.producer.tool}{artifact.producer.version ? ` ${artifact.producer.version}` : ""}</dd></div>
            {artifact.producer.nativeId ? (
              <CopyableArtifactValue
                copied={currentCopyFeedback?.field === "Native ID"}
                label="Native ID"
                onCopy={copyMetadata}
                value={artifact.producer.nativeId}
              />
            ) : null}
            {artifact.contentType ? <div><dt>Content type</dt><dd className={styles.mono}>{artifact.contentType}</dd></div> : null}
            {artifact.sha256 ? (
              <CopyableArtifactValue
                copied={currentCopyFeedback?.field === "SHA-256"}
                hash
                label="SHA-256"
                onCopy={copyMetadata}
                value={artifact.sha256}
              />
            ) : null}
          </dl>
          <p aria-live="polite" className={styles.srOnly} role="status">{currentCopyFeedback?.message ?? ""}</p>
          <p className={styles.boundaryNote}>
            {artifact.availability === "cloud-ready"
              ? "Artifact bytes are never proxied or previewed by the console. Browser downloads use a short-lived grant, but this page cannot verify the bytes after your browser saves them."
              : "Artifact bytes are never proxied or previewed by the console. This record is not currently eligible for a private cloud transfer."}
          </p>
          {artifact.availability === "cloud-ready" ? (
            <>
              <section aria-labelledby="artifact-cli-recovery-title" className={styles.recoveryOption}>
                <div>
                  <p className={styles.eyebrow}>End-to-end verification</p>
                  <h3 id="artifact-cli-recovery-title">Recover with the CLI</h3>
                  <p>The CLI streams directly to a new local file and verifies its SHA-256 before keeping it. It never extracts or opens the downloaded bytes.</p>
                </div>
                <div className={styles.commandCopy}>
                  <code>{pullCommand}</code>
                  <button
                    aria-label="Copy verified CLI recovery command"
                    className={styles.copyButton}
                    onClick={() => void copyMetadata("CLI command", pullCommand)}
                    type="button"
                  >
                    {currentCopyFeedback?.field === "CLI command" ? "Copied" : "Copy command"}
                  </button>
                </div>
                <p className={styles.commandHint}>Change the output filename if needed. The command refuses to overwrite an existing file.</p>
              </section>
              <div className={styles.detailActions}>
                <button disabled={actionState !== "idle"} onClick={download} type="button">
                  {actionState === "downloading" ? "Issuing transfer…" : "Download in browser"}
                </button>
                {confirmDelete ? (
                  <ArtifactDeleteConfirmation
                    artifactId={artifact.id}
                    busy={actionState !== "idle"}
                    confirmButtonRef={deleteConfirmRef}
                    deleting={actionState === "deleting"}
                    onCancel={cancelDelete}
                    onConfirm={remove}
                  />
                ) : (
                  <button
                    className={styles.dangerButton}
                    disabled={actionState !== "idle"}
                    onClick={() => setConfirmDelete(true)}
                    ref={deleteTriggerRef}
                    type="button"
                  >
                    Delete artifact
                  </button>
                )}
                <p aria-live="polite" className={styles.actionMessage}>{actionMessage}</p>
              </div>
            </>
          ) : null}
        </div>
      </aside>
    </div>
  );
}

export function artifactPullCommand(artifactId: string): string {
  return `fcheap pull ${artifactId} --output ./artifact-download.bin`;
}

export function ArtifactDeleteConfirmation({
  artifactId,
  busy,
  confirmButtonRef,
  deleting,
  onCancel,
  onConfirm,
}: {
  artifactId: string;
  busy: boolean;
  confirmButtonRef?: Ref<HTMLButtonElement>;
  deleting: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  const descriptionId = "artifact-delete-description";
  return (
    <div
      aria-describedby={descriptionId}
      aria-label="Permanent artifact deletion"
      className={styles.deleteConfirmation}
      role="group"
    >
      <p id={descriptionId}>Permanently delete <strong>{artifactId}</strong>? Its immutable bytes cannot be recovered from file.cheap.</p>
      <div>
        <button
          aria-describedby={descriptionId}
          className={styles.dangerButton}
          disabled={busy}
          onClick={onConfirm}
          ref={confirmButtonRef}
          type="button"
        >
          {deleting ? "Deleting…" : "Confirm permanent delete"}
        </button>
        <button disabled={busy} onClick={onCancel} type="button">Cancel</button>
      </div>
    </div>
  );
}

function CopyableArtifactValue({
  copied,
  hash = false,
  label,
  onCopy,
  value,
}: {
  copied: boolean;
  hash?: boolean;
  label: string;
  onCopy: (label: string, value: string) => Promise<void>;
  value: string;
}) {
  return (
    <div>
      <dt>{label}</dt>
      <dd className={`${styles.mono} ${hash ? styles.hash : ""} ${styles.copyableValue}`}>
        <span>{value}</span>
        <button aria-label={`Copy ${label}`} className={styles.copyButton} onClick={() => void onCopy(label, value)} type="button">
          {copied ? "Copied" : "Copy"}
        </button>
      </dd>
    </div>
  );
}
