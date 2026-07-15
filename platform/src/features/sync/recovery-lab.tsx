"use client";

import { useMemo, useRef, useState } from "react";

import {
  createRecoveryCard,
  createRecoveryDrillReport,
  parseRecoveryCard,
  recoveryCardIdentity,
  serializeRecoveryCard,
  serializeRecoveryDrillReport,
  type RecoveryCard,
  type RecoveryDrillReport,
} from "@/features/sync/recovery-artifacts";
import type { StashSummary } from "@/features/sync/sync-service";

const labFileLimitBytes = 64 * 1024 * 1024;

type LabStatus = {
  kind: "error" | "idle" | "success" | "working";
  message: string;
};

type PlanResponse = {
  receipt: string;
  state: "already_committed" | "object_present" | "upload_required";
  upload: {
    headers: Record<string, string>;
    method: "PUT";
    url: string;
  } | null;
};

type CommitResponse = { stash: StashSummary };

type DownloadResponse = {
  expected: { sha256: string; sizeBytes: number };
  grant: { headers: Record<string, string>; method: "GET"; url: string };
  stashId: string;
};

type HydrationEvidence = {
  attemptId: string;
  cardIdentity: string;
  downloadedSha256: string;
  startedAt: string;
};

export function RecoveryLab() {
  const operationInFlight = useRef(false);
  const [apiToken, setApiToken] = useState("");
  const [connected, setConnected] = useState(false);
  const [file, setFile] = useState<File | null>(null);
  const [stashId, setStashId] = useState("");
  const [stashes, setStashes] = useState<StashSummary[]>([]);
  const [recoveryCard, setRecoveryCard] = useState<RecoveryCard | null>(null);
  const [cardOrigin, setCardOrigin] = useState<"generated" | "imported" | null>(null);
  const [hydrationEvidence, setHydrationEvidence] = useState<HydrationEvidence | null>(null);
  const [recoveryReport, setRecoveryReport] = useState<RecoveryDrillReport | null>(null);
  const [status, setStatus] = useState<LabStatus>({
    kind: "idle",
    message: "Enter the local bearer token to unlock this simulated vault.",
  });

  const canPush = Boolean(
    connected && file && stashId.trim() && status.kind !== "working",
  );
  const currentCardIdentity = recoveryCard
    ? recoveryCardIdentity(recoveryCard)
    : null;
  const canVerifyReopenedFile = Boolean(
    recoveryCard &&
      hydrationEvidence &&
      hydrationEvidence.cardIdentity === currentCardIdentity &&
      hydrationEvidence.downloadedSha256 === recoveryCard.sha256,
  );
  const totalBytes = useMemo(
    () => stashes.reduce((total, stash) => total + stash.sizeBytes, 0),
    [stashes],
  );

  async function connectVault() {
    if (!apiToken.trim()) {
      setStatus({ kind: "error", message: "Enter the development bearer token." });
      return;
    }
    if (!beginOperation()) return;
    setHydrationEvidence(null);
    setRecoveryReport(null);
    try {
      setStatus({ kind: "working", message: "Authenticating and reading the vault…" });
      setStashes(await listStashes(apiToken));
      setConnected(true);
      setStatus({
        kind: "success",
        message: "Vault unlocked for this tab. The token is not persisted.",
      });
    } catch (error) {
      setConnected(false);
      setStashes([]);
      setStatus({ kind: "error", message: messageFor(error) });
    } finally {
      endOperation();
    }
  }

  function chooseArchive(nextFile: File | null) {
    if (operationInFlight.current) return;
    setRecoveryCard(null);
    setCardOrigin(null);
    setRecoveryReport(null);
    setHydrationEvidence(null);
    setFile(null);
    if (!nextFile) return;
    if (nextFile.size > labFileLimitBytes) {
      setStatus({
        kind: "error",
        message: `The browser lab is capped at ${formatBytes(labFileLimitBytes)}. Large archives belong in the future streaming Go client.`,
      });
      return;
    }
    setFile(nextFile);
    if (!stashId) {
      setStashId(nextFile.name.replace(/[^A-Za-z0-9._-]/g, "-").slice(0, 128));
    }
    setStatus({ kind: "idle", message: "Archive selected. No local file will be deleted." });
  }

  async function pushArchive() {
    if (!file || !stashId.trim()) return;
    if (!beginOperation()) return;

    try {
      setRecoveryCard(null);
      setCardOrigin(null);
      setRecoveryReport(null);
      setHydrationEvidence(null);
      setStatus({ kind: "working", message: "Hashing the exact archive bytes…" });
      const bytes = await file.arrayBuffer();
      const sha256 = await sha256Hex(bytes);
      const contentType = "application/vnd.filecheap.stash";

      setStatus({ kind: "working", message: "Requesting an immutable upload plan…" });
      const plan = await apiRequest<PlanResponse>("/api/v1/sync/plans", apiToken, {
        contentType,
        sha256,
        sizeBytes: file.size,
        stashId: stashId.trim(),
      });

      if (plan.upload) {
        setStatus({ kind: "working", message: "Transferring through the signed data path…" });
        const uploadResponse = await fetch(plan.upload.url, {
          body: file,
          headers: plan.upload.headers,
          method: plan.upload.method,
        });
        if (!uploadResponse.ok) throw await responseError(uploadResponse);
      }

      setStatus({ kind: "working", message: "Committing the catalog reference…" });
      const committed = await apiRequest<CommitResponse>(
        "/api/v1/sync/commits",
        apiToken,
        { receipt: plan.receipt },
      );
      setStashes(await listStashes(apiToken));
      setRecoveryCard(
        createRecoveryCard({
          committedAt: committed.stash.committedAt,
          originalFileName: file.name,
          sha256,
          sizeBytes: file.size,
          stashId: committed.stash.stashId,
        }),
      );
      setCardOrigin("generated");
      setStatus({
        kind: "success",
        message: `${committed.stash.stashId} committed. Export and re-import its card; use a clean profile for the strongest disaster drill.`,
      });
    } catch (error) {
      setStatus({ kind: "error", message: messageFor(error) });
    } finally {
      endOperation();
    }
  }

  async function importCard(cardFile: File | null) {
    if (!cardFile) return;
    if (!beginOperation()) return;
    try {
      setHydrationEvidence(null);
      setRecoveryReport(null);
      setStatus({ kind: "working", message: "Validating the portable recovery card…" });
      const card = parseRecoveryCard(await cardFile.text());
      setRecoveryCard(card);
      setCardOrigin("imported");
      setRecoveryReport(null);
      setHydrationEvidence(null);
      setFile(null);
      setStashId("");
      setStatus({
        kind: "success",
        message: `Recovery card loaded for ${card.stashId}. Hydrate to verify every byte.`,
      });
    } catch (error) {
      setRecoveryCard(null);
      setCardOrigin(null);
      setStatus({ kind: "error", message: `Invalid recovery card: ${messageFor(error)}` });
    } finally {
      endOperation();
    }
  }

  async function hydrateFromCard() {
    if (!connected || !recoveryCard) return;
    if (cardOrigin !== "imported") {
      setStatus({
        kind: "error",
        message: "Export and import this card before running the imported-card drill.",
      });
      return;
    }
    if (recoveryCard.sizeBytes > labFileLimitBytes) {
      setStatus({
        kind: "error",
        message: `This card describes ${formatBytes(recoveryCard.sizeBytes)}; the browser lab limit is ${formatBytes(labFileLimitBytes)}.`,
      });
      return;
    }
    if (!beginOperation()) return;

    const card = recoveryCard;
    const startedAt = new Date().toISOString();
    const attemptId = crypto.randomUUID();
    try {
      setHydrationEvidence(null);
      setRecoveryReport(null);
      setStatus({ kind: "working", message: `Downloading every byte of ${card.stashId}…` });
      const plan = await apiRequest<DownloadResponse>(
        "/api/v1/sync/downloads",
        apiToken,
        { stashId: card.stashId },
      );
      if (
        plan.stashId !== card.stashId ||
        plan.expected.sizeBytes !== card.sizeBytes ||
        plan.expected.sha256 !== card.sha256
      ) {
        throw new Error("The server download plan does not match this recovery card.");
      }

      const response = await fetch(plan.grant.url, {
        headers: plan.grant.headers,
        method: plan.grant.method,
      });
      if (!response.ok) throw await responseError(response);
      const bytes = await response.arrayBuffer();
      const sha256 = await sha256Hex(bytes);
      if (bytes.byteLength !== card.sizeBytes || sha256 !== card.sha256) {
        throw new Error("Recovered bytes failed the recovery card size or SHA-256 check.");
      }

      downloadBytes(bytes, card.originalFileName);
      setHydrationEvidence({
        attemptId,
        cardIdentity: recoveryCardIdentity(card),
        downloadedSha256: sha256,
        startedAt,
      });
      setStatus({
        kind: "success",
        message: "Downloaded bytes verified and offered for saving. Select the saved download below for a local content-equivalence check.",
      });
    } catch (error) {
      setHydrationEvidence(null);
      setStatus({ kind: "error", message: messageFor(error) });
    } finally {
      endOperation();
    }
  }

  async function verifySelectedFile(selectedFile: File | null) {
    if (
      !selectedFile ||
      !recoveryCard ||
      !hydrationEvidence ||
      hydrationEvidence.cardIdentity !== recoveryCardIdentity(recoveryCard) ||
      hydrationEvidence.downloadedSha256 !== recoveryCard.sha256
    ) return;
    if (!beginOperation()) return;
    const card = recoveryCard;
    const evidence = hydrationEvidence;
    try {
      if (selectedFile.size > labFileLimitBytes) {
        throw new Error(`The selected file exceeds the ${formatBytes(labFileLimitBytes)} lab limit.`);
      }
      setStatus({ kind: "working", message: "Hashing the file selected from disk…" });
      const bytes = await selectedFile.arrayBuffer();
      const sha256 = await sha256Hex(bytes);
      if (selectedFile.size !== card.sizeBytes || sha256 !== card.sha256) {
        throw new Error("The selected file does not match the recovery card.");
      }
      const completedAt = new Date().toISOString();
      setRecoveryReport(
        createRecoveryDrillReport({
          attemptId: evidence.attemptId,
          completedAt,
          recoveryCard: card,
          sha256,
          sizeBytes: selectedFile.size,
          startedAt: evidence.startedAt,
          stashId: card.stashId,
        }),
      );
      setStatus({
        kind: "success",
        message: `${card.stashId}: the selected local file is byte-equivalent to the verified download.`,
      });
    } catch (error) {
      setRecoveryReport(null);
      setStatus({ kind: "error", message: messageFor(error) });
    } finally {
      endOperation();
    }
  }

  function beginOperation(): boolean {
    if (operationInFlight.current) return false;
    operationInFlight.current = true;
    return true;
  }

  function endOperation(): void {
    operationInFlight.current = false;
  }

  return (
    <div className="labPanel">
      <div className="labToolbar">
        <label>
          <span>Development bearer token</span>
          <input
            autoComplete="off"
            disabled={status.kind === "working"}
            onChange={(event) => {
              setApiToken(event.target.value);
              setConnected(false);
              setStashes([]);
              setHydrationEvidence(null);
              setRecoveryReport(null);
              setStatus({
                kind: "idle",
                message: "Token changed. Unlock the vault again; it is not persisted.",
              });
            }}
            placeholder="local-development-token"
            spellCheck={false}
            type="password"
            value={apiToken}
          />
        </label>
        <button disabled={status.kind === "working"} onClick={connectVault} type="button">
          {connected ? "Reconnect" : "Unlock vault"}
        </button>
      </div>

      <div className="uploadRow">
        <label className="filePicker">
          <span>Archive · max {formatBytes(labFileLimitBytes)}</span>
          <input
            disabled={status.kind === "working"}
            onChange={(event) => {
              const selected = event.currentTarget.files?.[0] ?? null;
              event.currentTarget.value = "";
              chooseArchive(selected);
            }}
            type="file"
          />
          <strong>{file ? `${file.name} · ${formatBytes(file.size)}` : "Choose a test archive"}</strong>
        </label>
        <label className="stashField">
          <span>Stash ID</span>
          <input
            disabled={status.kind === "working"}
            maxLength={128}
            onChange={(event) => setStashId(event.target.value)}
            placeholder="investigation-01"
            spellCheck={false}
            value={stashId}
          />
        </label>
        <button disabled={!canPush} onClick={pushArchive} type="button">Push archive</button>
      </div>

      <div className={`labStatus ${status.kind}`} role="status" aria-live="polite">
        <span aria-hidden="true" />
        {status.message}
      </div>

      <section className="recoveryDrill" aria-labelledby="recovery-drill-title">
        <div className="vaultHeader">
          <div>
            <span>Portable recovery drill</span>
            <strong id="recovery-drill-title">Card → hydrate → download → compare</strong>
          </div>
          <label className={status.kind === "working" ? "compactPicker disabled" : "compactPicker"}>
            Import card
            <input
              accept="application/json,.json"
              disabled={status.kind === "working"}
              onChange={(event) => {
                const selected = event.currentTarget.files?.[0] ?? null;
                event.currentTarget.value = "";
                void importCard(selected);
              }}
              type="file"
            />
          </label>
        </div>
        {recoveryCard ? (
          <div className="recoveryCard">
            <div>
              <strong>{recoveryCard.stashId}</strong>
              <span>{recoveryCard.originalFileName} · {formatBytes(recoveryCard.sizeBytes)}</span>
              <span>{shortHash(recoveryCard.sha256)}</span>
              <span>{cardOrigin === "imported" ? "imported card · drill ready" : "generated card · export/import required"}</span>
            </div>
            <div className="recoveryActions">
              <button
                onClick={() => downloadJson(
                  serializeRecoveryCard(recoveryCard),
                  `${recoveryCard.stashId}.recovery-card.json`,
                )}
                type="button"
              >Export card</button>
              <button
                disabled={!connected || status.kind === "working" || cardOrigin !== "imported"}
                onClick={hydrateFromCard}
                type="button"
              >Hydrate + download</button>
              <label className={canVerifyReopenedFile && status.kind !== "working" ? "compactPicker" : "compactPicker disabled"}>
                Select saved download
                <input
                  disabled={!canVerifyReopenedFile || status.kind === "working"}
                  onChange={(event) => {
                    const selected = event.currentTarget.files?.[0] ?? null;
                    event.currentTarget.value = "";
                    void verifySelectedFile(selected);
                  }}
                  type="file"
                />
              </label>
              <button
                disabled={!recoveryReport}
                onClick={() => recoveryReport && downloadJson(
                  serializeRecoveryDrillReport(recoveryReport),
                  `${recoveryReport.stashId}.recovery-drill-report.json`,
                )}
                type="button"
              >Export report</button>
            </div>
          </div>
        ) : (
          <div className="emptyRecovery">Push an archive or import a recovery card. Use a clean profile for a disaster-recovery drill.</div>
        )}
      </section>

      <div className="vaultHeader">
        <div>
          <span>Authenticated catalog</span>
          <strong>{connected ? `${stashes.length} objects · ${formatBytes(totalBytes)}` : "Locked"}</strong>
        </div>
        <span className="tableHint">client full GET + SHA-256 required</span>
      </div>

      {!connected ? (
        <div className="emptyVault"><span aria-hidden="true">□</span><p>Unlock the vault to load its catalog.</p></div>
      ) : stashes.length === 0 ? (
        <div className="emptyVault"><span aria-hidden="true">□</span><p>No committed stashes yet.</p></div>
      ) : (
        <div className="stashList">
          {stashes.map((stash) => (
            <article key={stash.stashId}>
              <div>
                <strong>{stash.stashId}</strong>
                <span>{formatBytes(stash.sizeBytes)} · {shortHash(stash.sha256)}</span>
                <span>commit evidence: {stash.storageVerification}</span>
              </div>
            </article>
          ))}
        </div>
      )}
    </div>
  );
}

async function apiRequest<T>(path: string, token: string, body: unknown): Promise<T> {
  const response = await fetch(path, {
    body: JSON.stringify(body),
    headers: { authorization: `Bearer ${token}`, "content-type": "application/json" },
    method: "POST",
  });
  if (!response.ok) throw await responseError(response);
  return response.json() as Promise<T>;
}

async function listStashes(token: string): Promise<StashSummary[]> {
  const response = await fetch("/api/v1/stashes", {
    headers: { authorization: `Bearer ${token}` },
  });
  if (!response.ok) throw await responseError(response);
  return ((await response.json()) as { stashes: StashSummary[] }).stashes;
}

async function responseError(response: Response): Promise<Error> {
  try {
    const problem = (await response.json()) as {
      detail?: string;
      requestId?: string;
      title?: string;
    };
    const requestId = problem.requestId ? ` [${problem.requestId}]` : "";
    return new Error(`${problem.detail ?? problem.title ?? `Request failed (${response.status})`}${requestId}`);
  } catch {
    return new Error(`Request failed (${response.status})`);
  }
}

async function sha256Hex(bytes: ArrayBuffer): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", bytes);
  return Array.from(new Uint8Array(digest), (byte) => byte.toString(16).padStart(2, "0")).join("");
}

function downloadBytes(bytes: ArrayBuffer, fileName: string): void {
  const url = URL.createObjectURL(new Blob([bytes], { type: "application/octet-stream" }));
  clickDownload(url, fileName);
  setTimeout(() => URL.revokeObjectURL(url), 60_000);
}

function downloadJson(contents: string, fileName: string): void {
  const url = URL.createObjectURL(new Blob([contents], { type: "application/json" }));
  clickDownload(url, fileName);
  setTimeout(() => URL.revokeObjectURL(url), 0);
}

function clickDownload(url: string, fileName: string): void {
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = fileName;
  document.body.append(anchor);
  anchor.click();
  anchor.remove();
}

function messageFor(error: unknown): string {
  return error instanceof Error ? error.message : "The recovery lab failed unexpectedly.";
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 ** 2) return `${(bytes / 1024).toFixed(1)} KB`;
  if (bytes < 1024 ** 3) return `${(bytes / 1024 ** 2).toFixed(1)} MB`;
  return `${(bytes / 1024 ** 3).toFixed(1)} GB`;
}

function shortHash(hash: string): string {
  return `sha256:${hash.slice(0, 8)}…${hash.slice(-6)}`;
}
