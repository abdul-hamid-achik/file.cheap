import { createHash } from "node:crypto";
import { describe, expect, test } from "bun:test";

import type { ArtifactPlanResponse, ArtifactPlanResult } from "@/features/artifacts/contracts";
import { testPlanReceiptKeyring } from "@/features/artifacts/plan-receipts.test-helper";
import { InMemoryArtifactRepository } from "@/features/artifacts/repository";
import { ArtifactService } from "@/features/artifacts/service";
import { InMemoryArtifactObjectStore } from "@/platform/artifacts/in-memory-object-store";

const bytes = new TextEncoder().encode("owner-scoped-artifact");
const sha256 = createHash("sha256").update(bytes).digest("hex");

describe("console artifact access", () => {
  test("scopes list, detail, download, and deletion to the authenticated owner", async () => {
    const store = new InMemoryArtifactObjectStore();
    const service = new ArtifactService(store, new InMemoryArtifactRepository(), testPlanReceiptKeyring);
    const input = {
      contentType: "application/json",
      idempotencyKey: "00000000-0000-4000-8000-000000000099",
      kind: "glyphrun.evidence-pack",
      producer: { native_id: "run_1", tool: "glyphrun" },
      sha256,
      sizeBytes: bytes.byteLength,
    } as const;
    const plan = requirePlanned(await service.plan(input, undefined, "acc_owner123"));
    await expect(service.plan(input, undefined, "acc_someone_else")).rejects.toMatchObject({
      code: "idempotency_conflict",
    });
    const key = new URL(plan.upload.url).pathname.replace(/^\/upload\//u, "");
    store.seed({ bytes, contentType: "application/json", key, sizeBytes: bytes.byteLength });
    const committed = await service.commit(plan.receipt);

    expect((await service.list({ limit: 20 }, "acc_owner123")).artifacts).toHaveLength(1);
    expect((await service.list({ limit: 20 }, "acc_someone_else")).artifacts).toHaveLength(0);
    await expect(service.get(committed.artifact.artifactId, "acc_someone_else")).rejects.toMatchObject({ code: "artifact_not_found" });
    await expect(service.download({ artifactId: committed.artifact.artifactId }, undefined, undefined, "acc_someone_else")).rejects.toMatchObject({ code: "artifact_not_found" });

    expect(await service.delete(committed.artifact.artifactId, "acc_owner123")).toEqual({ artifactId: committed.artifact.artifactId, state: "deleted" });
    expect(await service.delete(committed.artifact.artifactId, "acc_owner123")).toEqual({ artifactId: committed.artifact.artifactId, state: "deleted" });
  });
});

function requirePlanned(result: ArtifactPlanResult): ArtifactPlanResponse {
  if (!("upload" in result)) throw new Error("Expected a fresh upload plan");
  return result;
}
