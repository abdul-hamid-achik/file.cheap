import { createHash } from "node:crypto";
import { describe, expect, test } from "bun:test";

import { artifactSummarySchema } from "@/features/artifacts/contracts";
import { testPlanReceiptKeyring } from "@/features/artifacts/plan-receipts.test-helper";
import { InMemoryArtifactRepository } from "@/features/artifacts/repository";
import { ArtifactService } from "@/features/artifacts/service";
import { artifactWebUrl, consoleArtifactRedirect } from "@/features/artifacts/web-url";
import { InMemoryArtifactObjectStore } from "@/platform/artifacts/in-memory-object-store";

const bytes = new TextEncoder().encode("web-url-test");
const sha256 = createHash("sha256").update(bytes).digest("hex");
const artifactId = "art_0123456789abcdef";

describe("artifact web_url", () => {
  test("is a path-only https console link on the configured origin", () => {
    expect(artifactWebUrl("https://file.cheap", artifactId)).toBe("https://file.cheap/console/artifacts/art_0123456789abcdef");
    expect(artifactWebUrl("https://file.cheap/", artifactId)).toBe("https://file.cheap/console/artifacts/art_0123456789abcdef");
  });

  test("is omitted when the origin is missing, non-https, or credentialed", () => {
    expect(artifactWebUrl(undefined, artifactId)).toBeUndefined();
    expect(artifactWebUrl("http://127.0.0.1:3100", artifactId)).toBeUndefined();
    expect(artifactWebUrl("https://user:pw@file.cheap", artifactId)).toBeUndefined();
    expect(artifactWebUrl("not a url", artifactId)).toBeUndefined();
  });

  test("redirect targets stay inside the console and select exactly one drawer", () => {
    expect(consoleArtifactRedirect(artifactId, true)).toBe(`/console/runs?q=${artifactId}&run=${artifactId}`);
    expect(consoleArtifactRedirect(artifactId, false)).toBe(`/console?q=${artifactId}&artifact=${artifactId}`);
  });

  test("the service emits web_url on plan, commit, get, list, and download refs", async () => {
    const store = new InMemoryArtifactObjectStore();
    const service = new ArtifactService(store, new InMemoryArtifactRepository(), testPlanReceiptKeyring, undefined, "https://file.cheap");
    const planned = await service.plan({ contentType: "application/gzip", idempotencyKey: "123e4567-e89b-42d3-a456-426614174077", kind: "cairntrace.run", producer: { native_id: "run-1", native_schema: "urn:cairntrace.dev:run:v1", tool: "cairntrace" }, sha256, sizeBytes: bytes.byteLength });
    if (!("receipt" in planned)) throw new Error("expected a planned artifact");
    const expected = `https://file.cheap/console/artifacts/${planned.artifact.artifactId}`;
    expect(planned.artifactRef.web_url).toBe(expected);
    store.seed({ bytes, contentType: "application/gzip", key: new URL(planned.upload.url).pathname.replace(/^\/upload\//, ""), sizeBytes: bytes.byteLength });
    const committed = await service.commit(planned.receipt);
    expect(committed.artifactRef.web_url).toBe(expected);
    expect((await service.get(planned.artifact.artifactId)).artifactRef.web_url).toBe(expected);
    expect((await service.list({ limit: 10 })).artifacts[0]?.artifactRef.web_url).toBe(expected);
    expect((await service.download({ artifactId: planned.artifact.artifactId })).artifactRef.web_url).toBe(expected);
    const parsed = artifactSummarySchema.parse(committed);
    expect(parsed.artifactRef.web_url).not.toMatch(/[?#]/u);
  });

  test("the service omits web_url on a loopback http origin and the schema rejects signed links", async () => {
    const store = new InMemoryArtifactObjectStore();
    const service = new ArtifactService(store, new InMemoryArtifactRepository(), testPlanReceiptKeyring, undefined, "http://127.0.0.1:3100");
    const planned = await service.plan({ contentType: "application/gzip", idempotencyKey: "123e4567-e89b-42d3-a456-426614174078", kind: "cairntrace.run", producer: { tool: "cairntrace" }, sha256, sizeBytes: bytes.byteLength });
    expect("web_url" in planned.artifactRef).toBe(false);
    const ref = { ...planned.artifactRef, web_url: "https://file.cheap/console/artifacts/x?token=secret" };
    expect(() => artifactSummarySchema.parse({ artifact: planned.artifact, artifactRef: ref })).toThrow();
  });
});
