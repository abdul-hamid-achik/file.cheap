export type RecoveryAttemptState = {
  hasRetainedAttemptEvidence: boolean;
  hydrationIsCurrentAttempt: boolean;
  reportIsCurrentAttempt: boolean;
};

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
