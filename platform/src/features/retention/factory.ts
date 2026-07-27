import { getArtifactService } from "@/features/artifacts/factory";
import type { RetentionStage } from "@/features/retention/repository";
import { RetentionRunService } from "@/features/retention/service";
import { getDatabase } from "@/platform/database/client";
import {
  cleanupConsoleAuthorizations,
  cleanupConsoleDeviceFamilies,
  cleanupConsoleRateLimits,
  cleanupConsoleSessions,
} from "@/platform/database/console-cleanup";
import { DrizzleInboundReplayRepository } from "@/platform/database/inbound-email-replay-repository";
import { DrizzleRetentionBacklogProbe } from "@/platform/database/retention-backlog";
import { DrizzleRetentionRunRepository } from "@/platform/database/retention-repository";

let service: RetentionRunService | undefined;

/**
 * Build the isolated retention stages, one per cleanup area. The artifacts
 * stage resolves `getArtifactService()` lazily inside its own `execute()`
 * rather than at construction time: that service depends on the plan-receipt
 * keyring, which can throw on misconfiguration, and a synchronous throw here
 * would previously abort building every other stage before
 * `RetentionRunService` even existed. Deferring the resolution lets a broken
 * keyring fail only the "artifacts" stage through the normal
 * failedAreas/partial isolation instead of the whole retention run. Once
 * `getArtifactService()` succeeds it memoizes itself, so later runs reuse the
 * same instance instead of rebuilding it.
 */
export function buildRetentionStages(
  db: ReturnType<typeof getDatabase>,
): RetentionStage[] {
  const replay = new DrizzleInboundReplayRepository(db);
  return [
    {
      async execute(_now, signal) {
        signal.throwIfAborted();
        const artifacts = getArtifactService();
        const report = await artifacts.reconcile(signal);
        return {
          counters: {
            artifactCandidates: report.candidates,
            artifactFailures: report.failures,
            artifactsDeleted: report.deleted,
          },
          outcome: report.failures > 0 ? "partial" as const : "succeeded" as const,
        };
      },
      name: "artifacts",
    },
    cleanupStage("inbound_email_replays", "inboundReplayRecordsDeleted", (now) => replay.cleanup(now)),
    cleanupStage("console_authorizations", "consoleAuthorizationRecordsDeleted", (now) => cleanupConsoleAuthorizations(now, db)),
    // Count sessions before an expired family cascade can remove them.
    cleanupStage("console_sessions", "consoleSessionRecordsDeleted", (now) => cleanupConsoleSessions(now, db)),
    cleanupStage("console_device_families", "consoleDeviceFamilyRecordsDeleted", (now) => cleanupConsoleDeviceFamilies(now, db)),
    cleanupStage("console_rate_limits", "consoleRateLimitRecordsDeleted", (now) => cleanupConsoleRateLimits(now, db)),
  ];
}

export function getRetentionRunService(): RetentionRunService {
  if (service) return service;
  const db = getDatabase();
  service = new RetentionRunService(
    new DrizzleRetentionRunRepository(db),
    new DrizzleRetentionBacklogProbe(db),
    buildRetentionStages(db),
  );
  return service;
}

export function setRetentionRunServiceForTests(
  value?: RetentionRunService,
): void {
  service = value;
}

function cleanupStage(
  name: Exclude<RetentionStage["name"], "artifacts">,
  counter: "consoleAuthorizationRecordsDeleted" |
    "consoleDeviceFamilyRecordsDeleted" |
    "consoleRateLimitRecordsDeleted" |
    "consoleSessionRecordsDeleted" |
    "inboundReplayRecordsDeleted",
  cleanup: (now: Date) => Promise<number>,
): RetentionStage {
  return {
    async execute(now, signal) {
      signal.throwIfAborted();
      const deleted = await cleanup(now);
      signal.throwIfAborted();
      return { counters: { [counter]: deleted } };
    },
    name,
  };
}
