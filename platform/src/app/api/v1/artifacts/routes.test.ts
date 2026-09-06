import { createHash } from "node:crypto";
import { afterEach, describe, expect, test } from "bun:test";

import { POST as commit } from "@/app/api/v1/artifacts/commits/route";
import { POST as download } from "@/app/api/v1/artifacts/downloads/route";
import { POST as plan } from "@/app/api/v1/artifacts/plans/route";
import { GET as getArtifact } from "@/app/api/v1/artifacts/[artifactId]/route";
import { ArtifactService } from "@/features/artifacts/service";
import { testPlanReceiptKeyring } from "@/features/artifacts/plan-receipts.test-helper";
import { InMemoryArtifactRepository } from "@/features/artifacts/repository";
import { InMemoryArtifactObjectStore } from "@/platform/artifacts/in-memory-object-store";
import { setArtifactServiceForTests } from "@/features/artifacts/factory";
import { resetConfigForTests } from "@/shared/config/env";
import { defaultProducerMaxSizeBytes, maximumArtifactBytes } from "@/shared/config/limits";

const original = { ...process.env };
const ingest = "i".repeat(43);
const cairntraceIngest = "c".repeat(43);
const monitorIngest = "m".repeat(43);
// The documented chalupa-cli publisher policy from
// platform/docs/integrations/chalupa-agent-artifacts.md and .env.example.
const inferenceReceiptMaxSizeBytes = 256 * 1024;
const agentSessionMaxSizeBytes = 8 * 1024 * 1024;

afterEach(() => {
  for (const key of Object.keys(process.env)) if (!(key in original)) delete process.env[key];
  for (const [key, value] of Object.entries(original)) process.env[key] = value;
  resetConfigForTests();
  setArtifactServiceForTests();
});

describe("private artifact routes", () => {
  test("requires ingest authentication and commits only a verified direct upload", async () => {
    process.env.VERCEL = "1";
    process.env.DATABASE_URL = "postgresql://runtime";
    process.env.FILECHEAP_OIDC_ISSUER = "https://oidc.vercel.com/example";
    process.env.FILECHEAP_OIDC_AUDIENCE = "https://vercel.com/example";
    process.env.FILECHEAP_OIDC_SUBJECTS = "owner:example:project:chalupa:environment:production";
    process.env.FILECHEAP_PUBLISHER_TOKENS = JSON.stringify({
      cairntrace: {
        kinds: ["cairntrace.run"],
        maxSizeBytes: 32 * 1024 * 1024,
        nativeSchemas: ["urn:cairntrace.dev:run:v1"],
        tokens: [cairntraceIngest],
      },
      chalupa: {
        kinds: ["chalupa.log-chunk"],
        nativeSchemas: ["urn:chalupa:log-chunk:v1"],
        tokens: [ingest],
      },
      monitor: {
        kinds: ["monitor.incident"],
        nativeSchemas: ["urn:monitor.dev:incident:v1"],
        tokens: [monitorIngest],
      },
    });
    process.env.FILECHEAP_ADMIN_TOKEN = "a".repeat(32);
    process.env.FILECHEAP_OWNER_ACCOUNT_ID = "acc_owner123";
    process.env.CRON_SECRET = "z".repeat(32);
    resetConfigForTests();
    const bytes = new TextEncoder().encode("route artifact");
    const sha256 = createHash("sha256").update(bytes).digest("hex");
    const planInput = { contentType: "application/zstd", idempotencyKey: "123e4567-e89b-42d3-a456-426614174002", kind: "chalupa.log-chunk", producer: { native_schema: "urn:chalupa:log-chunk:v1", tool: "chalupa" }, sha256, sizeBytes: bytes.byteLength };
    const store = new InMemoryArtifactObjectStore();
    setArtifactServiceForTests(new ArtifactService(store, new InMemoryArtifactRepository(), testPlanReceiptKeyring));
    const malformedDetail = await getArtifact(
      new Request("https://file.cheap/api/v1/artifacts/not-an-artifact", {
        headers: { authorization: `Bearer ${"a".repeat(32)}` },
      }),
      { params: Promise.resolve({ artifactId: "not-an-artifact" }) },
    );
    expect(malformedDetail.status).toBe(422);
    expect((await malformedDetail.json()).code).toBe("invalid_request");
    const unauthorized = await plan(new Request("https://file.cheap/api/v1/artifacts/plans", { method: "POST", headers: { "content-type": "application/json" }, body: "{}" }));
    expect(unauthorized.status).toBe(401);
    const wrongProducer = await plan(new Request("https://file.cheap/api/v1/artifacts/plans", { method: "POST", headers: { authorization: `Bearer ${cairntraceIngest}`, "content-type": "application/json" }, body: JSON.stringify(planInput) }));
    expect(wrongProducer.status).toBe(401);
    const wrongKind = await plan(new Request("https://file.cheap/api/v1/artifacts/plans", { method: "POST", headers: { authorization: `Bearer ${ingest}`, "content-type": "application/json" }, body: JSON.stringify({ ...planInput, kind: "chalupa.report" }) }));
    expect(wrongKind.status).toBe(401);
    const overQuota = await plan(new Request("https://file.cheap/api/v1/artifacts/plans", { method: "POST", headers: { authorization: `Bearer ${ingest}`, "content-type": "application/json" }, body: JSON.stringify({ ...planInput, sizeBytes: defaultProducerMaxSizeBytes + 1 }) }));
    expect(overQuota.status).toBe(413);
    const overQuotaBody = await overQuota.json();
    expect(overQuotaBody.code).toBe("producer_quota_exceeded");
    expect(overQuotaBody.detail).toContain(`producer 'chalupa' allows up to ${defaultProducerMaxSizeBytes} bytes`);
    const overCeiling = await plan(new Request("https://file.cheap/api/v1/artifacts/plans", { method: "POST", headers: { authorization: `Bearer ${cairntraceIngest}`, "content-type": "application/json" }, body: JSON.stringify({ ...planInput, kind: "cairntrace.run", producer: { native_schema: "urn:cairntrace.dev:run:v1", tool: "cairntrace" }, sizeBytes: maximumArtifactBytes + 1 }) }));
    expect(overCeiling.status).toBe(422);
    const response = await plan(new Request("https://file.cheap/api/v1/artifacts/plans", { method: "POST", headers: { authorization: `Bearer ${ingest}`, "content-type": "application/json" }, body: JSON.stringify(planInput) }));
    expect(response.status).toBe(201);
    const monitorResponse = await plan(new Request("https://file.cheap/api/v1/artifacts/plans", {
      method: "POST",
      headers: { authorization: `Bearer ${monitorIngest}`, "content-type": "application/json" },
      body: JSON.stringify({
        ...planInput,
        idempotencyKey: "123e4567-e89b-42d3-a456-426614174003",
        kind: "monitor.incident",
        producer: {
          entrypoint: "manifest.json",
          native_id: "incident_01",
          native_schema: "urn:monitor.dev:incident:v1",
          tool: "monitor",
        },
      }),
    }));
    expect(monitorResponse.status).toBe(201);
    expect((await monitorResponse.json()).artifactRef).toMatchObject({
      kind: "monitor.incident",
      producer: {
        native_schema: "urn:monitor.dev:incident:v1",
        tool: "monitor",
      },
    });
    const planned = await response.json() as { receipt: string; upload: { url: string } };
    store.seed({ bytes, contentType: "application/zstd", key: new URL(planned.upload.url).pathname.replace(/^\/upload\//, ""), sizeBytes: bytes.byteLength });
    const crossProducerCommit = await commit(new Request("https://file.cheap/api/v1/artifacts/commits", { method: "POST", headers: { authorization: `Bearer ${cairntraceIngest}`, "content-type": "application/json" }, body: JSON.stringify({ receipt: planned.receipt }) }));
    expect(crossProducerCommit.status).toBe(400);
    expect((await crossProducerCommit.json()).code).toBe("invalid_receipt");
    const committed = await commit(new Request("https://file.cheap/api/v1/artifacts/commits", { method: "POST", headers: { authorization: `Bearer ${ingest}`, "content-type": "application/json" }, body: JSON.stringify({ receipt: planned.receipt }) }));
    expect(committed.status).toBe(200);
    const committedBody = await committed.json();
    expect(committedBody.artifact.verification).toBe("server-sha256");
    const publisherDownload = await download(new Request("https://file.cheap/api/v1/artifacts/downloads", { method: "POST", headers: { authorization: `Bearer ${ingest}`, "content-type": "application/json" }, body: JSON.stringify({ artifactId: committedBody.artifact.artifactId }) }));
    // The producer that published it may read it back; another producer's
    // credential never sees it exists.
    expect(publisherDownload.status).toBe(201);
    expect((await publisherDownload.json()).download.method).toBe("GET");
    const crossProducerDownload = await download(new Request("https://file.cheap/api/v1/artifacts/downloads", { method: "POST", headers: { authorization: `Bearer ${cairntraceIngest}`, "content-type": "application/json" }, body: JSON.stringify({ artifactId: committedBody.artifact.artifactId }) }));
    expect(crossProducerDownload.status).toBe(404);
    expect((await crossProducerDownload.json()).code).toBe("artifact_not_found");
    const adminDownload = await download(new Request("https://file.cheap/api/v1/artifacts/downloads", { method: "POST", headers: { authorization: `Bearer ${"a".repeat(32)}`, "content-type": "application/json" }, body: JSON.stringify({ artifactId: committedBody.artifact.artifactId }) }));
    expect(adminDownload.status).toBe(201);
    const recovered = await plan(new Request("https://file.cheap/api/v1/artifacts/plans", { method: "POST", headers: { authorization: `Bearer ${ingest}`, "content-type": "application/json" }, body: JSON.stringify(planInput) }));
    expect(recovered.status).toBe(200);
    const recoveredBody = await recovered.json();
    expect(recoveredBody.artifact.state).toBe("committed");
    expect(recoveredBody).not.toHaveProperty("upload");
  });

  test("lets a chalupa-cli laptop token publish and read back only its own inference receipts", async () => {
    const chalupaCliIngest = "l".repeat(43);
    process.env.VERCEL = "1";
    process.env.DATABASE_URL = "postgresql://runtime";
    process.env.FILECHEAP_OIDC_ISSUER = "https://oidc.vercel.com/example";
    process.env.FILECHEAP_OIDC_AUDIENCE = "https://vercel.com/example";
    process.env.FILECHEAP_OIDC_SUBJECTS = "owner:example:project:chalupa:environment:production";
    process.env.FILECHEAP_PUBLISHER_TOKENS = JSON.stringify({
      cairntrace: {
        kinds: ["cairntrace.run"],
        nativeSchemas: ["urn:cairntrace.dev:run:v1"],
        tokens: [cairntraceIngest],
      },
      "chalupa-cli": {
        kindSchemaBindings: [
          {
            kind: "chalupa.inference-receipt",
            maxSizeBytes: inferenceReceiptMaxSizeBytes,
            nativeSchema: "urn:chalupa:inference-receipt:v1",
          },
          {
            kind: "chalupa.agent-session",
            maxSizeBytes: agentSessionMaxSizeBytes,
            nativeSchema: "urn:chalupa:agent-session:v1",
          },
        ],
        maxSizeBytes: agentSessionMaxSizeBytes,
        tokens: [chalupaCliIngest],
      },
    });
    process.env.FILECHEAP_ADMIN_TOKEN = "a".repeat(32);
    process.env.FILECHEAP_OWNER_ACCOUNT_ID = "acc_owner123";
    process.env.CRON_SECRET = "z".repeat(32);
    resetConfigForTests();

    const receiptBytes = new TextEncoder().encode(JSON.stringify({
      model: { name: "qwen3-coder:30b", num_ctx: 32768 },
      run_id: "chalupa-run-1",
      schema: "local-agent.turn-receipt.v1",
    }));
    const sha256 = createHash("sha256").update(receiptBytes).digest("hex");
    const expiresAt = new Date(Date.now() + 7 * 24 * 60 * 60 * 1000).toISOString();
    const receiptPlanInput = {
      contentType: "application/json",
      expiresAt,
      idempotencyKey: "123e4567-e89b-42d3-a456-4266141740c1",
      kind: "chalupa.inference-receipt",
      producer: {
        native_id: "chalupa-run-1",
        native_schema: "urn:chalupa:inference-receipt:v1",
        tool: "chalupa-cli",
      },
      sha256,
      sizeBytes: receiptBytes.byteLength,
    };
    const store = new InMemoryArtifactObjectStore();
    setArtifactServiceForTests(new ArtifactService(store, new InMemoryArtifactRepository(), testPlanReceiptKeyring));
    const post = (path: string, token: string, body: unknown) =>
      new Request(`https://file.cheap/api/v1/artifacts/${path}`, {
        body: JSON.stringify(body),
        headers: { authorization: `Bearer ${token}`, "content-type": "application/json" },
        method: "POST",
      });

    // The per-kind quota is the receipt cap, not the producer's session cap.
    const oversizedReceipt = await plan(post("plans", chalupaCliIngest, { ...receiptPlanInput, sizeBytes: inferenceReceiptMaxSizeBytes + 1 }));
    expect(oversizedReceipt.status).toBe(413);
    const oversizedBody = await oversizedReceipt.json();
    expect(oversizedBody.code).toBe("producer_quota_exceeded");
    expect(oversizedBody.detail).toContain(`producer 'chalupa-cli' allows up to ${inferenceReceiptMaxSizeBytes} bytes for kind 'chalupa.inference-receipt'`);
    // The same size is inside the larger session-transcript kind.
    const sessionPlan = await plan(post("plans", chalupaCliIngest, {
      ...receiptPlanInput,
      idempotencyKey: "123e4567-e89b-42d3-a456-4266141740c2",
      kind: "chalupa.agent-session",
      producer: { ...receiptPlanInput.producer, native_schema: "urn:chalupa:agent-session:v1" },
      sizeBytes: inferenceReceiptMaxSizeBytes + 1,
    }));
    expect(sessionPlan.status).toBe(201);
    // A kind and native schema from different bindings is never authorized.
    const crossedBinding = await plan(post("plans", chalupaCliIngest, {
      ...receiptPlanInput,
      idempotencyKey: "123e4567-e89b-42d3-a456-4266141740c3",
      producer: { ...receiptPlanInput.producer, native_schema: "urn:chalupa:agent-session:v1" },
    }));
    expect(crossedBinding.status).toBe(401);

    const planned = await plan(post("plans", chalupaCliIngest, receiptPlanInput));
    expect(planned.status).toBe(201);
    const plannedBody = await planned.json() as { artifact: { expiresAt: string }; artifactRef: unknown; receipt: string; upload: { headers: Record<string, string>; method: string; url: string } };
    expect(plannedBody.upload.method).toBe("PUT");
    expect(plannedBody.upload.headers["content-type"]).toBe("application/json");
    expect(plannedBody.artifact.expiresAt).toBe(expiresAt);
    expect(plannedBody.artifactRef).toEqual({
      $schema: "urn:filecheap.dev:artifact-ref:v1",
      artifact_id: "art_123e4567e89b42d3a4564266141740c1",
      kind: "chalupa.inference-receipt",
      producer: receiptPlanInput.producer,
      provider: "fcheap-cloud",
      uri: "fcheap://cloud/vaults/private/artifacts/art_123e4567e89b42d3a4564266141740c1",
      version: 1,
    });

    store.seed({ bytes: receiptBytes, contentType: "application/json", key: new URL(plannedBody.upload.url).pathname.replace(/^\/upload\//, ""), sizeBytes: receiptBytes.byteLength });
    const committed = await commit(post("commits", chalupaCliIngest, { receipt: plannedBody.receipt }));
    expect(committed.status).toBe(200);
    const committedBody = await committed.json();
    expect(committedBody.artifact.verification).toBe("server-sha256");
    const artifactId = committedBody.artifact.artifactId;

    const ownDownload = await download(post("downloads", chalupaCliIngest, { artifactId }));
    expect(ownDownload.status).toBe(201);
    expect((await ownDownload.json()).download.method).toBe("GET");
    const otherProducerDownload = await download(post("downloads", cairntraceIngest, { artifactId }));
    expect(otherProducerDownload.status).toBe(404);
    expect((await otherProducerDownload.json()).code).toBe("artifact_not_found");

    // And the reverse: chalupa-cli cannot read another producer's artifact.
    const cairntraceBytes = new TextEncoder().encode("cairntrace run archive");
    const cairntracePlan = await plan(post("plans", cairntraceIngest, {
      contentType: "application/zstd",
      idempotencyKey: "123e4567-e89b-42d3-a456-4266141740c4",
      kind: "cairntrace.run",
      producer: { native_schema: "urn:cairntrace.dev:run:v1", tool: "cairntrace" },
      sha256: createHash("sha256").update(cairntraceBytes).digest("hex"),
      sizeBytes: cairntraceBytes.byteLength,
    }));
    expect(cairntracePlan.status).toBe(201);
    const cairntraceBody = await cairntracePlan.json() as { receipt: string; upload: { url: string } };
    store.seed({ bytes: cairntraceBytes, contentType: "application/zstd", key: new URL(cairntraceBody.upload.url).pathname.replace(/^\/upload\//, ""), sizeBytes: cairntraceBytes.byteLength });
    const cairntraceCommitted = await commit(post("commits", cairntraceIngest, { receipt: cairntraceBody.receipt }));
    expect(cairntraceCommitted.status).toBe(200);
    const foreignDownload = await download(post("downloads", chalupaCliIngest, { artifactId: (await cairntraceCommitted.json()).artifact.artifactId }));
    expect(foreignDownload.status).toBe(404);
  });
});
