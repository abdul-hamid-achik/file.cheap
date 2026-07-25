import { createHash } from "node:crypto";
import { describe, expect, test } from "bun:test";
import type { ArtifactPlanResponse, ArtifactPlanResult } from "@/features/artifacts/contracts";
import { ArtifactService } from "@/features/artifacts/service";
import { InMemoryArtifactRepository } from "@/features/artifacts/repository";
import { InMemoryArtifactObjectStore } from "@/platform/artifacts/in-memory-object-store";

const bytes = new TextEncoder().encode("artifact-test");
const sha256 = createHash("sha256").update(bytes).digest("hex");

describe("ArtifactService", () => {
  test("plans, commits, downloads, and emits a credential-free ArtifactRefV1", async () => {
    const store = new InMemoryArtifactObjectStore();
    const service = new ArtifactService(store, new InMemoryArtifactRepository());
    const plan = requirePlanned(await service.plan({ contentType: "application/zstd", idempotencyKey: "123e4567-e89b-42d3-a456-426614174000", kind: "chalupa.log-bundle", producer: { tool: "chalupa", version: "1.0.0" }, sha256, sizeBytes: bytes.byteLength }));
    expect(plan.artifactRef).toMatchObject({ $schema: "urn:filecheap.dev:artifact-ref:v1", provider: "fcheap-cloud" });
    expect(plan.artifactRef.uri).not.toContain("?");
    const key = new URL(plan.upload.url).pathname.replace(/^\/upload\//, "");
    store.seed({ bytes, contentType: "application/zstd", key, sizeBytes: bytes.byteLength });
    const committed = await service.commit(plan.receipt);
    expect(committed.artifact.state).toBe("committed");
    const download = await service.download({ artifactId: committed.artifact.artifactId });
    expect(download.download.method).toBe("GET");
    expect(download.artifact.verification).toBe("server-sha256");
  });

  test("reconciles expired objects exactly once", async () => {
    const store = new InMemoryArtifactObjectStore();
    let now = new Date("2026-07-24T00:00:00.000Z");
    const service = new ArtifactService(store, new InMemoryArtifactRepository(), () => now);
    const plan = requirePlanned(await service.plan({ contentType: "application/zstd", expiresAt: "2026-07-24T00:05:00.000Z", idempotencyKey: "123e4567-e89b-42d3-a456-426614174001", kind: "chalupa.log-bundle", producer: { tool: "chalupa" }, sha256, sizeBytes: bytes.byteLength }));
    store.seed({ bytes, contentType: "application/zstd", key: new URL(plan.upload.url).pathname.replace(/^\/upload\//, ""), sizeBytes: bytes.byteLength });
    await service.commit(plan.receipt);
    now = new Date("2026-07-24T00:05:00.000Z");
    expect(await service.reconcile()).toEqual({ deleted: 1 });
    expect(await service.reconcile()).toEqual({ deleted: 0 });
  });

  test("binds idempotency to the complete plan and its original expiry", async () => {
    const store = new InMemoryArtifactObjectStore();
    let now = new Date("2026-07-24T00:00:00.000Z");
    const service = new ArtifactService(
      store,
      new InMemoryArtifactRepository(),
      () => now,
    );
    const input = {
      contentType: "application/zstd",
      idempotencyKey: "123e4567-e89b-42d3-a456-426614174010",
      kind: "chalupa.log-chunk",
      producer: { tool: "chalupa", native_schema: "urn:chalupa:log-chunk:v1" },
      sha256,
      sizeBytes: bytes.byteLength,
    } as const;

    const first = requirePlanned(await service.plan(input));
    now = new Date("2026-07-24T00:05:00.000Z");
    const replay = requirePlanned(await service.plan(input));
    expect(replay.receipt).toBe(first.receipt);
    expect(replay.upload.expiresAt).toBe(first.upload.expiresAt);

    await expect(
      service.plan({
        ...input,
        producer: { tool: "different-producer" },
      }),
    ).rejects.toMatchObject({ code: "idempotency_conflict", status: 409 });

    now = new Date("2026-07-24T00:16:00.000Z");
    const renewed = requirePlanned(await service.plan(input));
    expect(renewed.receipt).toBe(first.receipt);
    expect(renewed.upload.expiresAt).toBe("2026-07-24T00:31:00.000Z");
  });

  test("renews an expired identical plan and commits bytes from an ambiguous earlier upload", async () => {
    const store = new InMemoryArtifactObjectStore();
    let now = new Date("2026-07-24T00:00:00.000Z");
    const service = new ArtifactService(store, new InMemoryArtifactRepository(), () => now);
    const input = {
      contentType: "application/zstd",
      idempotencyKey: "123e4567-e89b-42d3-a456-426614174013",
      kind: "chalupa.log-chunk",
      producer: { tool: "chalupa" },
      sha256,
      sizeBytes: bytes.byteLength,
    } as const;
    const first = requirePlanned(await service.plan(input));
    const key = new URL(first.upload.url).pathname.replace(/^\/upload\//, "");
    store.seed({ bytes, contentType: input.contentType, key, sizeBytes: bytes.byteLength });

    now = new Date("2026-07-24T00:16:00.000Z");
    const renewed = requirePlanned(await service.plan(input));
    expect(renewed.artifact.artifactId).toBe(first.artifact.artifactId);
    expect(renewed.receipt).toBe(first.receipt);
    expect((await service.commit(renewed.receipt)).artifact.state).toBe("committed");
  });

  test("collapses concurrent identical plans and rejects a concurrent conflicting binding", async () => {
    const service = new ArtifactService(
      new InMemoryArtifactObjectStore(),
      new InMemoryArtifactRepository(),
      () => new Date("2026-07-24T00:00:00.000Z"),
    );
    const input = {
      contentType: "application/zstd",
      idempotencyKey: "123e4567-e89b-42d3-a456-426614174015",
      kind: "chalupa.log-chunk",
      producer: { tool: "chalupa" },
      sha256,
      sizeBytes: bytes.byteLength,
    } as const;
    const [leftResult, rightResult] = await Promise.all([service.plan(input), service.plan(input)]);
    const left = requirePlanned(leftResult);
    const right = requirePlanned(rightResult);
    expect(right.receipt).toBe(left.receipt);
    expect(right.artifact.artifactId).toBe(left.artifact.artifactId);

    const conflictingService = new ArtifactService(
      new InMemoryArtifactObjectStore(),
      new InMemoryArtifactRepository(),
      () => new Date("2026-07-24T00:00:00.000Z"),
    );
    const results = await Promise.allSettled([
      conflictingService.plan(input),
      conflictingService.plan({ ...input, producer: { tool: "other" } }),
    ]);
    expect(results.filter((result) => result.status === "fulfilled")).toHaveLength(1);
    const rejected = results.find((result) => result.status === "rejected");
    expect(rejected).toMatchObject({ reason: { code: "idempotency_conflict", status: 409 } });
  });

  test("reclaims abandoned plans and permits the same exact plan after cleanup", async () => {
    const store = new InMemoryArtifactObjectStore();
    const repository = new InMemoryArtifactRepository();
    let now = new Date("2026-07-24T00:00:00.000Z");
    const service = new ArtifactService(store, repository, () => now);
    const input = {
      contentType: "application/zstd",
      idempotencyKey: "123e4567-e89b-42d3-a456-426614174014",
      kind: "chalupa.log-chunk",
      producer: { tool: "chalupa" },
      sha256,
      sizeBytes: bytes.byteLength,
    } as const;
    const first = requirePlanned(await service.plan(input));
    const key = new URL(first.upload.url).pathname.replace(/^\/upload\//, "");
    store.seed({ bytes, contentType: input.contentType, key, sizeBytes: bytes.byteLength });

    now = new Date("2026-07-24T00:16:00.000Z");
    expect(await service.reconcile()).toEqual({ deleted: 1 });
    expect(await store.inspect(key)).toBeNull();

    const restarted = requirePlanned(await service.plan(input));
    expect(restarted.artifact.artifactId).toBe(first.artifact.artifactId);
    expect(restarted.receipt).not.toBe(first.receipt);
  });

  test("returns a completed exact plan without issuing another upload grant", async () => {
    const store = new InMemoryArtifactObjectStore();
    const service = new ArtifactService(store, new InMemoryArtifactRepository());
    const input = {
      contentType: "application/zstd",
      idempotencyKey: "123e4567-e89b-42d3-a456-426614174011",
      kind: "chalupa.log-chunk",
      producer: { tool: "chalupa" },
      sha256,
      sizeBytes: bytes.byteLength,
    } as const;
    const plan = requirePlanned(await service.plan(input));
    store.seed({
      bytes,
      contentType: input.contentType,
      key: new URL(plan.upload.url).pathname.replace(/^\/upload\//, ""),
      sizeBytes: bytes.byteLength,
    });
    const committed = await service.commit(plan.receipt);
    const recovered = await service.plan(input);
    expect(JSON.stringify(recovered)).toBe(JSON.stringify(committed));
    expect(recovered.artifact.state).toBe("committed");
    expect("upload" in recovered).toBe(false);

    let now = new Date("2026-07-24T00:16:00.000Z");
    const expiringService = new ArtifactService(store, new InMemoryArtifactRepository(), () => now);
    const expiringPlan = requirePlanned(await expiringService.plan({ ...input, idempotencyKey: "123e4567-e89b-42d3-a456-426614174017" }));
    store.seed({
      bytes,
      contentType: input.contentType,
      key: new URL(expiringPlan.upload.url).pathname.replace(/^\/upload\//, ""),
      sizeBytes: bytes.byteLength,
    });
    await expiringService.commit(expiringPlan.receipt);
    now = new Date("2026-07-24T00:32:00.000Z");
    expect((await expiringService.commit(expiringPlan.receipt)).artifact.state).toBe("committed");
  });

  test("rejects a cross-producer commit before inspecting private storage", async () => {
    const store = new CountingInspectStore();
    const service = new ArtifactService(store, new InMemoryArtifactRepository());
    const plan = requirePlanned(await service.plan({
      contentType: "application/zstd",
      idempotencyKey: "123e4567-e89b-42d3-a456-426614174018",
      kind: "chalupa.log-chunk",
      producer: {
        native_schema: "urn:chalupa:log-chunk:v1",
        tool: "chalupa",
      },
      sha256,
      sizeBytes: bytes.byteLength,
    }));

    await expect(
      service.commit(plan.receipt, undefined, {
        kinds: ["cairntrace.run"],
        nativeSchemas: ["urn:cairntrace.dev:run:v1"],
        producerTool: "cairntrace",
      }),
    ).rejects.toMatchObject({ code: "invalid_receipt", status: 400 });
    expect(store.inspectCalls).toBe(0);
  });

  test("restricts OIDC downloads before inspecting private storage", async () => {
    const store = new CountingInspectStore();
    const service = new ArtifactService(
      store,
      new InMemoryArtifactRepository(),
    );
    const plan = requirePlanned(await service.plan({
      contentType: "application/gzip",
      idempotencyKey: "123e4567-e89b-42d3-a456-426614174019",
      kind: "cairntrace.run",
      producer: {
        native_schema: "urn:cairntrace.dev:run:v1",
        tool: "cairntrace",
      },
      sha256,
      sizeBytes: bytes.byteLength,
    }));
    store.seed({
      bytes,
      contentType: "application/gzip",
      key: new URL(plan.upload.url).pathname.replace(/^\/upload\//, ""),
      sizeBytes: bytes.byteLength,
    });
    const committed = await service.commit(plan.receipt);
    const inspectCallsAfterCommit = store.inspectCalls;

    await expect(
      service.download(
        { artifactId: committed.artifact.artifactId },
        undefined,
        {
          kinds: ["chalupa.log-chunk"],
          nativeSchemas: ["urn:chalupa:log-chunk:v1"],
          producerTool: "chalupa",
        },
      ),
    ).rejects.toMatchObject({ code: "artifact_not_found", status: 404 });
    expect(store.inspectCalls).toBe(inspectCallsAfterCommit);
  });

  test("does not grant expired artifacts before retention reconciliation", async () => {
    const store = new CountingInspectStore();
    let now = new Date("2026-07-24T00:00:00.000Z");
    const service = new ArtifactService(
      store,
      new InMemoryArtifactRepository(),
      () => now,
    );
    const plan = requirePlanned(await service.plan({
      contentType: "application/zstd",
      expiresAt: "2026-07-24T00:05:00.000Z",
      idempotencyKey: "123e4567-e89b-42d3-a456-426614174020",
      kind: "chalupa.log-chunk",
      producer: {
        native_schema: "urn:chalupa:log-chunk:v1",
        tool: "chalupa",
      },
      sha256,
      sizeBytes: bytes.byteLength,
    }));
    store.seed({
      bytes,
      contentType: "application/zstd",
      key: new URL(plan.upload.url).pathname.replace(/^\/upload\//, ""),
      sizeBytes: bytes.byteLength,
    });
    const committed = await service.commit(plan.receipt);
    const inspectCallsAfterCommit = store.inspectCalls;
    now = new Date("2026-07-24T00:05:00.000Z");

    await expect(
      service.download({ artifactId: committed.artifact.artifactId }),
    ).rejects.toMatchObject({ code: "artifact_not_found", status: 404 });
    expect(store.inspectCalls).toBe(inspectCallsAfterCommit);
  });

  test("caps a download grant at the artifact retention boundary", async () => {
    const store = new InMemoryArtifactObjectStore();
    let now = new Date("2026-07-24T00:00:00.000Z");
    const service = new ArtifactService(
      store,
      new InMemoryArtifactRepository(),
      () => now,
    );
    const expiresAt = "2026-07-24T00:05:00.000Z";
    const plan = requirePlanned(await service.plan({
      contentType: "application/zstd",
      expiresAt,
      idempotencyKey: "123e4567-e89b-42d3-a456-426614174021",
      kind: "chalupa.log-chunk",
      producer: {
        native_schema: "urn:chalupa:log-chunk:v1",
        tool: "chalupa",
      },
      sha256,
      sizeBytes: bytes.byteLength,
    }));
    store.seed({
      bytes,
      contentType: "application/zstd",
      key: new URL(plan.upload.url).pathname.replace(/^\/upload\//, ""),
      sizeBytes: bytes.byteLength,
    });
    const committed = await service.commit(plan.receipt);
    now = new Date("2026-07-24T00:04:30.000Z");

    const download = await service.download({
      artifactId: committed.artifact.artifactId,
    });
    expect(download.download.expiresAt).toBe(expiresAt);
  });

  test("caps the upload plan and grant at the artifact retention boundary", async () => {
    const repository = new InMemoryArtifactRepository();
    const now = new Date("2026-07-24T00:00:00.000Z");
    const service = new ArtifactService(
      new InMemoryArtifactObjectStore(),
      repository,
      () => now,
    );
    const expiresAt = "2026-07-24T00:05:00.000Z";
    const plan = requirePlanned(await service.plan({
      contentType: "application/zstd",
      expiresAt,
      idempotencyKey: "123e4567-e89b-42d3-a456-426614174024",
      kind: "chalupa.log-chunk",
      producer: { tool: "chalupa" },
      sha256,
      sizeBytes: bytes.byteLength,
    }));

    expect(plan.upload.expiresAt).toBe(expiresAt);
    expect(
      (await repository.find(plan.artifact.artifactId))?.planExpiresAt.toISOString(),
    ).toBe(expiresAt);
  });

  test("rejects a planned commit exactly at retention before inspecting storage", async () => {
    const store = new CountingInspectStore();
    const repository = new InMemoryArtifactRepository();
    let now = new Date("2026-07-24T00:00:00.000Z");
    const service = new ArtifactService(store, repository, () => now);
    const input = {
      contentType: "application/zstd",
      expiresAt: "2026-07-24T00:05:00.000Z",
      idempotencyKey: "123e4567-e89b-42d3-a456-426614174025",
      kind: "chalupa.log-chunk",
      producer: { tool: "chalupa" },
      sha256,
      sizeBytes: bytes.byteLength,
    } as const;
    const plan = requirePlanned(await service.plan(input));
    store.seed({
      bytes,
      contentType: input.contentType,
      key: new URL(plan.upload.url).pathname.replace(/^\/upload\//, ""),
      sizeBytes: bytes.byteLength,
    });
    now = new Date(input.expiresAt);

    await expect(service.commit(plan.receipt)).rejects.toMatchObject({
      code: "invalid_receipt",
      status: 400,
    });
    await expect(service.plan(input)).rejects.toMatchObject({
      code: "artifact_retention_expired",
      status: 422,
    });
    expect(store.inspectCalls).toBe(0);
  });

  test("rejects a commit when retention expires during object verification", async () => {
    let now = new Date("2026-07-24T00:00:00.000Z");
    const expiresAt = new Date("2026-07-24T00:05:00.000Z");
    const store = new AdvancesTimeOnInspectStore(() => {
      now = expiresAt;
    });
    const repository = new InMemoryArtifactRepository();
    const service = new ArtifactService(store, repository, () => now);
    const plan = requirePlanned(await service.plan({
      contentType: "application/zstd",
      expiresAt: expiresAt.toISOString(),
      idempotencyKey: "123e4567-e89b-42d3-a456-426614174026",
      kind: "chalupa.log-chunk",
      producer: { tool: "chalupa" },
      sha256,
      sizeBytes: bytes.byteLength,
    }));
    store.seed({
      bytes,
      contentType: "application/zstd",
      key: new URL(plan.upload.url).pathname.replace(/^\/upload\//, ""),
      sizeBytes: bytes.byteLength,
    });

    await expect(service.commit(plan.receipt)).rejects.toMatchObject({
      code: "commit_conflict",
      status: 409,
    });
    expect((await repository.find(plan.artifact.artifactId))?.state).toBe("planned");
  });

  test("rejects a committed receipt replay at the retention boundary", async () => {
    const store = new CountingInspectStore();
    let now = new Date("2026-07-24T00:00:00.000Z");
    const expiresAt = "2026-07-24T00:05:00.000Z";
    const service = new ArtifactService(
      store,
      new InMemoryArtifactRepository(),
      () => now,
    );
    const plan = requirePlanned(await service.plan({
      contentType: "application/zstd",
      expiresAt,
      idempotencyKey: "123e4567-e89b-42d3-a456-426614174027",
      kind: "chalupa.log-chunk",
      producer: { tool: "chalupa" },
      sha256,
      sizeBytes: bytes.byteLength,
    }));
    store.seed({
      bytes,
      contentType: "application/zstd",
      key: new URL(plan.upload.url).pathname.replace(/^\/upload\//, ""),
      sizeBytes: bytes.byteLength,
    });
    await service.commit(plan.receipt);
    const inspectionsBeforeExpiry = store.inspectCalls;
    now = new Date(expiresAt);

    await expect(service.commit(plan.receipt)).rejects.toMatchObject({
      code: "invalid_receipt",
      status: 400,
    });
    expect(store.inspectCalls).toBe(inspectionsBeforeExpiry);
  });

  test("leaves a deletion lease for recovery when the final metadata transition fails", async () => {
    const store = new InMemoryArtifactObjectStore();
    const repository = new FailsOnceMarkDeletedRepository();
    let now = new Date("2026-07-24T00:00:00.000Z");
    const service = new ArtifactService(store, repository, () => now);
    const plan = requirePlanned(await service.plan({
      contentType: "application/zstd",
      expiresAt: "2026-07-24T00:05:00.000Z",
      idempotencyKey: "123e4567-e89b-42d3-a456-426614174012",
      kind: "chalupa.log-chunk",
      producer: { tool: "chalupa" },
      sha256,
      sizeBytes: bytes.byteLength,
    }));
    store.seed({
      bytes,
      contentType: "application/zstd",
      key: new URL(plan.upload.url).pathname.replace(/^\/upload\//, ""),
      sizeBytes: bytes.byteLength,
    });
    const committed = await service.commit(plan.receipt);

    now = new Date("2026-07-24T00:05:00.000Z");
    await expect(service.reconcile()).rejects.toThrow(
      "synthetic metadata failure",
    );
    expect((await repository.find(committed.artifact.artifactId))?.state).toBe("deleting");
    now = new Date("2026-07-24T00:21:00.000Z");
    expect(await service.reconcile()).toEqual({ deleted: 1 });
  });

  test("restores the original state when private object deletion fails", async () => {
    const store = new FailsOnceDeleteStore();
    const repository = new InMemoryArtifactRepository();
    let now = new Date("2026-07-24T00:00:00.000Z");
    const service = new ArtifactService(store, repository, () => now);
    const plan = requirePlanned(await service.plan({
      contentType: "application/zstd",
      expiresAt: "2026-07-24T00:05:00.000Z",
      idempotencyKey: "123e4567-e89b-42d3-a456-426614174016",
      kind: "chalupa.log-chunk",
      producer: { tool: "chalupa" },
      sha256,
      sizeBytes: bytes.byteLength,
    }));
    store.seed({
      bytes,
      contentType: "application/zstd",
      key: new URL(plan.upload.url).pathname.replace(/^\/upload\//, ""),
      sizeBytes: bytes.byteLength,
    });
    const committed = await service.commit(plan.receipt);

    now = new Date("2026-07-24T00:05:00.000Z");
    await expect(service.reconcile()).rejects.toThrow("synthetic storage failure");
    expect((await repository.find(committed.artifact.artifactId))?.state).toBe("committed");
    expect(await service.reconcile()).toEqual({ deleted: 1 });
  });

  test("continues a deterministic retention batch after one candidate fails", async () => {
    const store = new FailsForKeyDeleteStore();
    const repository = new InMemoryArtifactRepository();
    let now = new Date("2026-07-24T00:00:00.000Z");
    const service = new ArtifactService(store, repository, () => now);
    const input = {
      contentType: "application/zstd",
      expiresAt: "2026-07-24T00:05:00.000Z",
      kind: "chalupa.log-chunk",
      producer: { tool: "chalupa" },
      sha256,
      sizeBytes: bytes.byteLength,
    } as const;
    const poisonPlan = requirePlanned(await service.plan({
      ...input,
      idempotencyKey: "123e4567-e89b-42d3-a456-426614174022",
    }));
    const healthyPlan = requirePlanned(await service.plan({
      ...input,
      idempotencyKey: "123e4567-e89b-42d3-a456-426614174023",
    }));
    const poisonKey = new URL(poisonPlan.upload.url).pathname.replace(/^\/upload\//, "");
    const healthyKey = new URL(healthyPlan.upload.url).pathname.replace(/^\/upload\//, "");
    store.seed({
      bytes,
      contentType: input.contentType,
      key: poisonKey,
      sizeBytes: bytes.byteLength,
    });
    store.seed({
      bytes,
      contentType: input.contentType,
      key: healthyKey,
      sizeBytes: bytes.byteLength,
    });
    const poison = await service.commit(poisonPlan.receipt);
    const healthy = await service.commit(healthyPlan.receipt);
    store.failedKey = poisonKey;
    now = new Date(input.expiresAt);

    await expect(service.reconcile()).rejects.toThrow(
      "synthetic permanent storage failure",
    );

    expect(store.deleteAttempts).toEqual([poisonKey, healthyKey]);
    expect((await repository.find(poison.artifact.artifactId))?.state).toBe("committed");
    expect((await repository.find(healthy.artifact.artifactId))?.state).toBe("deleted");
    expect(await store.inspect(poisonKey)).not.toBeNull();
    expect(await store.inspect(healthyKey)).toBeNull();
  });
});

function requirePlanned(result: ArtifactPlanResult): ArtifactPlanResponse {
  if (!("upload" in result) || !("receipt" in result)) {
    throw new Error("Expected a planned artifact response");
  }
  return result;
}

class FailsOnceMarkDeletedRepository extends InMemoryArtifactRepository {
  private shouldFail = true;

  override async markDeleted(
    artifactId: string,
  ): Promise<void> {
    if (this.shouldFail) {
      this.shouldFail = false;
      throw new Error("synthetic metadata failure");
    }
    await super.markDeleted(artifactId);
  }
}

class FailsOnceDeleteStore extends InMemoryArtifactObjectStore {
  private shouldFail = true;

  override async delete(key: string): Promise<void> {
    if (this.shouldFail) {
      this.shouldFail = false;
      throw new Error("synthetic storage failure");
    }
    await super.delete(key);
  }
}

class FailsForKeyDeleteStore extends InMemoryArtifactObjectStore {
  deleteAttempts: string[] = [];
  failedKey = "";

  override async delete(key: string): Promise<void> {
    this.deleteAttempts.push(key);
    if (key === this.failedKey) {
      throw new Error("synthetic permanent storage failure");
    }
    await super.delete(key);
  }
}

class AdvancesTimeOnInspectStore extends InMemoryArtifactObjectStore {
  constructor(private readonly advance: () => void) {
    super();
  }

  override async inspect(key: string) {
    const result = await super.inspect(key);
    this.advance();
    return result;
  }
}

class CountingInspectStore extends InMemoryArtifactObjectStore {
  inspectCalls = 0;

  override async inspect(key: string) {
    this.inspectCalls += 1;
    return super.inspect(key);
  }
}
