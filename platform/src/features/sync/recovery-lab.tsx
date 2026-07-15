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
import { stashContentType, stashIdSchema } from "@/features/sync/contracts";
import type { StashSummary } from "@/features/sync/sync-service";

const labFileLimitBytes = 64 * 1024 * 1024;
const recoveryCardFileLimitBytes = 64 * 1024;
const objectUrlLifetimeMilliseconds = 60_000;

type LabStatus = {
  kind: "error" | "idle" | "success" | "working";
  message: string;
  requestId?: string;
};

type RecoveryStepStatus = "active" | "complete" | "pending";

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
  const [stashIdTouched, setStashIdTouched] = useState(false);
  const [stashes, setStashes] = useState<StashSummary[]>([]);
  const [recoveryCard, setRecoveryCard] = useState<RecoveryCard | null>(null);
  const [cardOrigin, setCardOrigin] = useState<"generated" | "imported" | null>(null);
  const [cardDownloadRequested, setCardDownloadRequested] = useState(false);
  const [hydrationEvidence, setHydrationEvidence] = useState<HydrationEvidence | null>(null);
  const [recoveryReport, setRecoveryReport] = useState<RecoveryDrillReport | null>(null);
  const [reportDownloadRequested, setReportDownloadRequested] = useState(false);
  const [status, setStatus] = useState<LabStatus>({
    kind: "idle",
    message: "Enter the local bearer token to unlock this simulated vault.",
  });

  const isWorking = status.kind === "working";
  const parsedStashId = stashIdSchema.safeParse(stashId.trim());
  const showStashIdError = stashIdTouched && !parsedStashId.success;
  const canPush = Boolean(connected && file && parsedStashId.success && !isWorking);
  const currentCardIdentity = recoveryCard
    ? recoveryCardIdentity(recoveryCard)
    : null;
  const canVerifyReopenedFile = Boolean(
    recoveryCard &&
      hydrationEvidence &&
      hydrationEvidence.cardIdentity === currentCardIdentity &&
      hydrationEvidence.downloadedSha256 === recoveryCard.sha256,
  );
  const totalLogicalBytes = useMemo(
    () => stashes.reduce((total, stash) => total + stash.sizeBytes, 0),
    [stashes],
  );
  const cardStepStatus: RecoveryStepStatus =
    cardOrigin === "imported" || cardDownloadRequested
      ? "complete"
      : recoveryCard
        ? "active"
        : "pending";
  const importStepStatus: RecoveryStepStatus =
    cardOrigin === "imported"
      ? "complete"
      : !recoveryCard || cardDownloadRequested
        ? "active"
        : "pending";
  const hydrateStepStatus: RecoveryStepStatus = canVerifyReopenedFile
    ? "complete"
    : cardOrigin === "imported" && connected
      ? "active"
      : "pending";
  const compareStepStatus: RecoveryStepStatus = recoveryReport
    ? "complete"
    : canVerifyReopenedFile
      ? "active"
      : "pending";
  const reportStepStatus: RecoveryStepStatus = reportDownloadRequested
    ? "complete"
    : recoveryReport
      ? "active"
      : "pending";

  async function connectVault() {
    if (!apiToken.trim()) {
      setStatus({ kind: "error", message: "Enter the development bearer token." });
      return;
    }
    if (!beginOperation()) return;
    try {
      setStatus({ kind: "working", message: "Authenticating and reading the vault…" });
      setStashes(await listStashes(apiToken.trim()));
      setConnected(true);
      setStatus({
        kind: "success",
        message: "Vault unlocked for this tab. The token is not persisted.",
      });
    } catch (error) {
      setConnected(false);
      setStashes([]);
      setStatus(errorStatus(error));
    } finally {
      endOperation();
    }
  }

  function chooseArchive(nextFile: File | null) {
    if (operationInFlight.current) return;
    if (!nextFile) return;
    if (nextFile.size === 0) {
      setStatus({ kind: "error", message: "Choose a non-empty archive." });
      return;
    }
    if (nextFile.size > labFileLimitBytes) {
      setStatus({
        kind: "error",
        message: `The browser lab is capped at ${formatBytes(labFileLimitBytes)}. Large archives belong in the future streaming Go client.`,
      });
      return;
    }

    const derivedStashId = deriveStashId(nextFile.name);
    const validatedStashId = stashIdSchema.safeParse(derivedStashId);
    if (!validatedStashId.success) {
      setStatus({
        kind: "error",
        message: "Could not derive a valid stash ID from this filename. Enter one manually.",
      });
      return;
    }
    setStashId(validatedStashId.data);
    setStashIdTouched(false);

    setFile(nextFile);
    setStatus({ kind: "idle", message: "Archive selected. No local file will be deleted." });
  }

  async function pushArchive() {
    if (!file) return;
    const validatedStashId = stashIdSchema.safeParse(stashId.trim());
    if (!validatedStashId.success) {
      setStatus({
        kind: "error",
        message: "Enter a valid stash ID using letters, numbers, dots, underscores, or hyphens.",
      });
      return;
    }
    if (!beginOperation()) return;

    try {
      setStatus({ kind: "working", message: "Hashing the exact archive bytes…" });
      const bytes = await file.arrayBuffer();
      const sha256 = await sha256Hex(bytes);
      const contentType = stashContentType;

      setStatus({ kind: "working", message: "Requesting an immutable upload plan…" });
      const plan = await apiRequest<PlanResponse>("/api/v1/sync/plans", apiToken.trim(), {
        contentType,
        sha256,
        sizeBytes: file.size,
        stashId: validatedStashId.data,
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
        apiToken.trim(),
        { receipt: plan.receipt },
      );
      const nextRecoveryCard = createRecoveryCard({
        committedAt: committed.stash.committedAt,
        originalFileName: file.name,
        sha256,
        sizeBytes: file.size,
        stashId: committed.stash.stashId,
      });
      setRecoveryCard(nextRecoveryCard);
      setCardOrigin("generated");
      setCardDownloadRequested(false);
      setHydrationEvidence(null);
      setRecoveryReport(null);
      setReportDownloadRequested(false);
      setStashes(await listStashes(apiToken.trim()));
      setStatus({
        kind: "success",
        message: `${committed.stash.stashId} committed. Export and re-import its card; use a clean profile for the strongest disaster drill.`,
      });
    } catch (error) {
      setStatus(
        errorStatus(
          error,
          undefined,
          Boolean(recoveryCard || hydrationEvidence || recoveryReport),
        ),
      );
    } finally {
      endOperation();
    }
  }

  async function importCard(cardFile: File | null) {
    if (!cardFile) return;
    if (cardFile.size === 0) {
      setStatus({ kind: "error", message: "Choose a non-empty recovery card." });
      return;
    }
    if (cardFile.size > recoveryCardFileLimitBytes) {
      setStatus({
        kind: "error",
        message: `Recovery cards must be ${formatBytes(recoveryCardFileLimitBytes)} or smaller.`,
      });
      return;
    }
    if (!beginOperation()) return;
    try {
      setStatus({ kind: "working", message: "Validating the portable recovery card…" });
      const card = parseRecoveryCard(await cardFile.text());
      setRecoveryCard(card);
      setCardOrigin("imported");
      setCardDownloadRequested(true);
      setRecoveryReport(null);
      setHydrationEvidence(null);
      setReportDownloadRequested(false);
      setFile(null);
      setStashId("");
      setStashIdTouched(false);
      setStatus({
        kind: "success",
        message: `Recovery card loaded for ${card.stashId}. Hydrate to verify every byte.`,
      });
    } catch {
      setStatus({
        kind: "error",
        message: `This file is not a valid file.cheap recovery card. Choose an exported filecheap.recovery-card.v1 JSON file.${recoveryCard ? " Previous valid recovery artifacts were retained." : ""}`,
      });
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
      setStatus({ kind: "working", message: `Downloading every byte of ${card.stashId}…` });
      const plan = await apiRequest<DownloadResponse>(
        "/api/v1/sync/downloads",
        apiToken.trim(),
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
      setRecoveryReport(null);
      setReportDownloadRequested(false);
      setStatus({
        kind: "success",
        message: "Downloaded bytes verified and offered for saving. Select the saved download below for a local content-equivalence check.",
      });
    } catch (error) {
      setStatus(
        errorStatus(
          error,
          undefined,
          Boolean(hydrationEvidence || recoveryReport),
        ),
      );
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
    if (selectedFile.size === 0) {
      setStatus({ kind: "error", message: "Choose a non-empty downloaded file." });
      return;
    }
    if (selectedFile.size > labFileLimitBytes) {
      setStatus({
        kind: "error",
        message: `The selected file exceeds the ${formatBytes(labFileLimitBytes)} lab limit.`,
      });
      return;
    }
    if (!beginOperation()) return;
    const card = recoveryCard;
    const evidence = hydrationEvidence;
    try {
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
      setReportDownloadRequested(false);
      setStatus({
        kind: "success",
        message: `${card.stashId}: the selected local file is byte-equivalent to the verified download.`,
      });
    } catch (error) {
      setStatus(errorStatus(error, undefined, Boolean(recoveryReport)));
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
    <div className="labPanel" aria-busy={isWorking}>
      <form
        className="labToolbar"
        onSubmit={(event) => {
          event.preventDefault();
          void connectVault();
        }}
      >
        <label>
          <span>Development bearer token</span>
          <input
            aria-describedby="lab-status"
            autoComplete="off"
            disabled={isWorking}
            onChange={(event) => {
              setApiToken(event.target.value);
              setConnected(false);
              setStashes([]);
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
        <button disabled={isWorking} type="submit">
          {connected ? "Reconnect" : "Unlock vault"}
        </button>
      </form>

      <div className="uploadRow">
        <label className="filePicker">
          <span>Archive · max {formatBytes(labFileLimitBytes)}</span>
          <input
            aria-describedby="lab-status"
            disabled={isWorking}
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
            aria-describedby={showStashIdError ? "stash-id-error lab-status" : "lab-status"}
            aria-invalid={showStashIdError}
            disabled={isWorking}
            maxLength={128}
            onChange={(event) => {
              setStashId(event.target.value);
              setStashIdTouched(true);
            }}
            placeholder="investigation-01"
            spellCheck={false}
            value={stashId}
          />
          {showStashIdError ? (
            <span className="fieldError" id="stash-id-error">
              Start with a letter or number; then use letters, numbers, dots, underscores, or hyphens.
            </span>
          ) : null}
        </label>
        <button className="primaryAction" disabled={!canPush} onClick={pushArchive} type="button">
          Push archive
        </button>
      </div>

      <div
        aria-atomic="true"
        aria-live={status.kind === "error" ? "assertive" : "polite"}
        className={`labStatus ${status.kind}`}
        id="lab-status"
        role={status.kind === "error" ? "alert" : "status"}
      >
        <span aria-hidden="true" />
        <div className="labStatusContent">
          <div className="labStatusMessage">{status.message}</div>
          {status.requestId ? (
            <details className="labStatusDetail">
              <summary>Technical details</summary>
              <code>Request ID: {status.requestId}</code>
            </details>
          ) : null}
        </div>
      </div>

      <section className="recoveryDrill" aria-labelledby="recovery-drill-title">
        <div className="vaultHeader">
          <div>
            <span>Portable recovery drill</span>
            <strong id="recovery-drill-title">Save → import → hydrate → compare → keep evidence</strong>
          </div>
        </div>
        <div className="recoveryCard">
          {recoveryCard ? (
            <div className="recoveryCardSummary">
              <strong>{recoveryCard.stashId}</strong>
              <span>{recoveryCard.originalFileName} · {formatBytes(recoveryCard.sizeBytes)}</span>
              <span>{shortHash(recoveryCard.sha256)}</span>
              <span>
                {cardOrigin === "imported"
                  ? "Imported card · ready for recovery"
                  : cardDownloadRequested
                    ? "Card download offered · import it to continue"
                    : "Generated card · request a download outside this session"}
              </span>
            </div>
          ) : (
            <div className="recoveryCardSummary emptyRecovery">
              <strong>No recovery card loaded</strong>
              <span>Push an archive to create one, or import an existing card to recover.</span>
            </div>
          )}

          <ol className="recoveryActions recoveryStepper" aria-label="Recovery drill steps">
            <li
              aria-current={cardStepStatus === "active" ? "step" : undefined}
              className={recoveryStepClass(cardStepStatus)}
            >
              <span className="recoveryStepIndex" aria-hidden="true">01</span>
              <div className="recoveryStepBody">
                <strong>Request the card download</strong>
                <span className="recoveryStepState">
                  {!recoveryCard
                    ? "Push an archive first"
                    : cardOrigin === "imported"
                      ? "Card re-imported"
                      : cardDownloadRequested
                        ? "Download offered"
                        : "Ready now"}
                </span>
              </div>
              <button
                className={recoveryActionClass(cardStepStatus)}
                disabled={!recoveryCard || isWorking}
                onClick={() => {
                  if (!recoveryCard) return;
                  downloadJson(
                    serializeRecoveryCard(recoveryCard),
                    `${recoveryCard.stashId}.recovery-card.json`,
                  );
                  setCardDownloadRequested(true);
                }}
                type="button"
              >Export card</button>
            </li>

            <li
              aria-current={importStepStatus === "active" ? "step" : undefined}
              className={recoveryStepClass(importStepStatus)}
            >
              <span className="recoveryStepIndex" aria-hidden="true">02</span>
              <div className="recoveryStepBody">
                <strong>Import the saved card</strong>
                <span className="recoveryStepState">{recoveryStepLabel(importStepStatus)}</span>
              </div>
              <label className={`${recoveryActionClass(importStepStatus)} compactPicker${isWorking ? " disabled" : ""}`}>
                Import card
                <input
                  accept="application/json,.json"
                  aria-describedby="lab-status"
                  disabled={isWorking}
                  onChange={(event) => {
                    const selected = event.currentTarget.files?.[0] ?? null;
                    event.currentTarget.value = "";
                    void importCard(selected);
                  }}
                  type="file"
                />
              </label>
            </li>

            <li
              aria-current={hydrateStepStatus === "active" ? "step" : undefined}
              className={recoveryStepClass(hydrateStepStatus)}
            >
              <span className="recoveryStepIndex" aria-hidden="true">03</span>
              <div className="recoveryStepBody">
                <strong>Hydrate and verify every byte</strong>
                <span className="recoveryStepState">
                  {cardOrigin === "imported" && !connected
                    ? "Unlock the vault first"
                    : recoveryStepLabel(hydrateStepStatus)}
                </span>
              </div>
              <button
                className={recoveryActionClass(hydrateStepStatus)}
                disabled={!connected || isWorking || cardOrigin !== "imported"}
                onClick={hydrateFromCard}
                type="button"
              >Hydrate + download</button>
            </li>

            <li
              aria-current={compareStepStatus === "active" ? "step" : undefined}
              className={recoveryStepClass(compareStepStatus)}
            >
              <span className="recoveryStepIndex" aria-hidden="true">04</span>
              <div className="recoveryStepBody">
                <strong>Compare the saved download</strong>
                <span className="recoveryStepState">{recoveryStepLabel(compareStepStatus)}</span>
              </div>
              <label className={`${recoveryActionClass(compareStepStatus)} compactPicker${canVerifyReopenedFile && !isWorking ? "" : " disabled"}`}>
                Select saved download
                <input
                  aria-describedby="lab-status"
                  disabled={!canVerifyReopenedFile || isWorking}
                  onChange={(event) => {
                    const selected = event.currentTarget.files?.[0] ?? null;
                    event.currentTarget.value = "";
                    void verifySelectedFile(selected);
                  }}
                  type="file"
                />
              </label>
            </li>

            <li
              aria-current={reportStepStatus === "active" ? "step" : undefined}
              className={recoveryStepClass(reportStepStatus)}
            >
              <span className="recoveryStepIndex" aria-hidden="true">05</span>
              <div className="recoveryStepBody">
                <strong>Download the local observation</strong>
                <span className="recoveryStepState">
                  {reportDownloadRequested
                    ? "Download offered"
                    : recoveryStepLabel(reportStepStatus)}
                </span>
              </div>
              <button
                className={recoveryActionClass(reportStepStatus)}
                disabled={!recoveryReport || isWorking}
                onClick={() => {
                  if (!recoveryReport) return;
                  downloadJson(
                    serializeRecoveryDrillReport(recoveryReport),
                    `${recoveryReport.stashId}.recovery-drill-report.json`,
                  );
                  setReportDownloadRequested(true);
                }}
                type="button"
              >Export report</button>
            </li>
          </ol>
        </div>
      </section>

      <div className="vaultHeader">
        <div>
          <span>Authenticated catalog</span>
          <strong>
            {connected
              ? `${stashes.length} ${stashes.length === 1 ? "stash" : "stashes"} · ${formatBytes(totalLogicalBytes)} logical bytes`
              : "Locked"}
          </strong>
        </div>
        <span className="tableHint">Full download + client SHA-256 proves recovery</span>
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
                <span className={`verificationBadge ${stash.storageVerification}`}>
                  Storage check: {verificationLabel(stash.storageVerification)}
                </span>
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

class ApiResponseError extends Error {
  constructor(
    message: string,
    readonly requestId?: string,
  ) {
    super(message);
    this.name = "ApiResponseError";
  }
}

async function responseError(response: Response): Promise<ApiResponseError> {
  const headerRequestId = response.headers.get("x-request-id") ?? undefined;
  try {
    const problem = (await response.json()) as {
      detail?: string;
      requestId?: string;
      title?: string;
    };
    return new ApiResponseError(
      problem.detail ?? problem.title ?? `Request failed (${response.status})`,
      problem.requestId ?? headerRequestId,
    );
  } catch {
    return new ApiResponseError(`Request failed (${response.status})`, headerRequestId);
  }
}

async function sha256Hex(bytes: ArrayBuffer): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", bytes);
  return Array.from(new Uint8Array(digest), (byte) => byte.toString(16).padStart(2, "0")).join("");
}

function downloadBytes(bytes: ArrayBuffer, fileName: string): void {
  const url = URL.createObjectURL(new Blob([bytes], { type: "application/octet-stream" }));
  clickDownload(url, fileName);
  setTimeout(() => URL.revokeObjectURL(url), objectUrlLifetimeMilliseconds);
}

function downloadJson(contents: string, fileName: string): void {
  const url = URL.createObjectURL(new Blob([contents], { type: "application/json" }));
  clickDownload(url, fileName);
  setTimeout(() => URL.revokeObjectURL(url), objectUrlLifetimeMilliseconds);
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

function errorStatus(
  error: unknown,
  prefix?: string,
  retainedEvidence = false,
): LabStatus {
  const message = `${messageFor(error)}${retainedEvidence ? " Previous verified artifacts were retained." : ""}`;
  return {
    kind: "error",
    message: prefix ? `${prefix}: ${message}` : message,
    ...(error instanceof ApiResponseError && error.requestId
      ? { requestId: error.requestId }
      : {}),
  };
}

function deriveStashId(fileName: string): string {
  const candidate = fileName
    .replace(/[^A-Za-z0-9._-]/g, "-")
    .replace(/^[^A-Za-z0-9]+/, "")
    .slice(0, 128);
  return candidate || "stash";
}

function recoveryStepClass(status: RecoveryStepStatus): string {
  return `recoveryStep ${status}`;
}

function recoveryActionClass(status: RecoveryStepStatus): string {
  return `recoveryStepAction ${
    status === "active" ? "recoveryStepActionPrimary" : "recoveryStepActionSecondary"
  }`;
}

function recoveryStepLabel(status: RecoveryStepStatus): string {
  switch (status) {
    case "active":
      return "Ready now";
    case "complete":
      return "Complete";
    case "pending":
      return "Waiting for the previous step";
  }
}

function verificationLabel(
  verification: StashSummary["storageVerification"],
): string {
  switch (verification) {
    case "server-sha256":
      return "Full SHA-256 verified before commit";
    case "presence-size-etag":
      return "Presence and size only; recovery hash required";
  }
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
