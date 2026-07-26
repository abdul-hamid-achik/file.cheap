import { describe, expect, test } from "bun:test";

import type { RunListQuery, RunSummary } from "@/features/runs/contracts";
import type { RunRepository } from "@/features/runs/repository";
import { RunService } from "@/features/runs/service";

const run: RunSummary = {
  artifactId: `art_${"a".repeat(32)}`,
  counts: { artifacts: 1, outcomes: 1, steps: 2 },
  createdAt: "2026-07-26T12:00:00.000Z",
  detector: { name: "glyphrun-run", version: "1" },
  evidence: [{ inspectability: "metadata-only", integrity: "verified", medium: "text", path: "run.json", presence: "present", role: "run-record", sensitivity: "metadata-safe" }],
  health: { changed: 0, declared: 1, empty: 0, missing: 0, present: 1, reasons: [], state: "ok" },
  outcomes: [{ id: "case-1", status: "passed" }],
  producer: { native_id: "run-1", native_schema: "urn:glyphrun.dev:run:v1", tool: "glyphrun" },
  run: { nativeId: "run-1", seriesKey: "series_key_123456", status: "passed" },
  runIndexSha256: "b".repeat(64),
  source: { contentType: "application/zstd", kind: "glyphrun.run", sha256: "c".repeat(64), sizeBytes: 1024 },
  updatedAt: "2026-07-26T12:00:00.000Z",
};

describe("RunService", () => {
  test("keeps owner scope in the repository boundary and returns an opaque 404", async () => {
    const repository = new OwnerScopedRunRepository("acc_owner123", run);
    const service = new RunService(repository);
    expect((await service.list({ limit: 50 }, "acc_owner123")).runs).toEqual([run]);
    expect((await service.list({ limit: 50 }, "acc_other")).runs).toEqual([]);
    expect(await service.get(run.artifactId, "acc_owner123")).toEqual(run);
    await expect(service.get(run.artifactId, "acc_other")).rejects.toMatchObject({ code: "run_not_found", status: 404 });
  });
});

class OwnerScopedRunRepository implements RunRepository {
  constructor(private readonly owner: string, private readonly record: RunSummary) {}
  async find(artifactId: string, ownerAccountId: string) { return ownerAccountId === this.owner && artifactId === this.record.artifactId ? this.record : null; }
  async list(_query: RunListQuery, ownerAccountId: string) { return { nextCursor: null, runs: ownerAccountId === this.owner ? [this.record] : [] }; }
}
