import { describe, expect, test } from "bun:test";

import { artifactCommitInputSchema } from "@/features/artifacts/contracts";
import {
  derivePlanReceiptLookup,
  derivePlanReceiptLookupCandidates,
  generatePlanReceiptNonce,
  issuePlanReceipt,
  parsePlanReceiptLookup,
  parsePlanReceiptNonce,
  PlanReceiptConfigurationError,
  PlanReceiptKeyring,
  reconstructPlanReceipt,
  verifyPlanReceipt,
  verifyPlanReceiptLookup,
  type PlanReceiptKeyInput,
} from "@/features/artifacts/plan-receipts";

const artifactId = "art_123e4567e89b42d3a456426614174000";
const otherArtifactId = "art_123e4567e89b42d3a456426614174001";
const knownNonce = parsePlanReceiptNonce(
  Buffer.from(Array.from({ length: 32 }, (_, index) => index)).toString(
    "base64url",
  ),
);

describe("plan receipt derivation", () => {
  test("keeps a stable UUID and lookup vector for the v1 domains", () => {
    const issued = issuePlanReceipt(knownKeyring(), artifactId, knownNonce);

    expect(issued).toEqual({
      receipt: "acb20cab-7793-418c-8abb-6e3910e349bc",
      receiptKid: "2026-07",
      receiptLookup: parsePlanReceiptLookup(
        "6oGxqnVvAzBgZunsITCbJOQyKtRcFjseD7U0Q4En5Hg",
      ),
      receiptNonce: knownNonce,
      receiptScheme: "hmac-sha256-v1",
    });
    expect(artifactCommitInputSchema.parse({ receipt: issued.receipt })).toEqual({
      receipt: issued.receipt,
    });
    expect(
      reconstructPlanReceipt(knownKeyring(), {
        artifactId,
        receiptKid: issued.receiptKid,
        receiptNonce: issued.receiptNonce,
      }),
    ).toBe(issued.receipt);
  });

  test("is deterministic for one artifact, nonce, and key", () => {
    const keyring = knownKeyring();
    const left = issuePlanReceipt(keyring, artifactId, knownNonce);
    const right = issuePlanReceipt(keyring, artifactId, knownNonce);

    expect(right).toEqual(left);
    expect(Object.isFrozen(left)).toBe(true);
  });

  test("binds the receipt independently to artifact, nonce, and signing key", () => {
    const keyring = knownKeyring();
    const original = issuePlanReceipt(keyring, artifactId, knownNonce);
    const otherNonce = parsePlanReceiptNonce(Buffer.alloc(32, 91).toString("base64url"));
    const otherKeyring = new PlanReceiptKeyring({
      activeKid: "2026-07",
      keys: [key("2026-07", 51, 52)],
    });

    expect(issuePlanReceipt(keyring, otherArtifactId, knownNonce).receipt).not.toBe(
      original.receipt,
    );
    expect(issuePlanReceipt(keyring, artifactId, otherNonce).receipt).not.toBe(
      original.receipt,
    );
    expect(issuePlanReceipt(otherKeyring, artifactId, knownNonce).receipt).not.toBe(
      original.receipt,
    );
  });

  test("generates independent canonical 256-bit nonces by default", () => {
    const firstNonce = generatePlanReceiptNonce();
    const secondNonce = generatePlanReceiptNonce();
    const first = issuePlanReceipt(knownKeyring(), artifactId);
    const second = issuePlanReceipt(knownKeyring(), artifactId);

    expect(firstNonce).toHaveLength(43);
    expect(parsePlanReceiptNonce(firstNonce)).toBe(firstNonce);
    expect(secondNonce).not.toBe(firstNonce);
    expect(second.receiptNonce).not.toBe(first.receiptNonce);
    expect(second.receipt).not.toBe(first.receipt);
  });

  test("uses separate per-key lookup HMACs and normalizes UUID case", () => {
    const keyring = rotatingKeyring();
    const issued = issuePlanReceipt(keyring, artifactId, knownNonce);
    const candidates = derivePlanReceiptLookupCandidates(keyring, issued.receipt);
    const uppercase = derivePlanReceiptLookupCandidates(
      keyring,
      issued.receipt.toUpperCase(),
    );

    expect(candidates.map((candidate) => candidate.receiptKid)).toEqual([
      "current",
      "old-a",
      "old-z",
    ]);
    expect(candidates).toEqual(uppercase);
    expect(new Set(candidates.map((candidate) => candidate.receiptLookup)).size).toBe(3);
    expect(candidates[0]?.receiptLookup).toBe(
      derivePlanReceiptLookup(keyring, "current", issued.receipt),
    );
    expect(candidates.every(Object.isFrozen)).toBe(true);
    expect(Object.isFrozen(candidates)).toBe(true);
  });

  test("reconstructs receipts issued with a retained rotation key", () => {
    const oldOnly = new PlanReceiptKeyring({
      activeKid: "old-a",
      keys: [key("old-a", 3, 4)],
    });
    const oldReceipt = issuePlanReceipt(oldOnly, artifactId, knownNonce);
    const rotated = rotatingKeyring();

    expect(
      reconstructPlanReceipt(rotated, {
        artifactId,
        receiptKid: oldReceipt.receiptKid,
        receiptNonce: oldReceipt.receiptNonce,
      }),
    ).toBe(oldReceipt.receipt);
    expect(
      derivePlanReceiptLookupCandidates(rotated, oldReceipt.receipt),
    ).toContainEqual({
      receiptKid: "old-a",
      receiptLookup: oldReceipt.receiptLookup,
    });
  });
});

describe("plan receipt verification", () => {
  test("requires both deterministic receipt and independent lookup bindings", () => {
    const keyring = knownKeyring();
    const issued = issuePlanReceipt(keyring, artifactId, knownNonce);
    const stored = {
      artifactId,
      receiptKid: issued.receiptKid,
      receiptLookup: issued.receiptLookup,
      receiptNonce: issued.receiptNonce,
    };

    expect(verifyPlanReceipt(keyring, issued.receipt, stored)).toBe(true);
    expect(
      verifyPlanReceipt(keyring, mutateUuid(issued.receipt), stored),
    ).toBe(false);
    expect(
      verifyPlanReceipt(keyring, issued.receipt, {
        ...stored,
        artifactId: otherArtifactId,
      }),
    ).toBe(false);
    expect(
      verifyPlanReceipt(keyring, issued.receipt, {
        ...stored,
        receiptNonce: Buffer.alloc(32, 99).toString("base64url"),
      }),
    ).toBe(false);
    expect(
      verifyPlanReceipt(keyring, issued.receipt, {
        ...stored,
        receiptLookup: Buffer.alloc(32, 100).toString("base64url"),
      }),
    ).toBe(false);
    expect(
      verifyPlanReceipt(keyring, issued.receipt, {
        ...stored,
        receiptKid: "retired",
      }),
    ).toBe(false);
  });

  test("verifies lookup-only material for a keyed legacy backfill", () => {
    const keyring = knownKeyring();
    const issued = issuePlanReceipt(keyring, artifactId, knownNonce);
    const stored = {
      receiptKid: issued.receiptKid,
      receiptLookup: issued.receiptLookup,
    };

    expect(verifyPlanReceiptLookup(keyring, issued.receipt, stored)).toBe(true);
    expect(
      verifyPlanReceiptLookup(keyring, mutateUuid(issued.receipt), stored),
    ).toBe(false);
    expect(
      verifyPlanReceiptLookup(keyring, issued.receipt, {
        ...stored,
        receiptLookup: "invalid",
      }),
    ).toBe(false);
    expect(
      verifyPlanReceiptLookup(keyring, issued.receipt, {
        ...stored,
        receiptKid: "retired",
      }),
    ).toBe(false);
  });

  test("fails closed for malformed receipt material", () => {
    const keyring = knownKeyring();
    const issued = issuePlanReceipt(keyring, artifactId, knownNonce);
    const stored = {
      artifactId,
      receiptKid: issued.receiptKid,
      receiptLookup: issued.receiptLookup,
      receiptNonce: issued.receiptNonce,
    };

    expect(verifyPlanReceipt(keyring, "not-a-uuid", stored)).toBe(false);
    expect(
      verifyPlanReceipt(keyring, issued.receipt, {
        ...stored,
        artifactId: "invalid",
      }),
    ).toBe(false);
    expect(
      verifyPlanReceipt(keyring, issued.receipt, {
        ...stored,
        receiptNonce: "invalid",
      }),
    ).toBe(false);
    expect(
      verifyPlanReceipt(keyring, issued.receipt, {
        ...stored,
        receiptLookup: "invalid",
      }),
    ).toBe(false);
  });
});

describe("plan receipt keyring validation", () => {
  test("copies key bytes and exposes only rotation metadata", () => {
    const signingKey = Buffer.alloc(32, 17);
    const lookupKey = Buffer.alloc(32, 34);
    const keyring = new PlanReceiptKeyring({
      activeKid: "2026-07",
      keys: [{ kid: "2026-07", lookupKey, signingKey }],
    });
    const before = issuePlanReceipt(keyring, artifactId, knownNonce);

    signingKey.fill(90);
    lookupKey.fill(91);

    expect(issuePlanReceipt(keyring, artifactId, knownNonce)).toEqual(before);
    expect(Object.keys(keyring).sort()).toEqual(["activeKid", "kids"]);
    expect(JSON.stringify(keyring)).not.toContain(Buffer.alloc(32, 17).toString("hex"));
    expect(Object.isFrozen(keyring)).toBe(true);
    expect(Object.isFrozen(keyring.kids)).toBe(true);
  });

  test("requires a bounded keyring with a loaded, safe active kid", () => {
    expectConfigurationError(
      () => new PlanReceiptKeyring({ activeKid: "current", keys: [] }),
      "keyring_size",
    );
    expectConfigurationError(
      () =>
        new PlanReceiptKeyring({
          activeKid: "current",
          keys: Array.from({ length: 17 }, (_, index) =>
            key(`key-${index}`, index * 2 + 1, index * 2 + 2),
          ),
        }),
      "keyring_size",
    );
    expectConfigurationError(
      () => new PlanReceiptKeyring({ activeKid: "bad kid", keys: [key("ok", 1, 2)] }),
      "invalid_active_key_id",
    );
    expectConfigurationError(
      () => new PlanReceiptKeyring({ activeKid: "missing", keys: [key("loaded", 1, 2)] }),
      "active_key_missing",
    );
    expectConfigurationError(
      () => new PlanReceiptKeyring({ activeKid: "current", keys: [key("bad/kid", 1, 2)] }),
      "invalid_key_id",
    );
  });

  test("rejects duplicate IDs and weak, oversized, or reused key material", () => {
    expectConfigurationError(
      () =>
        new PlanReceiptKeyring({
          activeKid: "current",
          keys: [key("current", 1, 2), key("current", 3, 4)],
        }),
      "duplicate_key_id",
    );
    expectConfigurationError(
      () =>
        new PlanReceiptKeyring({
          activeKid: "current",
          keys: [{ ...key("current", 1, 2), signingKey: Buffer.alloc(31, 1) }],
        }),
      "invalid_key_material",
    );
    expectConfigurationError(
      () =>
        new PlanReceiptKeyring({
          activeKid: "current",
          keys: [{ ...key("current", 1, 2), lookupKey: Buffer.alloc(65, 2) }],
        }),
      "invalid_key_material",
    );
    expectConfigurationError(
      () =>
        new PlanReceiptKeyring({
          activeKid: "current",
          keys: [{ kid: "current", lookupKey: Buffer.alloc(32, 1), signingKey: Buffer.alloc(32, 1) }],
        }),
      "key_material_reused",
    );
    expectConfigurationError(
      () =>
        new PlanReceiptKeyring({
          activeKid: "current",
          keys: [
            key("current", 1, 2),
            { ...key("old", 3, 4), signingKey: Buffer.alloc(32, 2) },
          ],
        }),
      "key_material_reused",
    );
  });

  test("throws typed errors for a key removed before reconstruction", () => {
    const keyring = knownKeyring();

    expect(() =>
      reconstructPlanReceipt(keyring, {
        artifactId,
        receiptKid: "retired",
        receiptNonce: knownNonce,
      }),
    ).toThrow(PlanReceiptConfigurationError);
  });
});

describe("persisted plan receipt encoding", () => {
  test("round-trips only canonical fixed-size base64url values", () => {
    const lookup = Buffer.alloc(32, 44).toString("base64url");

    expect(parsePlanReceiptNonce(knownNonce)).toBe(knownNonce);
    expect(String(parsePlanReceiptLookup(lookup))).toBe(lookup);
    for (const invalid of [
      "",
      "a".repeat(42),
      "a".repeat(44),
      `${knownNonce}=`,
      knownNonce.replace("A", "+"),
    ]) {
      expect(() => parsePlanReceiptNonce(invalid)).toThrow(TypeError);
    }
    expect(() => parsePlanReceiptLookup("short")).toThrow(TypeError);
  });

  test("rejects invalid artifact, nonce, receipt, and unknown lookup key", () => {
    const keyring = knownKeyring();
    const issued = issuePlanReceipt(keyring, artifactId, knownNonce);

    expect(() => issuePlanReceipt(keyring, "invalid", knownNonce)).toThrow(TypeError);
    expect(() =>
      reconstructPlanReceipt(keyring, {
        artifactId,
        receiptKid: issued.receiptKid,
        receiptNonce: "invalid",
      }),
    ).toThrow(TypeError);
    expect(() =>
      derivePlanReceiptLookupCandidates(keyring, "not-a-uuid"),
    ).toThrow(TypeError);
    expect(() =>
      derivePlanReceiptLookup(keyring, "retired", issued.receipt),
    ).toThrow(PlanReceiptConfigurationError);
  });
});

function knownKeyring(): PlanReceiptKeyring {
  return new PlanReceiptKeyring({
    activeKid: "2026-07",
    keys: [key("2026-07", 17, 34)],
  });
}

function rotatingKeyring(): PlanReceiptKeyring {
  return new PlanReceiptKeyring({
    activeKid: "current",
    keys: [
      key("old-z", 5, 6),
      key("current", 1, 2),
      key("old-a", 3, 4),
    ],
  });
}

function key(kid: string, signingByte: number, lookupByte: number): PlanReceiptKeyInput {
  return {
    kid,
    lookupKey: Buffer.alloc(32, lookupByte),
    signingKey: Buffer.alloc(32, signingByte),
  };
}

function mutateUuid(value: string): string {
  const last = value.at(-1);
  return `${value.slice(0, -1)}${last === "0" ? "1" : "0"}`;
}

function expectConfigurationError(
  callback: () => unknown,
  code: PlanReceiptConfigurationError["code"],
): void {
  try {
    callback();
    throw new Error("Expected plan receipt configuration to be rejected");
  } catch (error) {
    expect(error).toBeInstanceOf(PlanReceiptConfigurationError);
    expect(error).toMatchObject({ code });
  }
}
