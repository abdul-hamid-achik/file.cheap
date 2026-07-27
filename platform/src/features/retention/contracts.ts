import { z } from "zod";

export const retentionRunStatusSchema = z.enum([
  "running",
  "succeeded",
  "partial",
  "failed",
  "abandoned",
]);

export const retentionRunIdSchema = z.string().regex(
  /^rtn_[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/,
);

export const retentionStageNameSchema = z.enum([
  "artifacts",
  "inbound_email_replays",
  "console_authorizations",
  "console_device_families",
  "console_sessions",
  "console_rate_limits",
]);

export const retentionFailureAreaSchema = z.enum([
  ...retentionStageNameSchema.options,
  "backlog_probe",
  "run_lease",
]);

const counterSchema = z.number().int().nonnegative();

export const retentionWorkCountersSchema = z.object({
  artifactCandidates: counterSchema,
  artifactFailures: counterSchema,
  artifactsDeleted: counterSchema,
  consoleAuthorizationRecordsDeleted: counterSchema,
  consoleDeviceFamilyRecordsDeleted: counterSchema,
  consoleRateLimitRecordsDeleted: counterSchema,
  consoleSessionRecordsDeleted: counterSchema,
  inboundReplayRecordsDeleted: counterSchema,
}).strict();

export const retentionCountersSchema = retentionWorkCountersSchema.extend({
  stagesAttempted: counterSchema,
  stagesFailed: counterSchema,
  stagesSucceeded: counterSchema,
}).strict();

export const retentionStageResultSchema = z.object({
  counters: retentionWorkCountersSchema.partial().default({}),
  outcome: z.enum(["succeeded", "partial"]).default("succeeded"),
}).strict();

export type RetentionCounters = z.infer<typeof retentionCountersSchema>;
export type RetentionFailureArea = z.infer<typeof retentionFailureAreaSchema>;
export type RetentionRunStatus = z.infer<typeof retentionRunStatusSchema>;
export type RetentionStageName = z.infer<typeof retentionStageNameSchema>;
export type RetentionStageResult = z.input<typeof retentionStageResultSchema>;
export type RetentionWorkCounters = z.infer<typeof retentionWorkCountersSchema>;

export type RetentionRun = Readonly<{
  counters: RetentionCounters;
  failedAreas: RetentionFailureArea[];
  finishedAt: Date | null;
  heartbeatAt: Date;
  id: string;
  oldestDueAt: Date | null;
  startedAt: Date;
  status: RetentionRunStatus;
}>;

export type RetentionHealth = Readonly<{
  activeRunCount: number;
  counters: RetentionCounters;
  lastFinishedAt: Date | null;
  lastRunId: string | null;
  lastStartedAt: Date | null;
  oldestDueAt: Date | null;
  status: RetentionRunStatus | null;
}>;

export const retentionRunReportSchema = z.object({
  counters: retentionCountersSchema,
  failedAreas: z.array(retentionFailureAreaSchema).max(8),
  finishedAt: z.iso.datetime().nullable(),
  oldestDueAt: z.iso.datetime().nullable(),
  runId: retentionRunIdSchema,
  startedAt: z.iso.datetime(),
  status: retentionRunStatusSchema,
  version: z.literal("filecheap-retention-run/1"),
}).strict();

export const retentionHealthReportSchema = z.object({
  activeRunCount: z.number().int().nonnegative(),
  counters: retentionCountersSchema,
  lastFinishedAt: z.iso.datetime().nullable(),
  lastRunId: retentionRunIdSchema.nullable(),
  lastStartedAt: z.iso.datetime().nullable(),
  oldestDueAt: z.iso.datetime().nullable(),
  status: retentionRunStatusSchema.nullable(),
  version: z.literal("filecheap-retention-health/1"),
}).strict();

export type RetentionRunReport = z.infer<typeof retentionRunReportSchema>;
export type RetentionHealthReport = z.infer<typeof retentionHealthReportSchema>;

export function retentionRunReport(run: RetentionRun): RetentionRunReport {
  return retentionRunReportSchema.parse({
    counters: run.counters,
    failedAreas: run.failedAreas,
    finishedAt: run.finishedAt?.toISOString() ?? null,
    oldestDueAt: run.oldestDueAt?.toISOString() ?? null,
    runId: run.id,
    startedAt: run.startedAt.toISOString(),
    status: run.status,
    version: "filecheap-retention-run/1",
  });
}

export function retentionHealthReport(
  health: RetentionHealth,
): RetentionHealthReport {
  return retentionHealthReportSchema.parse({
    activeRunCount: health.activeRunCount,
    counters: health.counters,
    lastFinishedAt: health.lastFinishedAt?.toISOString() ?? null,
    lastRunId: health.lastRunId,
    lastStartedAt: health.lastStartedAt?.toISOString() ?? null,
    oldestDueAt: health.oldestDueAt?.toISOString() ?? null,
    status: health.status,
    version: "filecheap-retention-health/1",
  });
}

export function emptyRetentionCounters(): RetentionCounters {
  return {
    artifactCandidates: 0,
    artifactFailures: 0,
    artifactsDeleted: 0,
    consoleAuthorizationRecordsDeleted: 0,
    consoleDeviceFamilyRecordsDeleted: 0,
    consoleRateLimitRecordsDeleted: 0,
    consoleSessionRecordsDeleted: 0,
    inboundReplayRecordsDeleted: 0,
    stagesAttempted: 0,
    stagesFailed: 0,
    stagesSucceeded: 0,
  };
}
