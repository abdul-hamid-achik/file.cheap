import type { StashSummary } from "@/features/sync/contracts";

export type RecoveryAttemptState = {
  hasRetainedAttemptEvidence: boolean;
  hydrationIsCurrentAttempt: boolean;
  reportIsCurrentAttempt: boolean;
};

export type ExpectedStashContent = Pick<
  StashSummary,
  "contentType" | "sha256" | "sizeBytes" | "stashId"
>;

export type CommitReconciliation =
  | { kind: "committed"; stash: StashSummary }
  | { kind: "conflict" }
  | { kind: "not_found" };

export type PendingCommitResolution<T> =
  | { kind: "conflict"; pending: null }
  | { kind: "recovered"; pending: null; stash: StashSummary }
  | { kind: "unresolved"; pending: T };

export function deriveRecoveryAttemptState(input: {
  comparisonSucceeded: boolean;
  currentAttemptId: string | null;
  hydrationAttemptId?: string;
  reportAttemptId?: string;
}): RecoveryAttemptState {
  const hydrationIsCurrentAttempt = Boolean(
    input.currentAttemptId &&
      input.hydrationAttemptId === input.currentAttemptId,
  );
  const reportMatchesCurrentAttempt = Boolean(
    input.currentAttemptId && input.reportAttemptId === input.currentAttemptId,
  );
  const reportIsCurrentAttempt = Boolean(
    reportMatchesCurrentAttempt && input.comparisonSucceeded,
  );

  return {
    hasRetainedAttemptEvidence: Boolean(
      (input.hydrationAttemptId && !hydrationIsCurrentAttempt) ||
        (input.reportAttemptId && !reportIsCurrentAttempt),
    ),
    hydrationIsCurrentAttempt,
    reportIsCurrentAttempt,
  };
}

export function shouldLockAfterConnectionFailure(status?: number): boolean {
  return status === 401;
}

export function reconcileCommitFromCatalog(
  stashes: readonly StashSummary[],
  expected: ExpectedStashContent,
): CommitReconciliation {
  const matches = stashes.filter(
    (candidate) => candidate.stashId === expected.stashId,
  );
  if (matches.length === 0) return { kind: "not_found" };
  if (matches.length !== 1) return { kind: "conflict" };
  return sameStashContent(matches[0], expected)
    ? { kind: "committed", stash: matches[0] }
    : { kind: "conflict" };
}

export function resolvePendingCommitAfterReconnect<
  T extends { expected: ExpectedStashContent },
>(
  pending: T,
  stashes: readonly StashSummary[],
): PendingCommitResolution<T> {
  const reconciliation = reconcileCommitFromCatalog(stashes, pending.expected);
  if (reconciliation.kind === "committed") {
    return {
      kind: "recovered",
      pending: null,
      stash: reconciliation.stash,
    };
  }
  if (reconciliation.kind === "conflict") {
    return { kind: "conflict", pending: null };
  }
  return { kind: "unresolved", pending };
}

export function sameStashContent(
  stash: StashSummary,
  expected: ExpectedStashContent,
): boolean {
  return (
    stash.contentType === expected.contentType &&
    stash.sha256 === expected.sha256 &&
    stash.sizeBytes === expected.sizeBytes &&
    stash.stashId === expected.stashId
  );
}
