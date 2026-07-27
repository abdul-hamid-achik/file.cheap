import { describe, expect, test } from "bun:test";

import {
  handleAuthorizationDecisionRequest,
  type AuthorizationDecisionRouteDependencies,
} from "@/app/api/console/auth/decisions/route";

describe("authorization decision route", () => {
  test("uses one canonical rate-limit bucket for equivalent user-code variants", async () => {
    const keys: string[] = [];
    const dependencies: AuthorizationDecisionRouteDependencies = {
      decide: async () => null,
      enforceRateLimit: async (input) => {
        keys.push(input.key);
      },
      setSession: async () => undefined,
    };

    for (const userCode of ["ABCD-EFGH", "abcdefgh", "AB.CD.EFGH"]) {
      const response = await handleAuthorizationDecisionRequest(
        decisionRequest(userCode),
        dependencies,
      );
      expect(response.status).toBe(200);
    }

    expect(keys).toEqual(["ABCD-EFGH", "ABCD-EFGH", "ABCD-EFGH"]);
  });
});

function decisionRequest(userCode: string): Request {
  return new Request("https://file.cheap/api/console/auth/decisions", {
    body: JSON.stringify({
      decision: "deny",
      email: "owner@example.com",
      otp: "123456",
      userCode,
    }),
    headers: {
      "content-type": "application/json",
      origin: "https://file.cheap",
      "sec-fetch-site": "same-origin",
    },
    method: "POST",
  });
}
