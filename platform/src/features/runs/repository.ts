import type { RunListQuery, RunSummary } from "@/features/runs/contracts";

export interface RunRepository {
  find(artifactId: string, ownerAccountId: string): Promise<RunSummary | null>;
  list(query: RunListQuery, ownerAccountId: string): Promise<{ nextCursor: string | null; runs: RunSummary[] }>;
}
