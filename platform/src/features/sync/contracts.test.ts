import { describe, expect, test } from "bun:test";

import {
  commitPlanSchema,
  createDownloadSchema,
  createPlanSchema,
  stashContentType,
} from "@/features/sync/contracts";

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
});
