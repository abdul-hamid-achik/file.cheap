import { afterEach, describe, expect, test } from "bun:test";

import { GET, PUT } from "@/app/api/v1/local-objects/route";

const originalLabEnabled = process.env.PLATFORM_RECOVERY_LAB_ENABLED;
const originalVercel = process.env.VERCEL;

afterEach(() => {
  restoreEnvironment(
    "PLATFORM_RECOVERY_LAB_ENABLED",
    originalLabEnabled,
  );
  restoreEnvironment("VERCEL", originalVercel);
});

describe("local object transfer Route Handler", () => {
  test("hides valid transfer methods when the recovery lab is disabled", async () => {
    process.env.VERCEL = "1";
    delete process.env.PLATFORM_RECOVERY_LAB_ENABLED;

    for (const response of [
      await GET(request("GET")),
      await PUT(request("PUT")),
    ]) {
      expect(response.status).toBe(404);
      expect(response.headers.get("content-type")).toContain(
        "application/problem+json",
      );
      expect(await response.json()).toMatchObject({
        code: "route_unavailable",
        status: 404,
      });
    }
  });
});

function request(method: "GET" | "PUT"): Request {
  return new Request("http://127.0.0.1:3100/api/v1/local-objects", {
    method,
  });
}

function restoreEnvironment(name: string, value: string | undefined): void {
  if (value === undefined) {
    delete process.env[name];
  } else {
    process.env[name] = value;
  }
}
