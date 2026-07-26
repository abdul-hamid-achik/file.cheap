import type { RunListQuery, RunSummary } from "@/features/runs/contracts";
import type { RunRepository } from "@/features/runs/repository";
import { PlatformError } from "@/shared/errors/platform-error";

export class RunService {
  constructor(private readonly repository: RunRepository) {}

  list(query: RunListQuery, ownerAccountId: string): Promise<{ nextCursor: string | null; runs: RunSummary[] }> {
    return this.repository.list(query, ownerAccountId);
  }

  async get(artifactId: string, ownerAccountId: string): Promise<RunSummary> {
    const run = await this.repository.find(artifactId, ownerAccountId);
    if (!run) throw runNotFound();
    return run;
  }
}

function runNotFound(): PlatformError {
  return new PlatformError({
    code: "run_not_found",
    detail: "The run does not exist or is not available to this console owner.",
    status: 404,
    title: "Run not found",
  });
}
