import { describe, expect, test } from "bun:test";

import {
  deriveRecoveryAttemptState,
  shouldLockAfterConnectionFailure,
} from "@/features/sync/recovery-lab-state";

describe("Recovery Lab state transitions", () => {
  test("does not present proof from a failed newer recovery attempt as current", () => {
    expect(
      deriveRecoveryAttemptState({
        comparisonSucceeded: true,
        currentAttemptId: "attempt-new",
        hydrationAttemptId: "attempt-old",
        reportAttemptId: "attempt-old",
      }),
    ).toEqual({
      hasRetainedAttemptEvidence: true,
      hydrationIsCurrentAttempt: false,
      reportIsCurrentAttempt: false,
    });
  });

  test("marks a successful download current only after its comparison passes", () => {
    const downloaded = {
      comparisonSucceeded: false,
      currentAttemptId: "attempt-current",
      hydrationAttemptId: "attempt-current",
      reportAttemptId: "attempt-current",
    };

    expect(deriveRecoveryAttemptState(downloaded)).toEqual({
      hasRetainedAttemptEvidence: true,
      hydrationIsCurrentAttempt: true,
      reportIsCurrentAttempt: false,
    });
    expect(
      deriveRecoveryAttemptState({
        ...downloaded,
        comparisonSucceeded: true,
      }),
    ).toEqual({
      hasRetainedAttemptEvidence: false,
      hydrationIsCurrentAttempt: true,
      reportIsCurrentAttempt: true,
    });
  });

  test("locks only when the server rejects the bearer token", () => {
    expect(shouldLockAfterConnectionFailure(401)).toBe(true);
    expect(shouldLockAfterConnectionFailure(500)).toBe(false);
    expect(shouldLockAfterConnectionFailure()).toBe(false);
  });
});
