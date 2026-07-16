"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { z, type ZodType } from "zod";

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
import { readExactResponseBytes } from "@/features/sync/bounded-response";
import {
  commitPlanResponseSchema,
  downloadPlanSchema,
  protocolV1MaxObjectBytes,
  stashListSchema,
  stashContentType,
  stashIdSchema,
  syncPlanSchema,
  type StashSummary,
} from "@/features/sync/contracts";
import {
  OperationCanceledError,
  throwIfOperationCanceled,
  withRequestDeadline,
} from "@/features/sync/request-lifecycle";
import {
  MutationOutcomeUnknownError,
  parseSuccessResponse,
  readBoundedJsonResponse,
  ResponseContractError,
} from "@/features/sync/response-contract";
import {
  deriveRecoveryAttemptState,
  reconcileCommitFromCatalog,
  resolvePendingCommitAfterReconnect,
  sameStashContent,
  shouldLockAfterConnectionFailure,
  type ExpectedStashContent,
} from "@/features/sync/recovery-lab-state";

const labFileLimitBytes = protocolV1MaxObjectBytes;
const recoveryCardFileLimitBytes = 64 * 1024;
const objectUrlLifetimeMilliseconds = 60_000;
const controlPlaneTimeoutMilliseconds = 30_000;
const transferTimeoutMilliseconds = 5 * 60_000;

const problemDetailsSchema = z
  .object({
    code: z.string().max(256).optional(),
    detail: z.string().max(8_192).optional(),
    requestId: z.string().max(256).optional(),
    title: z.string().max(512).optional(),
  })
  .passthrough();

type LabStatus = {
  kind: "error" | "idle" | "success" | "working";
  message: string;
  requestId?: string;
};

type RecoveryStepStatus = "active" | "complete" | "pending";

type HydrationEvidence = {
  attemptId: string;
  cardIdentity: string;
  downloadedSha256: string;
  startedAt: string;
};

type PendingCommit = {
  expected: ExpectedStashContent;
  file: File;
  requestId?: string;
  retryAllowed: boolean;
};

type RecoveryLabProps = {
  storageDriver: "local" | "vercel-blob";
};

export function RecoveryLab({ storageDriver }: RecoveryLabProps) {
  const operationInFlight = useRef(false);
  const operationController = useRef<AbortController | null>(null);
  const operationReturnFocus = useRef<HTMLElement | null>(null);
  const restoreFocusAfterCancel = useRef(false);
  const tokenInput = useRef<HTMLInputElement | null>(null);
  const [apiToken, setApiToken] = useState("");
  const [cancelRequested, setCancelRequested] = useState(false);
  const [connected, setConnected] = useState(false);
  const [file, setFile] = useState<File | null>(null);
  const [stashId, setStashId] = useState("");
  const [stashIdTouched, setStashIdTouched] = useState(false);
  const [stashes, setStashes] = useState<StashSummary[]>([]);
  const [recoveryCard, setRecoveryCard] = useState<RecoveryCard | null>(null);
  const [cardSourceFile, setCardSourceFile] = useState<File | null>(null);
  const [cardOrigin, setCardOrigin] = useState<"generated" | "imported" | null>(null);
  const [cardDownloadRequested, setCardDownloadRequested] = useState(false);
  const [hydrationEvidence, setHydrationEvidence] = useState<HydrationEvidence | null>(null);
  const [recoveryReport, setRecoveryReport] = useState<RecoveryDrillReport | null>(null);
  const [reportDownloadRequested, setReportDownloadRequested] = useState(false);
  const [currentRecoveryAttemptId, setCurrentRecoveryAttemptId] = useState<string | null>(null);
  const [currentComparisonSucceeded, setCurrentComparisonSucceeded] = useState(false);
  const [pendingCommit, setPendingCommit] = useState<PendingCommit | null>(null);
  const [status, setStatus] = useState<LabStatus>({
    kind: "idle",
    message: "Enter the local bearer token to unlock this simulated vault.",
  });

  const isWorking = status.kind === "working";
  const adapterLabel =
    storageDriver === "local"
      ? "Local filesystem adapter"
      : "Vercel Blob experiment";
  const adapterVerification =
    storageDriver === "local"
      ? "Full server SHA-256 before commit"
      : "Presence + size + ETag; recovery hash still required";
  const parsedStashId = stashIdSchema.safeParse(stashId.trim());
  const showStashIdError = stashIdTouched && !parsedStashId.success;
  const canPush = Boolean(
    connected &&
      file &&
      parsedStashId.success &&
      !isWorking &&
      (!pendingCommit ||
        (pendingCommit.retryAllowed &&
          file === pendingCommit.file &&
          parsedStashId.data === pendingCommit.expected.stashId)),
  );
  const currentCardIdentity = recoveryCard
    ? recoveryCardIdentity(recoveryCard)
    : null;
  const {
    hasRetainedAttemptEvidence,
    hydrationIsCurrentAttempt,
    reportIsCurrentAttempt,
  } = deriveRecoveryAttemptState({
    comparisonSucceeded: currentComparisonSucceeded,
    currentAttemptId: currentRecoveryAttemptId,
    hydrationAttemptId: hydrationEvidence?.attemptId,
    reportAttemptId: recoveryReport?.attemptId,
  });
  const evidenceBelongsToPreviousDrill = Boolean(
    recoveryCard &&
      file &&
      (file !== cardSourceFile || recoveryCard.stashId !== stashId.trim()),
  );
  const hasRetainedEvidence =
    evidenceBelongsToPreviousDrill || hasRetainedAttemptEvidence;
  const canVerifyReopenedFile = Boolean(
    recoveryCard &&
      hydrationEvidence &&
      hydrationIsCurrentAttempt &&
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
  const compareStepStatus: RecoveryStepStatus = reportIsCurrentAttempt
    ? "complete"
    : canVerifyReopenedFile
      ? "active"
      : "pending";
  const reportStepStatus: RecoveryStepStatus =
    reportIsCurrentAttempt && reportDownloadRequested
      ? "complete"
      : reportIsCurrentAttempt
        ? "active"
        : "pending";

  useEffect(() => {
    return () => {
      operationController.current?.abort(new OperationCanceledError());
    };
  }, []);

  function applyAuthenticationFailure(error: unknown): void {
    if (
      shouldLockAfterConnectionFailure(
        error instanceof ApiResponseError && error.origin === "control"
          ? error.status
          : undefined,
      )
    ) {
      setConnected(false);
      setStashes([]);
    }
  }

  async function connectVault() {
    if (!apiToken.trim()) {
      setStatus({ kind: "error", message: "Enter the development bearer token." });
      return;
    }
    const signal = beginOperation();
    if (!signal) return;
    try {
      setStatus({ kind: "working", message: "Authenticating and reading the vault…" });
      const catalog = await listStashes(apiToken.trim(), signal);
      setStashes(catalog);
      setConnected(true);
      if (pendingCommit) {
        const resolution = resolvePendingCommitAfterReconnect(
          pendingCommit,
          catalog,
        );
        if (resolution.kind === "recovered") {
          installCommittedRecovery(
            resolution.stash,
            pendingCommit.file,
            pendingCommit.expected,
          );
          setPendingCommit(null);
          setStatus({
            kind: "success",
            message: `${resolution.stash.stashId} was found after reconnecting. Its recovery card is ready; no second upload was attempted.`,
            ...(pendingCommit.requestId
              ? { requestId: pendingCommit.requestId }
              : {}),
          });
          return;
        }
        if (resolution.kind === "conflict") {
          setPendingCommit(null);
          setStatus({
            kind: "error",
            message:
              "Reconnect found this stash ID bound to different content. Do not retry the commit.",
            ...(pendingCommit.requestId
              ? { requestId: pendingCommit.requestId }
              : {}),
          });
          return;
        }
        setStatus({
          kind: "error",
          message:
            "Vault unlocked, but the dispatched commit is not in the catalog. Rerun Upload + commit with the unchanged file and stash ID; the immutable plan will reuse any bytes already present.",
          ...(pendingCommit.requestId
            ? { requestId: pendingCommit.requestId }
            : {}),
        });
        setFile(pendingCommit.file);
        setStashId(pendingCommit.expected.stashId);
        setStashIdTouched(false);
        setPendingCommit({ ...pendingCommit, retryAllowed: true });
        return;
      }
      setStatus({
        kind: "success",
        message: "Vault unlocked for this tab. The token is not persisted.",
      });
    } catch (error) {
      applyAuthenticationFailure(error);
      setStatus(errorStatus(error));
    } finally {
      endOperation();
    }
  }

  function installCommittedRecovery(
    committedStash: StashSummary,
    sourceFile: File,
    expected: ExpectedStashContent,
  ): void {
    const nextRecoveryCard = createRecoveryCard({
      committedAt: committedStash.committedAt,
      originalFileName: sourceFile.name,
      sha256: expected.sha256,
      sizeBytes: expected.sizeBytes,
      stashId: committedStash.stashId,
    });
    setRecoveryCard(nextRecoveryCard);
    setCardSourceFile(sourceFile);
    setCardOrigin("generated");
    setCardDownloadRequested(false);
    setHydrationEvidence(null);
    setRecoveryReport(null);
    setReportDownloadRequested(false);
    setCurrentRecoveryAttemptId(null);
    setCurrentComparisonSucceeded(false);
  }

  function chooseArchive(nextFile: File | null) {
    if (operationInFlight.current || pendingCommit) return;
    if (!nextFile) return;
    if (nextFile.size === 0) {
      rejectArchiveSelection("Choose a non-empty archive.");
      return;
    }
    if (nextFile.size > labFileLimitBytes) {
      rejectArchiveSelection(
        `The browser lab is capped at ${formatBytes(labFileLimitBytes)}. Large archives require the future multipart Go-client protocol.`,
      );
      return;
    }

    const derivedStashId = deriveStashId(nextFile.name);
    const validatedStashId = stashIdSchema.safeParse(derivedStashId);
    if (!validatedStashId.success) {
      rejectArchiveSelection(
        "Could not derive a valid stash ID from this filename. Choose another archive.",
      );
      return;
    }
    setStashId(validatedStashId.data);
    setStashIdTouched(false);

    setFile(nextFile);
    const retainsPreviousEvidence = Boolean(
      recoveryCard &&
        (nextFile !== cardSourceFile ||
          recoveryCard.stashId !== validatedStashId.data),
    );
    setStatus({
      kind: "idle",
      message: retainsPreviousEvidence
        ? `New drill ready for ${validatedStashId.data}. Verified evidence for ${recoveryCard?.stashId} remains available until a new archive commits.`
        : "Archive selected. No local file will be deleted.",
    });
  }

  function rejectArchiveSelection(message: string): void {
    setFile(null);
    setStashId("");
    setStashIdTouched(false);
    setStatus({
      kind: "error",
      message: `${message} The previous archive selection was cleared.`,
    });
  }

  async function pushArchive() {
    if (!file) return;
    const selectedFile = file;
    const validatedStashId = stashIdSchema.safeParse(stashId.trim());
    if (!validatedStashId.success) {
      setStatus({
        kind: "error",
        message: "Enter a valid stash ID using letters, numbers, dots, underscores, or hyphens.",
      });
      return;
    }
    const signal = beginOperation();
    if (!signal) return;

    try {
      setStatus({ kind: "working", message: "Hashing the exact archive bytes…" });
      const bytes = await selectedFile.arrayBuffer();
      throwIfOperationCanceled(signal);
      const sha256 = await sha256Hex(bytes);
      throwIfOperationCanceled(signal);
      const contentType = stashContentType;
      const expectedContent: ExpectedStashContent = {
        contentType,
        sha256,
        sizeBytes: selectedFile.size,
        stashId: validatedStashId.data,
      };

      setStatus({ kind: "working", message: "Requesting an immutable upload plan…" });
      const plan = await apiRequest(
        "/api/v1/sync/plans",
        apiToken.trim(),
        {
          contentType,
          sha256,
          sizeBytes: selectedFile.size,
          stashId: validatedStashId.data,
        },
        signal,
        syncPlanSchema.superRefine((candidate, context) => {
          if (
            candidate.object.sha256 !== sha256 ||
            candidate.object.sizeBytes !== selectedFile.size
          ) {
            context.addIssue({
              code: "custom",
              message: "plan object must match the selected archive",
              path: ["object"],
            });
          }
        }),
        { operation: "Upload plan" },
      );

      if (plan.upload) {
        setStatus({ kind: "working", message: "Transferring through the signed data path…" });
        await withRequestDeadline({
          label: "Archive upload",
          operation: async (requestSignal) => {
            const uploadResponse = await fetch(plan.upload!.url, {
              body: selectedFile,
              headers: plan.upload!.headers,
              method: plan.upload!.method,
              signal: requestSignal,
            });
            if (!uploadResponse.ok) {
              throw await responseError(uploadResponse, "transfer");
            }
          },
          signal,
          timeoutMilliseconds: transferTimeoutMilliseconds,
        });
      }

      setStatus({ kind: "working", message: "Committing the catalog reference…" });
      throwIfOperationCanceled(signal);
      let committedStash: StashSummary;
      let reconciledCatalog: StashSummary[] | undefined;
      let reconciledCommitRequestId: string | undefined;
      try {
        const committed = await apiRequest(
          "/api/v1/sync/commits",
          apiToken.trim(),
          { receipt: plan.receipt },
          signal,
          commitPlanResponseSchema.superRefine((candidate, context) => {
            if (
              !sameStashContent(candidate.stash, expectedContent)
            ) {
              context.addIssue({
                code: "custom",
                message: "committed stash must match the selected archive",
                path: ["stash"],
              });
            }
          }),
          { acknowledgedMutation: true, operation: "Commit" },
        );
        committedStash = committed.stash;
      } catch (error) {
        if (isDefinitiveCommitRejection(error)) throw error;
        const pending: PendingCommit = {
          expected: expectedContent,
          file: selectedFile,
          retryAllowed: false,
          ...(requestIdForClientError(error)
            ? { requestId: requestIdForClientError(error) }
            : {}),
        };
        if (signal.aborted) {
          setPendingCommit(pending);
          throw new MutationOutcomeUnknownError(requestIdForClientError(error));
        }

        try {
          reconciledCatalog = await listStashes(apiToken.trim(), signal);
        } catch (reconciliationError) {
          applyAuthenticationFailure(reconciliationError);
          setPendingCommit(pending);
          throw new MutationOutcomeUnknownError(requestIdForClientError(error));
        }
        const reconciliation = reconcileCommitFromCatalog(
          reconciledCatalog,
          expectedContent,
        );
        if (reconciliation.kind === "conflict") {
          setPendingCommit(null);
          throw new Error(
            "Catalog reconciliation found this stash ID bound to different content. Do not retry the commit.",
          );
        }
        if (reconciliation.kind === "not_found") {
          setPendingCommit(pending);
          throw new MutationOutcomeUnknownError(requestIdForClientError(error));
        }
        committedStash = reconciliation.stash;
        reconciledCommitRequestId = requestIdForClientError(error);
      }
      setPendingCommit(null);
      installCommittedRecovery(committedStash, selectedFile, expectedContent);

      let refreshError: unknown;
      if (reconciledCatalog) {
        setStashes(reconciledCatalog);
      } else {
        try {
          setStashes(await listStashes(apiToken.trim(), signal));
        } catch (error) {
          refreshError = error;
          applyAuthenticationFailure(error);
        }
      }

      setStatus({
        kind: "success",
        message: reconciledCatalog
          ? `${committedStash.stashId} was found in the catalog after an interrupted or invalid commit response. Its recovery card is ready; no second upload was attempted.`
          : refreshError
            ? `${committedStash.stashId} committed and its recovery card is ready. The catalog refresh failed; reconnect to refresh the list.`
            : `${committedStash.stashId} committed. Export and re-import its card; use a clean profile for the strongest disaster drill.`,
        ...(reconciledCommitRequestId
          ? { requestId: reconciledCommitRequestId }
          : refreshError instanceof ApiResponseError && refreshError.requestId
            ? { requestId: refreshError.requestId }
            : {}),
      });
    } catch (error) {
      applyAuthenticationFailure(error);
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
    const signal = beginOperation();
    if (!signal) return;
    try {
      setStatus({ kind: "working", message: "Validating the portable recovery card…" });
      const cardContents = await cardFile.text();
      throwIfOperationCanceled(signal);
      const card = parseRecoveryCard(cardContents);
      setRecoveryCard(card);
      setCardSourceFile(null);
      setCardOrigin("imported");
      setCardDownloadRequested(true);
      setRecoveryReport(null);
      setHydrationEvidence(null);
      setReportDownloadRequested(false);
      setCurrentRecoveryAttemptId(null);
      setCurrentComparisonSucceeded(false);
      setFile(null);
      setStashId("");
      setStashIdTouched(false);
      setStatus({
        kind: "success",
        message: `Recovery card loaded for ${card.stashId}. Recover to verify every byte.`,
      });
    } catch (error) {
      setStatus(
        error instanceof OperationCanceledError
          ? errorStatus(error, undefined, Boolean(recoveryCard))
          : {
              kind: "error",
              message: `This file is not a valid file.cheap recovery card. Choose an exported filecheap.recovery-card.v1 JSON file.${recoveryCard ? " Previous valid recovery artifacts were retained." : ""}`,
            },
      );
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
    const signal = beginOperation();
    if (!signal) return;

    const card = recoveryCard;
    const startedAt = new Date().toISOString();
    const attemptId = crypto.randomUUID();
    setCurrentRecoveryAttemptId(attemptId);
    setCurrentComparisonSucceeded(false);
    setReportDownloadRequested(false);
    try {
      setStatus({ kind: "working", message: `Downloading every byte of ${card.stashId}…` });
      const plan = await apiRequest(
        "/api/v1/sync/downloads",
        apiToken.trim(),
        { stashId: card.stashId },
        signal,
        downloadPlanSchema.superRefine((candidate, context) => {
          if (
            candidate.stashId !== card.stashId ||
            candidate.expected.sizeBytes !== card.sizeBytes ||
            candidate.expected.sha256 !== card.sha256
          ) {
            context.addIssue({
              code: "custom",
              message: "download plan must match the recovery card",
            });
          }
        }),
        { operation: "Download plan" },
      );

      const bytes = await withRequestDeadline({
        label: "Recovery download",
        operation: async (requestSignal) => {
          const response = await fetch(plan.grant.url, {
            headers: plan.grant.headers,
            method: plan.grant.method,
            signal: requestSignal,
          });
          if (!response.ok) throw await responseError(response, "transfer");
          return readExactResponseBytes(response, card.sizeBytes);
        },
        signal,
        timeoutMilliseconds: transferTimeoutMilliseconds,
      });
      throwIfOperationCanceled(signal);
      const sha256 = await sha256Hex(bytes);
      throwIfOperationCanceled(signal);
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
      applyAuthenticationFailure(error);
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
      !hydrationIsCurrentAttempt ||
      hydrationEvidence.cardIdentity !== recoveryCardIdentity(recoveryCard) ||
      hydrationEvidence.downloadedSha256 !== recoveryCard.sha256
    ) return;
    setCurrentComparisonSucceeded(false);
    setReportDownloadRequested(false);
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
    const signal = beginOperation();
    if (!signal) return;
    const card = recoveryCard;
    const evidence = hydrationEvidence;
    try {
      setStatus({ kind: "working", message: "Hashing the file selected from disk…" });
      const bytes = await selectedFile.arrayBuffer();
      throwIfOperationCanceled(signal);
      const sha256 = await sha256Hex(bytes);
      throwIfOperationCanceled(signal);
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
      setCurrentComparisonSucceeded(true);
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

  function beginOperation(): AbortSignal | null {
    if (operationInFlight.current) return null;
    operationInFlight.current = true;
    operationController.current = new AbortController();
    operationReturnFocus.current =
      document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null;
    restoreFocusAfterCancel.current = false;
    setCancelRequested(false);
    return operationController.current.signal;
  }

  function endOperation(): void {
    operationInFlight.current = false;
    operationController.current = null;
    setCancelRequested(false);
    const returnFocus = operationReturnFocus.current;
    operationReturnFocus.current = null;
    if (restoreFocusAfterCancel.current && returnFocus) {
      requestAnimationFrame(() => {
        if (document.activeElement === document.body) returnFocus.focus();
      });
    }
    restoreFocusAfterCancel.current = false;
  }

  function cancelOperation(): void {
    const controller = operationController.current;
    if (!controller || controller.signal.aborted) return;
    controller.abort(new OperationCanceledError());
    restoreFocusAfterCancel.current = true;
    setCancelRequested(true);
    setStatus({ kind: "working", message: "Canceling the current operation…" });
  }

  function lockVault(): void {
    if (operationInFlight.current) return;
    setApiToken("");
    setConnected(false);
    setStashes([]);
    setStatus({
      kind: "idle",
      message: "Vault locked. The bearer token was cleared from this tab.",
    });
    requestAnimationFrame(() => tokenInput.current?.focus());
  }

  return (
    <div aria-busy={isWorking} className="labPanel">
      <form
        className="labToolbar"
        onSubmit={(event) => {
          event.preventDefault();
          void connectVault();
        }}
      >
        <label>
          <span className="labTokenLabel">
            <span>Development bearer token</span>
            <small id="lab-token-help">
              Use PLATFORM_API_TOKEN from platform/.env.local
            </small>
          </span>
          <input
            aria-describedby="lab-token-help lab-status"
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
            ref={tokenInput}
            spellCheck={false}
            type="password"
            value={apiToken}
          />
        </label>
        <div className="labToolbarActions">
          <button disabled={isWorking} type="submit">
            {connected ? "Reconnect" : "Unlock vault"}
          </button>
          {connected ? (
            <button disabled={isWorking} onClick={lockVault} type="button">
              Lock + clear token
            </button>
          ) : null}
        </div>
      </form>

      <aside className="labSafetyNotice" aria-labelledby="lab-safety-title">
        <span className="labSafetyMark" aria-hidden="true">!</span>
        <div className="labSafetyCopy">
          <strong id="lab-safety-title">Synthetic, non-sensitive test data only</strong>
          <p>
            This laboratory does not encrypt archives client-side. Never use secrets,
            customer files, or the only copy of anything.
          </p>
        </div>
        <dl className="labAdapterFacts">
          <div><dt>Active adapter</dt><dd>{adapterLabel}</dd></div>
          <div><dt>Storage evidence</dt><dd>{adapterVerification}</dd></div>
        </dl>
      </aside>

      <nav className="labJourneyNav" aria-label="Choose a recovery lab path">
        <a href="#backup-test-archive">
          <span>Back up</span>
          <strong>Back up a test archive</strong>
          <small>Create an immutable object and portable card</small>
        </a>
        <a href="#recover-from-card">
          <span>Recover</span>
          <strong>Recover from a card</strong>
          <small>Download, hash, and compare a saved copy</small>
        </a>
      </nav>

      <div
        aria-label="Back up a test archive"
        className="uploadRow"
        id="backup-test-archive"
        role="region"
      >
        <label className="filePicker">
          <span>Archive · max {formatBytes(labFileLimitBytes)}</span>
          <input
            aria-describedby="lab-status"
            disabled={isWorking || Boolean(pendingCommit)}
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
            disabled={isWorking || Boolean(pendingCommit)}
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
          <small className="fieldHint" id="stash-immutability-note">
            Protocol v1 permanently binds this ID to these exact bytes; it cannot
            be rebound or deleted from this lab.
          </small>
        </label>
        <button
          aria-describedby="stash-immutability-note"
          className="primaryAction"
          disabled={!canPush}
          onClick={pushArchive}
          type="button"
        >
          Upload + commit
        </button>
      </div>

      <div className={`labStatus ${status.kind}`}>
        <span aria-hidden="true" />
        <div
          aria-atomic="true"
          aria-live={status.kind === "error" ? "assertive" : "polite"}
          className="labStatusContent"
          id="lab-status"
          role={status.kind === "error" ? "alert" : "status"}
        >
          <div className="labStatusMessage">{status.message}</div>
          {status.requestId ? (
            <details className="labStatusDetail">
              <summary>Technical details</summary>
              <code>Request ID: {status.requestId}</code>
            </details>
          ) : null}
        </div>
        {isWorking ? (
          <button
            className="cancelOperation"
            disabled={cancelRequested}
            onClick={cancelOperation}
            type="button"
          >
            Cancel
          </button>
        ) : null}
      </div>

      <section
        className="recoveryDrill"
        id="recover-from-card"
        aria-labelledby="recovery-drill-title"
      >
        <div className="vaultHeader">
          <div>
            <span>Portable recovery drill</span>
            <h3 id="recovery-drill-title">
              Export → re-import → recover → compare → export evidence
            </h3>
          </div>
        </div>
        <div className="recoveryCard">
          {recoveryCard ? (
            <div className="recoveryCardSummary">
              {hasRetainedEvidence ? (
                <span className="retainedEvidenceBadge">
                  Retained evidence · not the current attempt
                </span>
              ) : null}
              <strong>{recoveryCard.stashId}</strong>
              <span>{recoveryCard.originalFileName} · {formatBytes(recoveryCard.sizeBytes)}</span>
              <span>{shortHash(recoveryCard.sha256)}</span>
              <span>
                {evidenceBelongsToPreviousDrill
                  ? `This card belongs to ${recoveryCard.stashId}; the selected archive is a new drill.`
                  : hasRetainedAttemptEvidence
                    ? "A newer attempt did not complete. Earlier artifacts remain exportable, but do not count as current proof."
                  : cardOrigin === "imported"
                    ? "Imported unsigned metadata · verifies expected bytes, not who created the card"
                    : cardDownloadRequested
                      ? "Card download offered · re-import it to continue"
                      : "Generated card · export it, then re-import it to continue"}
              </span>
              <a className="startNewDrillLink" href="#backup-test-archive">
                Start a new drill ↑
              </a>
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
                <strong>Export the recovery card</strong>
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
                  setStatus({
                    kind: "success",
                    message: "Recovery card download offered. Re-import that saved JSON file to continue the drill.",
                  });
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
                <strong>Re-import the recovery card</strong>
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
                <strong>Recover and verify every byte</strong>
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
              >Recover + verify</button>
            </li>

            <li
              aria-current={compareStepStatus === "active" ? "step" : undefined}
              className={recoveryStepClass(compareStepStatus)}
            >
              <span className="recoveryStepIndex" aria-hidden="true">04</span>
              <div className="recoveryStepBody">
                <strong>Compare the saved recovery</strong>
                <span className="recoveryStepState">{recoveryStepLabel(compareStepStatus)}</span>
              </div>
              <label className={`${recoveryActionClass(compareStepStatus)} compactPicker${canVerifyReopenedFile && !isWorking ? "" : " disabled"}`}>
                Select saved file
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
                <strong>Export the recovery report</strong>
                <span className="recoveryStepState">
                  {!reportIsCurrentAttempt && recoveryReport
                    ? "Previous attempt retained"
                    : reportDownloadRequested
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
                  setStatus({
                    kind: "success",
                    message: "Recovery report download offered. It is an unsigned local observation, not proof of origin.",
                  });
                }}
                type="button"
              >
                {!recoveryReport || reportIsCurrentAttempt
                  ? "Export report"
                  : "Export retained report"}
              </button>
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
                  Commit-time evidence: {verificationLabel(stash.storageVerification)}
                </span>
              </div>
            </article>
          ))}
        </div>
      )}
    </div>
  );
}

async function apiRequest<T>(
  path: string,
  token: string,
  body: unknown,
  signal: AbortSignal,
  schema: ZodType<T>,
  options: {
    acknowledgedMutation?: boolean;
    operation: string;
  },
): Promise<T> {
  return withRequestDeadline({
    label: "Control-plane request",
    operation: async (requestSignal) => {
      const response = await fetch(path, {
        body: JSON.stringify(body),
        headers: {
          authorization: `Bearer ${token}`,
          "content-type": "application/json",
        },
        method: "POST",
        signal: requestSignal,
      });
      if (!response.ok) throw await responseError(response, "control");
      return parseSuccessResponse(response, schema, options);
    },
    signal,
    timeoutMilliseconds: controlPlaneTimeoutMilliseconds,
  });
}

async function listStashes(
  token: string,
  signal: AbortSignal,
): Promise<StashSummary[]> {
  return withRequestDeadline({
    label: "Catalog request",
    operation: async (requestSignal) => {
      const response = await fetch("/api/v1/stashes", {
        headers: { authorization: `Bearer ${token}` },
        signal: requestSignal,
      });
      if (!response.ok) throw await responseError(response, "control");
      return (
        await parseSuccessResponse(response, stashListSchema, {
          operation: "Catalog request",
        })
      ).stashes;
    },
    signal,
    timeoutMilliseconds: controlPlaneTimeoutMilliseconds,
  });
}

class ApiResponseError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly origin: "control" | "transfer",
    readonly code?: string,
    readonly requestId?: string,
  ) {
    super(message);
    this.name = "ApiResponseError";
  }
}

export async function responseError(
  response: Response,
  origin: "control" | "transfer",
): Promise<ApiResponseError> {
  const headerRequestId = response.headers.get("x-request-id") ?? undefined;
  try {
    const parsed = problemDetailsSchema.safeParse(
      await readBoundedJsonResponse(response, 64 * 1024),
    );
    if (!parsed.success) throw new Error("Invalid problem response");
    const problem = parsed.data;
    return new ApiResponseError(
      problem.detail ?? problem.title ?? `Request failed (${response.status})`,
      response.status,
      origin,
      problem.code,
      problem.requestId ?? headerRequestId,
    );
  } catch {
    return new ApiResponseError(
      `Request failed (${response.status})`,
      response.status,
      origin,
      undefined,
      headerRequestId,
    );
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
    ...(requestIdForClientError(error)
      ? { requestId: requestIdForClientError(error) }
      : {}),
  };
}

function requestIdForClientError(error: unknown): string | undefined {
  if (
    error instanceof ApiResponseError ||
    error instanceof ResponseContractError ||
    error instanceof MutationOutcomeUnknownError
  ) {
    return error.requestId;
  }
  return undefined;
}

function isDefinitiveCommitRejection(error: unknown): boolean {
  return (
    error instanceof ApiResponseError &&
    [400, 401, 403, 409, 410, 413, 415, 422].includes(error.status)
  );
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
      return "Presence, size, and committed ETag; recovery hash required";
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
