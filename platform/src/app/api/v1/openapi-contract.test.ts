import { access } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { join } from "node:path";

import { describe, expect, test } from "bun:test";

import document from "../../../../openapi.json";
import { artifactListQuerySchema, artifactPlanInputSchema, artifactPlanReplayResponseSchema, artifactPlanResponseSchema, artifactSummarySchema } from "@/features/artifacts/contracts";

const routes = {
  "/api/v1/health": "src/app/api/v1/health/route.ts",
  "/api/v1/openapi.json": "src/app/api/v1/openapi.json/route.ts",
  "/api/v1/artifacts": "src/app/api/v1/artifacts/route.ts",
  "/api/v1/artifacts/{artifactId}": "src/app/api/v1/artifacts/[artifactId]/route.ts",
  "/api/v1/artifacts/plans": "src/app/api/v1/artifacts/plans/route.ts",
  "/api/v1/artifacts/commits": "src/app/api/v1/artifacts/commits/route.ts",
  "/api/v1/artifacts/downloads": "src/app/api/v1/artifacts/downloads/route.ts",
  "/api/internal/retention": "src/app/api/internal/retention/route.ts",
} as const;

const artifactRef = { $schema: "urn:filecheap.dev:artifact-ref:v1", artifact_id: "art_abcdefghijklmnop", kind: "chalupa.log-chunk", producer: { native_schema: "urn:chalupa.dev:log:v1", tool: "chalupa" }, provider: "fcheap-cloud", uri: "fcheap://cloud/vaults/private/artifacts/art_abcdefghijklmnop", version: 1 };
const artifact = { artifactId: "art_abcdefghijklmnop", committedAt: "2026-07-24T00:00:00.000Z", contentType: "application/zstd", expiresAt: null, kind: "chalupa.log-chunk", producer: artifactRef.producer, sha256: "a".repeat(64), sizeBytes: 1024, state: "committed", verification: "server-sha256" as const };

describe("private artifact OpenAPI contract", () => {
  test("documents exactly the implemented route set and methods", async () => {
    expect(document.openapi).toBe("3.1.1");
    expect(Object.keys(document.paths).sort()).toEqual(Object.keys(routes).sort());
    for (const routeFile of Object.values(routes)) await access(join(fileURLToPath(new URL("../../../../", import.meta.url)), routeFile));
    expect(Object.keys(document.paths["/api/v1/artifacts/plans"])).toEqual(["post"]);
    expect(Object.keys(document.paths["/api/v1/artifacts"])).toEqual(["get"]);
    expect(Object.keys(document.paths["/api/internal/retention"])).toEqual(["get"]);
  });

  test("keeps plan, artifact, and pagination examples inside runtime Zod contracts", () => {
    const plan = artifactPlanInputSchema.parse({ contentType: "application/zstd", expiresAt: new Date(Date.now() + 60_000).toISOString(), idempotencyKey: "123e4567-e89b-42d3-a456-426614174000", kind: "chalupa.log-chunk", producer: artifactRef.producer, sha256: artifact.sha256, sizeBytes: artifact.sizeBytes });
    expect(plan.idempotencyKey).toBe("123e4567-e89b-42d3-a456-426614174000");
    expect(artifactPlanInputSchema.parse({ ...plan, idempotencyKey: "123E4567-E89B-42D3-A456-426614174000" }).idempotencyKey).toBe(plan.idempotencyKey);
    const summary = artifactSummarySchema.parse({ artifact, artifactRef });
    const plannedSummary = artifactSummarySchema.parse({ artifact: { ...artifact, committedAt: null, state: "planned" }, artifactRef });
    const response = artifactPlanResponseSchema.parse({ ...plannedSummary, receipt: "123e4567-e89b-42d3-a456-426614174001", upload: { expiresAt: new Date(Date.now() + 60_000).toISOString(), headers: { "content-type": "application/zstd" }, method: "PUT", url: "https://private.blob.example/upload?capability=opaque" } });
    expect(response.upload.method).toBe("PUT");
    expect(artifactPlanReplayResponseSchema.parse(summary).artifact.state).toBe("committed");
    expect(artifactListQuerySchema.parse({ after: artifact.artifactId, limit: 100 })).toEqual({ after: artifact.artifactId, limit: 100 });
  });

  test("documents strict idempotency, bounded bytes, verified states, RFC9457, and retry metadata", () => {
    const schemas = document.components.schemas;
    expect(schemas.ArtifactPlanInput.properties.idempotencyKey.format).toBe("uuid");
    expect(schemas.ArtifactPlanInput.properties.sizeBytes.maximum).toBe(2 * 1024 * 1024);
    expect(schemas.Artifact.properties.verification.const).toBe("server-sha256");
    expect(schemas.Artifact.properties.state.enum).toEqual(["planned", "committed", "deleting", "deleted"]);
    expect(schemas.Problem.required).toContain("requestId");
    expect(document.components.responses.ServiceUnavailable.headers["Retry-After"].$ref).toBe("#/components/headers/RetryAfter");
    expect(document.components.responses.Conflict.headers["Retry-After"].$ref).toBe("#/components/headers/RetryAfter");
    expect(document.paths["/api/v1/artifacts/plans"].post.responses["201"].content["application/json"].schema.$ref).toBe("#/components/schemas/ArtifactPlanResponse");
    expect(document.paths["/api/v1/artifacts/plans"].post.responses["200"].content["application/json"].schema.$ref).toBe("#/components/schemas/ArtifactPlanReplayResponse");
    expect(document.paths["/api/v1/artifacts/plans"].post.description).toContain("renews the exact immutable plan");
    expect(document.paths["/api/v1/artifacts/plans"].post.description).toContain("already-committed plan returns 200");
    expect(document.paths["/api/v1/artifacts/plans"].post.description).toContain("exact producer.tool, kind, and producer.native_schema");
    expect(document.paths["/api/v1/artifacts/plans"].post.description).toContain("capped at the artifact retention timestamp");
    expect(document.paths["/api/v1/artifacts/commits"].post.description).toContain("before reading private storage");
    expect(document.paths["/api/v1/artifacts/commits"].post.description).toContain("verification crosses that boundary");
    expect(document.components.securitySchemes.ingestOidcOrBearer.description).toContain("bound to one exact producer.tool");
    expect(document.paths["/api/v1/artifacts/downloads"].post.security).toEqual([
      { adminBearer: [] },
      { chalupaOidc: [] },
    ]);
    expect(document.paths["/api/v1/artifacts/downloads"].post.description).toContain("Publisher credentials are rejected");
    expect(document.paths["/api/internal/retention"].get.description).toContain("abandoned upload plans");
    expect(document.paths["/api/internal/retention"].get.description).toContain("continues with the remaining candidates");
    expect(schemas.ArtifactPlanResponse.additionalProperties).toBe(false);
    expect(schemas.UploadGrant.properties.method.const).toBe("PUT");
    expect(schemas.DownloadGrant.properties.method.const).toBe("GET");
  });

  test("keeps producer routing metadata credential-free and path-safe", () => {
    const base = {
      contentType: "application/zstd",
      idempotencyKey: "123e4567-e89b-42d3-a456-426614174099",
      kind: "chalupa.log-chunk",
      producer: { tool: "chalupa" },
      sha256: "a".repeat(64),
      sizeBytes: 1,
    };

    expect(
      artifactPlanInputSchema.safeParse({
        ...base,
        producer: {
          tool: "chalupa",
          native_schema: "https://user:password@example.test/schema",
        },
      }).success,
    ).toBe(false);
    expect(
      artifactPlanInputSchema.safeParse({
        ...base,
        producer: { tool: "chalupa", native_schema: "urn:example:schema?grant=secret" },
      }).success,
    ).toBe(false);
    expect(
      artifactPlanInputSchema.safeParse({
        ...base,
        producer: { tool: "chalupa", entrypoint: "../private.log" },
      }).success,
    ).toBe(false);
    expect(
      artifactPlanInputSchema.safeParse({
        ...base,
        contentType: "application/zstd\r\nx-unsafe: value",
      }).success,
    ).toBe(false);
  });
});
