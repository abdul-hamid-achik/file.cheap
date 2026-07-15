import { describe, expect, test } from "bun:test";

import {
  createRecoveryCard,
  createRecoveryDrillReport,
  parseRecoveryCard,
  parseRecoveryDrillReport,
  recoveryCardSchema,
  recoveryDrillReportSchema,
  recoveryCardIdentity,
  sanitizeRecoveryFileName,
  serializeRecoveryCard,
  serializeRecoveryDrillReport,
} from "@/features/sync/recovery-artifacts";

const sha256 = "a".repeat(64);

describe("RecoveryCard", () => {
  test("round trips through its serialized artifact contract", () => {
    const card = createRecoveryCard({
      committedAt: "2026-07-15T22:15:00.000Z",
      originalFileName: "investigation-01.fcheap",
      sha256,
      sizeBytes: 1024,
      stashId: "investigation-01",
    });

    expect(card).toEqual({
      committedAt: "2026-07-15T22:15:00.000Z",
      originalFileName: "investigation-01.fcheap",
      protocolVersion: "filecheap-sync/1",
      schema: "filecheap.recovery-card.v1",
      sha256,
      sizeBytes: 1024,
      stashId: "investigation-01",
    });
    expect(parseRecoveryCard(serializeRecoveryCard(card))).toEqual(card);
    expect(parseRecoveryCard(card)).toEqual(card);
    expect(recoveryCardIdentity(card)).toBe(serializeRecoveryCard(card));
  });

  test("sanitizes traversal and platform-specific filename separators", () => {
    const card = createRecoveryCard({
      committedAt: "2026-07-15T22:15:00.000Z",
      originalFileName: "../../private\\vault/archive.fcheap",
      sha256,
      sizeBytes: 1024,
      stashId: "investigation-01",
    });

    expect(card.originalFileName).toBe("archive.fcheap");
    expect(sanitizeRecoveryFileName("../../")).toBe("recovered.fcheap");
    expect(sanitizeRecoveryFileName("CON.txt")).toBe("_CON.txt");
  });

  test("produces normalized portable UTF-8 basenames without bidi controls", () => {
    const normalized = sanitizeRecoveryFileName(
      `${"e\u0301".repeat(200)}\u202ereport.fcheap`,
    );
    const boundary = sanitizeRecoveryFileName(`${"a".repeat(254)}😀`);
    const invalidUnicode = sanitizeRecoveryFileName("safe\ud83d.fcheap");

    expect(normalized).toBe(normalized.normalize("NFC"));
    expect(normalized).not.toContain("\u202e");
    expect(new TextEncoder().encode(normalized).byteLength).toBeLessThanOrEqual(
      255,
    );
    expect(boundary).toBe("a".repeat(254));
    expect(invalidUnicode).toBe("safe-.fcheap");
  });

  test("rejects incompatible or structurally manipulated cards", () => {
    const card = createRecoveryCard({
      committedAt: "2026-07-15T22:15:00.000Z",
      originalFileName: "investigation-01.fcheap",
      sha256,
      sizeBytes: 1024,
      stashId: "investigation-01",
    });

    expect(() =>
      parseRecoveryCard({ ...card, schema: "filecheap.recovery-card.v2" }),
    ).toThrow();
    expect(() =>
      parseRecoveryCard({ ...card, sha256: "tampered" }),
    ).toThrow();
    expect(() =>
      recoveryCardSchema.parse({
        ...card,
        originalFileName: "../../archive.fcheap",
      }),
    ).toThrow();
  });
});

describe("RecoveryDrillReport", () => {
  test("creates and round trips a verified report", () => {
    const report = createRecoveryDrillReport({
      attemptId: "a9d32f0b-c36b-4f3a-bd8e-9ec795617c0c",
      completedAt: "2026-07-15T22:16:00.000Z",
      recoveryCard: createRecoveryCard({
        committedAt: "2026-07-15T22:14:00.000Z",
        originalFileName: "investigation-01.fcheap",
        sha256,
        sizeBytes: 1024,
        stashId: "investigation-01",
      }),
      sha256,
      sizeBytes: 1024,
      startedAt: "2026-07-15T22:15:00.000Z",
      stashId: "investigation-01",
    });

    expect(report).toMatchObject({
      checks: {
        download: "passed",
        selectedFileByteEquivalent: "passed",
      },
      evidenceType: "local-client-observation",
      result: "verified",
      schema: "filecheap.recovery-drill-report.v1",
      tamperEvident: false,
    });
    expect(report.durationMilliseconds).toBe(60_000);
    expect(
      parseRecoveryDrillReport(serializeRecoveryDrillReport(report)),
    ).toEqual(report);
    expect(recoveryDrillReportSchema.parse(report)).toEqual(report);
  });

  test("rejects a negative duration or an incomplete verification", () => {
    const base = {
      attemptId: "a9d32f0b-c36b-4f3a-bd8e-9ec795617c0c",
      completedAt: "2026-07-15T22:16:00.000Z",
      recoveryCard: createRecoveryCard({
        committedAt: "2026-07-15T22:14:00.000Z",
        originalFileName: "investigation-01.fcheap",
        sha256,
        sizeBytes: 1024,
        stashId: "investigation-01",
      }),
      sha256,
      sizeBytes: 1024,
      startedAt: "2026-07-15T22:15:00.000Z",
      stashId: "investigation-01",
    };

    expect(() =>
      createRecoveryDrillReport({
        ...base,
        completedAt: "2026-07-15T22:14:59.999Z",
      }),
    ).toThrow();
    expect(() =>
      recoveryDrillReportSchema.parse({
        ...createRecoveryDrillReport(base),
        durationMilliseconds: -1,
      }),
    ).toThrow();
    expect(() =>
      recoveryDrillReportSchema.parse({
        ...createRecoveryDrillReport(base),
        stashId: "different-stash",
      }),
    ).toThrow();
    expect(() =>
      recoveryDrillReportSchema.parse({
        ...createRecoveryDrillReport(base),
        checks: { download: "passed", selectedFileByteEquivalent: "failed" },
      }),
    ).toThrow();
    expect(() =>
      parseRecoveryDrillReport({
        ...createRecoveryDrillReport(base),
        durationMilliseconds: 1,
      }),
    ).toThrow("durationMilliseconds");
    expect(() =>
      parseRecoveryDrillReport({
        ...createRecoveryDrillReport(base),
        completedAt: "2026-07-15T22:14:00.000Z",
        durationMilliseconds: 0,
      }),
    ).toThrow("completedAt");
  });
});
