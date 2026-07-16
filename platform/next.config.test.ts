import { describe, expect, test } from "bun:test";

import nextConfig, { platformSecurityHeaders } from "./next.config";

describe("platform security headers", () => {
  test("applies the browser policy to every route", async () => {
    expect(nextConfig.headers).toBeFunction();
    const rules = await nextConfig.headers!();

    expect(rules).toHaveLength(1);
    expect(rules[0]).toEqual({
      headers: [...platformSecurityHeaders],
      source: "/:path*",
    });
  });

  test("locks framing and active content while allowing signed Blob transfers", () => {
    const headers = new Map(
      platformSecurityHeaders.map(({ key, value }) => [key, value]),
    );
    const policy = headers.get("Content-Security-Policy") ?? "";

    expect(policy).toContain("frame-ancestors 'none'");
    expect(policy).toContain("object-src 'none'");
    expect(policy).toContain(
      "connect-src 'self' https://vercel.com https://*.private.blob.vercel-storage.com",
    );
    expect(headers.get("Permissions-Policy")).toContain("payment=()");
    expect(headers.get("Referrer-Policy")).toBe("no-referrer");
    expect(headers.get("X-Content-Type-Options")).toBe("nosniff");
    expect(headers.get("X-Frame-Options")).toBe("DENY");
  });
});
