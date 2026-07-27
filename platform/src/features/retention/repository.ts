import type {
  RetentionCounters,
  RetentionFailureArea,
  RetentionHealth,
  RetentionRun,
  RetentionRunStatus,
  RetentionStageName,
  RetentionStageResult,
} from "@/features/retention/contracts";

export interface RetentionRunRepository {
  abandonStale(input: {
    abandonedAt: Date;
    staleBefore: Date;
  }): Promise<RetentionRun[]>;
  finish(input: {
    counters: RetentionCounters;
    failedAreas: RetentionFailureArea[];
    finishedAt: Date;
    id: string;
    oldestDueAt: Date | null;
    status: Exclude<RetentionRunStatus, "running">;
  }): Promise<RetentionRun>;
  health(oldestDueAt: Date | null): Promise<RetentionHealth>;
  heartbeat(id: string, at: Date): Promise<boolean>;
  start(input: { id: string; startedAt: Date }): Promise<RetentionRun>;
}

export interface RetentionBacklogProbe {
  oldestDueAt(now: Date): Promise<Date | null>;
}

export type RetentionStage = Readonly<{
  // Stages must stop before beginning more destructive work when this signal
  // aborts. Database stages are one bounded statement; object stages pass the
  // signal through to storage and check it between candidates.
  execute(now: Date, signal: AbortSignal): Promise<RetentionStageResult>;
  name: RetentionStageName;
}>;
