import { PlanReceiptKeyring } from "@/features/artifacts/plan-receipts";

export const testPlanReceiptKeyring = new PlanReceiptKeyring({
  activeKid: "test-current",
  keys: [
    {
      kid: "test-current",
      lookupKey: Buffer.alloc(32, 0x52),
      signingKey: Buffer.alloc(32, 0x51),
    },
  ],
});
