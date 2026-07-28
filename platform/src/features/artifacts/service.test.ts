import { createHash } from "node:crypto";
import { describe, expect, test } from "bun:test";
import type { ArtifactPlanResponse, ArtifactPlanResult } from "@/features/artifacts/contracts";
import { artifactPlanInputSchema } from "@/features/artifacts/contracts";
import { PlanReceiptKeyring } from "@/features/artifacts/plan-receipts";
import { testPlanReceiptKeyring } from "@/features/artifacts/plan-receipts.test-helper";
import { ArtifactService } from "@/features/artifacts/service";
import { InMemoryArtifactRepository } from "@/features/artifacts/repository";
import { InMemoryArtifactObjectStore } from "@/platform/artifacts/in-memory-object-store";

const bytes = new TextEncoder().encode("artifact-test");
const sha256 = createHash("sha256").update(bytes).digest("hex");

describe("ArtifactService", () => {
  test("accepts monitor.incident as an ordinary immutable artifact plan", () => {
    const input = artifactPlanInputSchema.parse({
      contentType: "application/zstd",
      idempotencyKey: "00000000-0000-4000-8000-000000000099",
      kind: "monitor.incident",
      producer: {
        entrypoint: "manifest.json",
        native_id: "incident_01",
        native_schema: "urn:monitor.dev:incident:v1",
        tool: "monitor",
      },
      sha256,
      sizeBytes: bytes.byteLength,
    });

    expect(input.kind).toBe("monitor.incident");
    expect(input.runIndex).toBeUndefined();
  });

  test("binds a metadata-only run index to the immutable artifact plan", async () => {
    const service = newTestArtifactService(new InMemoryArtifactObjectStore(), new InMemoryArtifactRepository());
    const input = artifactPlanInputSchema.parse({
      contentType: "application/json",
      idempotencyKey: "00000000-0000-4000-8000-000000000098",
      kind: "glyphrun.evidence-pack",
      producer: { native_id: "native-run-1", native_schema: "urn:glyphrun.dev:run:v1", tool: "glyphrun" },
      runIndex: {
        $schema: "urn:filecheap.dev:run-index:v1",
        counts: { artifacts: 1, outcomes: 1, steps: 4 },
        detector: { name: "glyphrun-run", version: "1" },
        evidence: [{ inspectability: "metadata-only", integrity: "declared", medium: "structured-text", path: "run.json", presence: "present", role: "run-overview", sensitivity: "metadata-safe" }],
        health: { changed: 0, declared: 1, empty: 0, missing: 0, present: 1, reasons: [], state: "ok" },
        outcomes: [{ id: "clean-exit", status: "passed" }],
        run: { nativeId: "native-run-1", seriesKey: "series_123456789", status: "passed" },
        version: 1,
      },
      sha256,
      sizeBytes: bytes.byteLength,
    });
    const first = requirePlanned(await service.plan(input, undefined, "acc_owner123"));
    expect(first.artifact.state).toBe("planned");
    await expect(service.plan({
      ...input,
      runIndex: { ...input.runIndex!, health: { ...input.runIndex!.health, state: "degraded" } },
    }, undefined, "acc_owner123")).rejects.toMatchObject({ code: "idempotency_conflict" });
    expect(() => artifactPlanInputSchema.parse({
      ...input,
      producer: { ...input.producer, native_id: "different-run" },
    })).toThrow();
  });

  test("plans, commits, downloads, and emits a credential-free ArtifactRefV1", async () => {
    const store = new InMemoryArtifactObjectStore();
    const service = newTestArtifactService(store, new InMemoryArtifactRepository());
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
    const service = newTestArtifactService(store, new InMemoryArtifactRepository(), () => now);
    const plan = requirePlanned(await service.plan({ contentType: "application/zstd", expiresAt: "2026-07-24T00:05:00.000Z", idempotencyKey: "123e4567-e89b-42d3-a456-426614174001", kind: "chalupa.log-bundle", producer: { tool: "chalupa" }, sha256, sizeBytes: bytes.byteLength }));
    store.seed({ bytes, contentType: "application/zstd", key: new URL(plan.upload.url).pathname.replace(/^\/upload\//, ""), sizeBytes: bytes.byteLength });
    await service.commit(plan.receipt);
    now = new Date("2026-07-24T00:05:00.000Z");
    expect(await service.reconcile()).toEqual({ candidates: 1, deleted: 1, failures: 0 });
    expect(await service.reconcile()).toEqual({ candidates: 0, deleted: 0, failures: 0 });
  });

  test("binds idempotency to the complete plan and its original expiry", async () => {
    const store = new InMemoryArtifactObjectStore();
    let now = new Date("2026-07-24T00:00:00.000Z");
    const service = newTestArtifactService(
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
    const service = newTestArtifactService(store, new InMemoryArtifactRepository(), () => now);
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
    const service = newTestArtifactService(
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

    const conflictingService = newTestArtifactService(
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

  test("keeps an old receipt valid across an overlapping key rotation", async () => {
    const store = new InMemoryArtifactObjectStore();
    const repository = new InMemoryArtifactRepository();
    const oldKeyring = receiptKeyring("old", [["old", 1, 2]]);
    const rotatedKeyring = receiptKeyring("current", [
      ["current", 3, 4],
      ["old", 1, 2],
    ]);
    const oldService = new ArtifactService(store, repository, oldKeyring);
    const rotatedService = new ArtifactService(store, repository, rotatedKeyring);
    const input = {
      contentType: "application/zstd",
      idempotencyKey: "123e4567-e89b-42d3-a456-426614174030",
      kind: "chalupa.log-chunk",
      producer: { tool: "chalupa" },
      sha256,
      sizeBytes: bytes.byteLength,
    } as const;

    const issued = requirePlanned(await oldService.plan(input));
    const replay = requirePlanned(await rotatedService.plan(input));
    expect(replay.receipt).toBe(issued.receipt);
    expect((await repository.find(issued.artifact.artifactId))?.planReceipt)
      .toMatchObject({ kid: "old", scheme: "hmac-sha256-v1" });

    store.seed({
      bytes,
      contentType: input.contentType,
      key: new URL(issued.upload.url).pathname.replace(/^\/upload\//u, ""),
      sizeBytes: bytes.byteLength,
    });
    expect((await rotatedService.commit(issued.receipt)).artifact.state)
      .toBe("committed");
  });

  test("never downgrades a new receipt to the dual-written raw token", async () => {
    const store = new CountingInspectStore();
    const repository = new InMemoryArtifactRepository();
    const service = newTestArtifactService(store, repository);
    const plan = requirePlanned(await service.plan({
      contentType: "application/zstd",
      idempotencyKey: "123e4567-e89b-42d3-a456-426614174031",
      kind: "chalupa.log-chunk",
      producer: { tool: "chalupa" },
      sha256,
      sizeBytes: bytes.byteLength,
    }));
    const record = await repository.find(plan.artifact.artifactId);
    if (!record?.planReceipt || record.planReceipt.nonce === null) {
      throw new Error("Expected a deterministic receipt record");
    }
    record.planReceipt = {
      ...record.planReceipt,
      lookup: Buffer.alloc(32, 0x7f).toString("base64url"),
    };

    await expect(service.commit(plan.receipt)).rejects.toMatchObject({
      code: "invalid_receipt",
      status: 400,
    });
    expect(record.planToken).toBe(plan.receipt);
    expect(store.inspectCalls).toBe(0);
  });

  test("accepts raw fallback only for a totally legacy row", async () => {
    const store = new InMemoryArtifactObjectStore();
    const repository = new InMemoryArtifactRepository();
    const service = newTestArtifactService(store, repository);
    const plan = requirePlanned(await service.plan({
      contentType: "application/zstd",
      idempotencyKey: "123e4567-e89b-42d3-a456-426614174032",
      kind: "chalupa.log-chunk",
      producer: { tool: "chalupa" },
      sha256,
      sizeBytes: bytes.byteLength,
    }));
    const record = await repository.find(plan.artifact.artifactId);
    if (!record) throw new Error("Expected a legacy test record");
    record.planReceipt = null;
    store.seed({
      bytes,
      contentType: record.contentType,
      key: record.objectKey,
      sizeBytes: bytes.byteLength,
    });

    expect((await service.commit(plan.receipt)).artifact.state).toBe("committed");
  });

  test("reclaims abandoned plans and permits the same exact plan after cleanup", async () => {
    const store = new InMemoryArtifactObjectStore();
    const repository = new InMemoryArtifactRepository();
    let now = new Date("2026-07-24T00:00:00.000Z");
    const service = newTestArtifactService(store, repository, () => now);
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
    expect(await service.reconcile()).toEqual({ candidates: 1, deleted: 1, failures: 0 });
    expect(await store.inspect(key)).toBeNull();

    const restarted = requirePlanned(await service.plan(input));
    expect(restarted.artifact.artifactId).toBe(first.artifact.artifactId);
    expect(restarted.receipt).not.toBe(first.receipt);
  });

  test("returns a completed exact plan without issuing another upload grant", async () => {
    const store = new InMemoryArtifactObjectStore();
    const service = newTestArtifactService(store, new InMemoryArtifactRepository());
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
    const expiringService = newTestArtifactService(store, new InMemoryArtifactRepository(), () => now);
    const expiringPlan = requirePlanned(await expiringService.plan({ ...input, idempotencyKey: "123e4567-e89b-42d3-a456-426614174017" }));
    store.seed({
      bytes,
      contentType: input.contentType,
      key: new URL(expiringPlan.upload.url).pathname.replace(/^\/upload\//, ""),
      sizeBytes: bytes.byteLength,
    });
    await expiringService.commit(expiringPlan.receipt);
    now = new Date("2026-07-24T00:32:00.000Z");
    await expect(expiringService.commit(expiringPlan.receipt)).rejects.toMatchObject({
      code: "invalid_receipt",
      status: 400,
    });
    expect((await expiringService.plan({
      ...input,
      idempotencyKey: "123e4567-e89b-42d3-a456-426614174017",
    })).artifact.state).toBe("committed");
  });

  test("verifies large artifacts by streaming and rejects a tampered object", async () => {
    const store = new InMemoryArtifactObjectStore();
    const service = newTestArtifactService(store, new InMemoryArtifactRepository());
    // Comfortably past the retired 2 MiB buffer-everything bound.
    const largeBytes = new Uint8Array(6 * 1024 * 1024);
    for (let index = 0; index < largeBytes.byteLength; index += 1) {
      largeBytes[index] = index % 251;
    }
    const largeSha256 = createHash("sha256").update(largeBytes).digest("hex");
    const input = {
      contentType: "application/zstd",
      idempotencyKey: "123e4567-e89b-42d3-a456-426614174030",
      kind: "chalupa.log-chunk",
      producer: { tool: "chalupa" },
      sha256: largeSha256,
      sizeBytes: largeBytes.byteLength,
    } as const;
    const plan = requirePlanned(await service.plan(input));
    const key = new URL(plan.upload.url).pathname.replace(/^\/upload\//, "");

    const tampered = largeBytes.slice();
    tampered[tampered.byteLength - 1] ^= 0xff;
    store.seed({ bytes: tampered, contentType: input.contentType, key, sizeBytes: tampered.byteLength });
    await expect(service.commit(plan.receipt)).rejects.toMatchObject({
      code: "integrity_mismatch",
      status: 422,
    });

    store.seed({ bytes: largeBytes, contentType: input.contentType, key, sizeBytes: largeBytes.byteLength });
    const committed = await service.commit(plan.receipt);
    expect(committed.artifact.state).toBe("committed");
    expect(committed.artifact.verification).toBe("server-sha256");
    expect(committed.artifact.sizeBytes).toBe(largeBytes.byteLength);
  });

  test("rejects a commit whose producer quota was tightened after the plan", async () => {
    const store = new InMemoryArtifactObjectStore();
    const service = newTestArtifactService(store, new InMemoryArtifactRepository());
    const plan = requirePlanned(await service.plan({
      contentType: "application/zstd",
      idempotencyKey: "123e4567-e89b-42d3-a456-426614174031",
      kind: "chalupa.log-chunk",
      producer: { native_schema: "urn:chalupa:log-chunk:v1", tool: "chalupa" },
      sha256,
      sizeBytes: bytes.byteLength,
    }));
    store.seed({
      bytes,
      contentType: "application/zstd",
      key: new URL(plan.upload.url).pathname.replace(/^\/upload\//, ""),
      sizeBytes: bytes.byteLength,
    });

    await expect(
      service.commit(plan.receipt, undefined, {
        kinds: ["chalupa.log-chunk"],
        maxSizeBytes: bytes.byteLength - 1,
        nativeSchemas: ["urn:chalupa:log-chunk:v1"],
        producerTool: "chalupa",
      }),
    ).rejects.toMatchObject({ code: "producer_quota_exceeded", status: 413 });

    const committed = await service.commit(plan.receipt, undefined, {
      kinds: ["chalupa.log-chunk"],
      maxSizeBytes: bytes.byteLength,
      nativeSchemas: ["urn:chalupa:log-chunk:v1"],
      producerTool: "chalupa",
    });
    expect(committed.artifact.state).toBe("committed");
  });

  test("rejects a cross-producer commit before inspecting private storage", async () => {
    const store = new CountingInspectStore();
    const service = newTestArtifactService(store, new InMemoryArtifactRepository());
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
        maxSizeBytes: 32 * 1024 * 1024,
        nativeSchemas: ["urn:cairntrace.dev:run:v1"],
        producerTool: "cairntrace",
      }),
    ).rejects.toMatchObject({ code: "invalid_receipt", status: 400 });
    expect(store.inspectCalls).toBe(0);
  });

  test("enforces exact OIDC kind and schema bindings on commit and download", async () => {
    const store = new CountingInspectStore();
    const service = newTestArtifactService(
      store,
      new InMemoryArtifactRepository(),
    );
    const policy = {
      kindSchemaBindings: [
        {
          kind: "chalupa.ci-artifact",
          nativeSchema: "urn:chalupa:ci-artifact:v1",
        },
        {
          kind: "chalupa.ci-manifest",
          nativeSchema: "urn:chalupa:ci-manifest:v1",
        },
      ],
      kinds: ["chalupa.ci-artifact", "chalupa.ci-manifest"],
      maxSizeBytes: 8 * 1024 * 1024,
      nativeSchemas: [
        "urn:chalupa:ci-artifact:v1",
        "urn:chalupa:ci-manifest:v1",
      ],
      producerTool: "chalupa",
    } as const;
    const mismatched = requirePlanned(await service.plan({
      contentType: "application/zstd",
      idempotencyKey: "123e4567-e89b-42d3-a456-426614174034",
      kind: "chalupa.ci-artifact",
      producer: {
        native_schema: "urn:chalupa:ci-manifest:v1",
        tool: "chalupa",
      },
      sha256,
      sizeBytes: bytes.byteLength,
    }));
    store.seed({
      bytes,
      contentType: "application/zstd",
      key: new URL(mismatched.upload.url).pathname.replace(/^\/upload\//, ""),
      sizeBytes: bytes.byteLength,
    });

    await expect(
      service.commit(mismatched.receipt, undefined, policy),
    ).rejects.toMatchObject({ code: "invalid_receipt", status: 400 });
    expect(store.inspectCalls).toBe(0);

    const committedMismatch = await service.commit(mismatched.receipt);
    const callsAfterCommit = store.inspectCalls;
    await expect(
      service.download(
        { artifactId: committedMismatch.artifact.artifactId },
        undefined,
        policy,
      ),
    ).rejects.toMatchObject({ code: "artifact_not_found", status: 404 });
    expect(store.inspectCalls).toBe(callsAfterCommit);

    const matching = requirePlanned(await service.plan({
      contentType: "application/zstd",
      idempotencyKey: "123e4567-e89b-42d3-a456-426614174035",
      kind: "chalupa.ci-artifact",
      producer: {
        native_schema: "urn:chalupa:ci-artifact:v1",
        tool: "chalupa",
      },
      sha256,
      sizeBytes: bytes.byteLength,
    }));
    store.seed({
      bytes,
      contentType: "application/zstd",
      key: new URL(matching.upload.url).pathname.replace(/^\/upload\//, ""),
      sizeBytes: bytes.byteLength,
    });
    const committedMatch = await service.commit(
      matching.receipt,
      undefined,
      policy,
    );
    await expect(
      service.download(
        { artifactId: committedMatch.artifact.artifactId },
        undefined,
        policy,
      ),
    ).resolves.toMatchObject({
      artifact: { artifactId: committedMatch.artifact.artifactId },
    });
  });

  test("restricts OIDC downloads before inspecting private storage", async () => {
    const store = new CountingInspectStore();
    const service = newTestArtifactService(
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
          maxSizeBytes: 8 * 1024 * 1024,
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
    const service = newTestArtifactService(
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
    const service = newTestArtifactService(
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
    const service = newTestArtifactService(
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
    const service = newTestArtifactService(store, repository, () => now);
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
    const service = newTestArtifactService(store, repository, () => now);
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
    const service = newTestArtifactService(
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
    const service = newTestArtifactService(store, repository, () => now);
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
    expect(await service.reconcile()).toEqual({ candidates: 1, deleted: 0, failures: 1 });
    expect((await repository.find(committed.artifact.artifactId))?.state).toBe("deleting");
    now = new Date("2026-07-24T00:21:00.000Z");
    expect(await service.reconcile()).toEqual({ candidates: 1, deleted: 1, failures: 0 });
  });

  test("restores the original state when private object deletion fails", async () => {
    const store = new FailsOnceDeleteStore();
    const repository = new InMemoryArtifactRepository();
    let now = new Date("2026-07-24T00:00:00.000Z");
    const service = newTestArtifactService(store, repository, () => now);
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
    expect(await service.reconcile()).toEqual({ candidates: 1, deleted: 0, failures: 1 });
    expect((await repository.find(committed.artifact.artifactId))?.state).toBe("committed");
    expect(await service.reconcile()).toEqual({ candidates: 1, deleted: 1, failures: 0 });
  });

  test("continues a deterministic retention batch after one candidate fails", async () => {
    const store = new FailsForKeyDeleteStore();
    const repository = new InMemoryArtifactRepository();
    let now = new Date("2026-07-24T00:00:00.000Z");
    const service = newTestArtifactService(store, repository, () => now);
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

    expect(await service.reconcile()).toEqual({ candidates: 2, deleted: 1, failures: 1 });

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

function newTestArtifactService(
  store: ConstructorParameters<typeof ArtifactService>[0],
  repository: ConstructorParameters<typeof ArtifactService>[1],
  now?: ConstructorParameters<typeof ArtifactService>[3],
): ArtifactService {
  return new ArtifactService(store, repository, testPlanReceiptKeyring, now);
}

function receiptKeyring(
  activeKid: string,
  keys: readonly (readonly [string, number, number])[],
): PlanReceiptKeyring {
  return new PlanReceiptKeyring({
    activeKid,
    keys: keys.map(([kid, signingByte, lookupByte]) => ({
      kid,
      lookupKey: Buffer.alloc(32, lookupByte),
      signingKey: Buffer.alloc(32, signingByte),
    })),
  });
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
