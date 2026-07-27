import { describe, expect, test } from "bun:test";

import { getPlanReceiptKeyring } from "@/features/artifacts/plan-receipt-config";
import {
  issuePlanReceipt,
  PlanReceiptConfigurationError,
} from "@/features/artifacts/plan-receipts";

const artifactId = "art_123e4567e89b42d3a456426614174000";

describe("private plan receipt configuration", () => {
  test("requires all three variables without a cryptographic default", () => {
    expect(() => getPlanReceiptKeyring({})).toThrow(
      "FILECHEAP_PLAN_RECEIPT_ACTIVE_KID",
    );
    expect(() =>
      getPlanReceiptKeyring({
        FILECHEAP_PLAN_RECEIPT_ACTIVE_KID: "current",
      }),
    ).toThrow("FILECHEAP_PLAN_RECEIPT_SIGNING_KEYS");
  });

  test("loads separate canonical signing and lookup maps with rotation", () => {
    const keyring = getPlanReceiptKeyring(environment());
    const issued = issuePlanReceipt(keyring, artifactId);

    expect(keyring.activeKid).toBe("current");
    expect(keyring.kids).toEqual(["current", "old"]);
    expect(issued.receiptKid).toBe("current");
    expect(issued.receipt).toMatch(
      /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/u,
    );
  });

  test("rejects malformed, oversized, or mismatched key maps", () => {
    expect(() =>
      getPlanReceiptKeyring({
        ...environment(),
        FILECHEAP_PLAN_RECEIPT_SIGNING_KEYS: "not-json",
      }),
    ).toThrow("JSON object");
    expect(() =>
      getPlanReceiptKeyring({
        ...environment(),
        FILECHEAP_PLAN_RECEIPT_SIGNING_KEYS: JSON.stringify([]),
      }),
    ).toThrow("JSON object");
    expect(() =>
      getPlanReceiptKeyring({
        ...environment(),
        FILECHEAP_PLAN_RECEIPT_SIGNING_KEYS: "x".repeat(8_193),
      }),
    ).toThrow("maximum encoded size");
    expect(() =>
      getPlanReceiptKeyring({
        ...environment(),
        FILECHEAP_PLAN_RECEIPT_LOOKUP_KEYS: JSON.stringify({
          different: encodedKey(9),
        }),
      }),
    ).toThrow("exactly the same key IDs");
  });

  test("rejects non-canonical, weak, and reused key material without echoing it", () => {
    const weak = Buffer.alloc(16, 7).toString("base64url");
    let error: unknown;
    try {
      getPlanReceiptKeyring({
        ...environment(),
        FILECHEAP_PLAN_RECEIPT_SIGNING_KEYS: JSON.stringify({
          current: weak,
          old: encodedKey(3),
        }),
      });
    } catch (caught) {
      error = caught;
    }
    expect(error).toBeInstanceOf(Error);
    expect(String(error)).not.toContain(weak);

    expect(() =>
      getPlanReceiptKeyring({
        ...environment(),
        FILECHEAP_PLAN_RECEIPT_SIGNING_KEYS: JSON.stringify({
          current: `${encodedKey(1)}=`,
          old: encodedKey(3),
        }),
      }),
    ).toThrow("canonical base64url");
    expect(() =>
      getPlanReceiptKeyring({
        ...environment(),
        FILECHEAP_PLAN_RECEIPT_LOOKUP_KEYS:
          environment().FILECHEAP_PLAN_RECEIPT_SIGNING_KEYS,
      }),
    ).toThrow(PlanReceiptConfigurationError);
  });

  test("requires the active kid to be present", () => {
    expect(() =>
      getPlanReceiptKeyring({
        ...environment(),
        FILECHEAP_PLAN_RECEIPT_ACTIVE_KID: "missing",
      }),
    ).toThrow(PlanReceiptConfigurationError);
  });
});

function environment(): Readonly<Record<string, string>> {
  return {
    FILECHEAP_PLAN_RECEIPT_ACTIVE_KID: "current",
    FILECHEAP_PLAN_RECEIPT_LOOKUP_KEYS: JSON.stringify({
      current: encodedKey(2),
      old: encodedKey(4),
    }),
    FILECHEAP_PLAN_RECEIPT_SIGNING_KEYS: JSON.stringify({
      current: encodedKey(1),
      old: encodedKey(3),
    }),
  };
}

function encodedKey(byte: number): string {
  return Buffer.alloc(32, byte).toString("base64url");
}
