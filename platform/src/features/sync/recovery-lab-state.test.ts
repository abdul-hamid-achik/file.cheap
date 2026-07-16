import { describe, expect, test } from "bun:test";

import {
  deriveRecoveryAttemptState,
  reconcileCommitFromCatalog,
  resolvePendingCommitAfterReconnect,
  shouldLockAfterConnectionFailure,
} from "@/features/sync/recovery-lab-state";
import { stashContentType, type StashSummary } from "@/features/sync/contracts";

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

  test("reconciles an ambiguous commit only from an exact catalog identity", () => {
    const committed = stash("archive-01", "a".repeat(64));
    const expected = {
      contentType: stashContentType,
      sha256: committed.sha256,
      sizeBytes: committed.sizeBytes,
      stashId: committed.stashId,
    };

    expect(reconcileCommitFromCatalog([committed], expected)).toEqual({
      kind: "committed",
      stash: committed,
    });
    expect(reconcileCommitFromCatalog([], expected)).toEqual({
      kind: "not_found",
    });
    expect(
      reconcileCommitFromCatalog(
        [{ ...committed, sha256: "b".repeat(64) }],
        expected,
      ),
    ).toEqual({ kind: "conflict" });

    const conflicting = { ...committed, sha256: "b".repeat(64) };
    for (const duplicateOrder of [
      [committed, conflicting],
      [conflicting, committed],
      [committed, committed],
    ]) {
      expect(reconcileCommitFromCatalog(duplicateOrder, expected)).toEqual({
        kind: "conflict",
      });
    }
  });

  test("retains a canceled dispatched commit until reconnect resolves it", () => {
    const committed = stash("archive-01", "a".repeat(64));
    const pending = {
      expected: {
        contentType: stashContentType,
        sha256: committed.sha256,
        sizeBytes: committed.sizeBytes,
        stashId: committed.stashId,
      },
      requestId: "commit-request-01",
    };

    expect(resolvePendingCommitAfterReconnect(pending, [])).toEqual({
      kind: "unresolved",
      pending,
    });
    expect(resolvePendingCommitAfterReconnect(pending, [committed])).toEqual({
      kind: "recovered",
      pending: null,
      stash: committed,
    });
    expect(
      resolvePendingCommitAfterReconnect(pending, [
        { ...committed, sha256: "b".repeat(64) },
      ]),
    ).toEqual({ kind: "conflict", pending: null });
  });
});

function stash(stashId: string, sha256: string): StashSummary {
  return {
    committedAt: "2026-07-15T22:15:00.000Z",
    contentType: stashContentType,
    sha256,
    sizeBytes: 128,
    stashId,
    storageVerification: "server-sha256",
  };
}
