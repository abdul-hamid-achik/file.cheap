import { describe, expect, test } from "bun:test";

import {
  commitPlanResponseSchema,
  commitPlanSchema,
  createDownloadSchema,
  createPlanSchema,
  downloadPlanSchema,
  protocolV1MaximumCatalogEntries,
  protocolV1MaxObjectBytes,
  stashListSchema,
  stashContentType,
  syncPlanSchema,
} from "@/features/sync/contracts";
import { controlPlaneResponseLimitBytes } from "@/features/sync/response-contract";

describe("sync request contracts", () => {
  test("defaults and restricts the protocol-v1 stash media type", () => {
    const base = {
      sha256: "a".repeat(64),
      sizeBytes: 1,
      stashId: "archive-01",
    };

    expect(createPlanSchema.parse(base).contentType).toBe(stashContentType);
    expect(() =>
      createPlanSchema.parse({ ...base, contentType: "application/octet-stream" }),
    ).toThrow();
  });

  test("rejects unknown properties instead of silently stripping them", () => {
    expect(() =>
      createPlanSchema.parse({
        extra: true,
        sha256: "a".repeat(64),
        sizeBytes: 1,
        stashId: "archive-01",
      }),
    ).toThrow();
    expect(() =>
      commitPlanSchema.parse({ extra: true, receipt: "receipt" }),
    ).toThrow();
    expect(() =>
      createDownloadSchema.parse({ extra: true, stashId: "archive-01" }),
    ).toThrow();
  });

  test("bounds signed commit receipts before verification", () => {
    expect(commitPlanSchema.parse({ receipt: "receipt" })).toEqual({
      receipt: "receipt",
    });
    expect(() =>
      commitPlanSchema.parse({ receipt: "r".repeat(4_097) }),
    ).toThrow();
  });

  test("keeps protocol v1 within the non-multipart laboratory limit", () => {
    const base = {
      sha256: "a".repeat(64),
      stashId: "archive-01",
    };

    expect(
      createPlanSchema.parse({ ...base, sizeBytes: protocolV1MaxObjectBytes })
        .sizeBytes,
    ).toBe(protocolV1MaxObjectBytes);
    expect(() =>
      createPlanSchema.parse({
        ...base,
        sizeBytes: protocolV1MaxObjectBytes + 1,
      }),
    ).toThrow();
  });

  test("strictly validates every control-plane success response", () => {
    const sha256 = "b".repeat(64);
    const stash = {
      committedAt: "2026-07-15T22:15:00.000Z",
      contentType: stashContentType,
      sha256,
      sizeBytes: 4,
      stashId: "archive-01",
      storageVerification: "server-sha256" as const,
    };

    expect(
      commitPlanResponseSchema.parse({
        requiresFullVerification: true,
        stash,
        version: "filecheap-sync/1",
      }).stash,
    ).toEqual(stash);
    expect(
      stashListSchema.parse({
        stashes: [stash],
        version: "filecheap-sync/1",
      }).stashes,
    ).toEqual([stash]);
    expect(() =>
      stashListSchema.parse({
        stashes: [stash],
        unexpected: true,
        version: "filecheap-sync/1",
      }),
    ).toThrow();
    expect(() =>
      commitPlanResponseSchema.parse({
        requiresFullVerification: false,
        stash,
        version: "filecheap-sync/1",
      }),
    ).toThrow();

    expect(() =>
      stashListSchema.parse({
        stashes: [stash, { ...stash, sha256: "c".repeat(64) }],
        version: "filecheap-sync/1",
      }),
    ).toThrow();
  });

  test("keeps the largest protocol-v1 catalog response inside the browser limit", () => {
    const stashes = Array.from(
      { length: protocolV1MaximumCatalogEntries },
      (_, index) => ({
        committedAt: "2026-07-15T22:15:00.000Z",
        contentType: stashContentType,
        sha256: "d".repeat(64),
        sizeBytes: protocolV1MaxObjectBytes,
        stashId: `stash-${index.toString().padStart(4, "0")}-${"x".repeat(117)}`,
        storageVerification: "presence-size-etag" as const,
      }),
    );
    const catalog = stashListSchema.parse({
      stashes,
      version: "filecheap-sync/1",
    });

    expect(new TextEncoder().encode(JSON.stringify(catalog)).byteLength).toBeLessThan(
      controlPlaneResponseLimitBytes,
    );
    expect(() =>
      stashListSchema.parse({
        stashes: [...stashes, { ...stashes[0], stashId: "overflow" }],
        version: "filecheap-sync/1",
      }),
    ).toThrow();
  });

  test("enforces response grant direction and plan-state invariants", () => {
    const sha256 = "c".repeat(64);
    const object = {
      key: `v1/objects/${sha256}.fcheap`,
      sha256,
      sizeBytes: 4,
    };
    const upload = {
      expiresAt: "2026-07-15T22:15:00.000Z",
      headers: { "content-type": stashContentType },
      method: "PUT" as const,
      url: "https://example.com/upload",
    };

    expect(
      syncPlanSchema.parse({
        object,
        receipt: "receipt",
        state: "upload_required",
        upload,
        version: "filecheap-sync/1",
      }).upload,
    ).toEqual(upload);
    expect(() =>
      syncPlanSchema.parse({
        object,
        receipt: "receipt",
        state: "object_present",
        upload,
        version: "filecheap-sync/1",
      }),
    ).toThrow();
    expect(() =>
      syncPlanSchema.parse({
        object,
        receipt: "receipt",
        state: "upload_required",
        upload: { ...upload, method: "GET" },
        version: "filecheap-sync/1",
      }),
    ).toThrow();

    expect(
      downloadPlanSchema.parse({
        expected: { sha256, sizeBytes: 4 },
        grant: { ...upload, headers: {}, method: "GET" },
        mustVerifySha256: true,
        stashId: "archive-01",
        version: "filecheap-sync/1",
      }).grant.method,
    ).toBe("GET");
    expect(() =>
      downloadPlanSchema.parse({
        expected: { sha256, sizeBytes: 4 },
        grant: upload,
        mustVerifySha256: true,
        stashId: "archive-01",
        version: "filecheap-sync/1",
      }),
    ).toThrow();

    for (const url of [
      "file:///tmp/archive",
      "javascript:alert(1)",
      "http://example.com/upload",
      "https://user:secret@example.com/upload",
      "https://example.com/upload#fragment",
    ]) {
      expect(() =>
        syncPlanSchema.parse({
          object,
          receipt: "receipt",
          state: "upload_required",
          upload: { ...upload, url },
          version: "filecheap-sync/1",
        }),
      ).toThrow();
    }
    expect(
      syncPlanSchema.parse({
        object,
        receipt: "receipt",
        state: "upload_required",
        upload: { ...upload, url: "http://127.0.0.1:3100/upload" },
        version: "filecheap-sync/1",
      }).upload?.url,
    ).toBe("http://127.0.0.1:3100/upload");
  });
});
