import { AsyncLocalStorage } from "node:async_hooks";

import { describe, expect, test } from "bun:test";

import {
  DOCS_OUTPUT_PREFIX,
  DOCS_ROOT_ASSETS,
  DOCS_SECTIONS,
} from "./docs-routing";
import nextConfig, { platformSecurityHeaders } from "./next.config";

type ConfigTesting = typeof import("next/experimental/testing/server");

let configTesting: Promise<ConfigTesting> | undefined;

function loadConfigTesting() {
  (
    globalThis as typeof globalThis & {
      AsyncLocalStorage?: typeof AsyncLocalStorage;
    }
  ).AsyncLocalStorage ??= AsyncLocalStorage;
  configTesting ??= import("next/experimental/testing/server");
  return configTesting;
}

async function responseFor(url: string) {
  const { unstable_getResponseFromNextConfig } = await loadConfigTesting();
  return unstable_getResponseFromNextConfig({ nextConfig, url });
}

describe("platform security headers", () => {
  test("applies the browser policy to every route", async () => {
    expect(nextConfig.headers).toBeFunction();
    const rules = await nextConfig.headers!();

    expect(rules).toHaveLength(3);
    expect(rules[0]).toEqual({
      headers: [...platformSecurityHeaders],
      source: "/:path*",
    });
    expect(rules[1]).toEqual({
      headers: [{ key: "X-Robots-Tag", value: "noindex, nofollow" }],
      source: "/_docs/:path*",
    });
    expect(rules[2]).toEqual({
      headers: [
        {
          key: "Cache-Control",
          value: "public, max-age=31536000, immutable",
        },
      ],
      source: "/assets/:path*",
    });
  });

  test("locks framing and active content while allowing signed Blob transfers", () => {
    const headers = new Map(
      platformSecurityHeaders.map(({ key, value }) => [key, value]),
    );
    const policy = headers.get("Content-Security-Policy") ?? "";

    expect(policy).toContain("frame-ancestors 'none'");
    expect(policy).toContain("object-src 'none'");
    expect(policy).toContain("https://vercel.com");
    expect(policy).toContain("https://*.private.blob.vercel-storage.com");
    expect(policy).toContain("https://fonts.googleapis.com");
    expect(policy).toContain("https://fonts.gstatic.com");
    expect(policy).toContain(
      "font-src 'self' data: https://fonts.gstatic.com",
    );
    expect(policy).toContain(
      "style-src 'self' 'unsafe-inline' https://fonts.googleapis.com",
    );
    expect(headers.get("Permissions-Policy")).toContain("payment=()");
    expect(policy).toContain("worker-src 'self' blob:");
    expect(policy).toContain("upgrade-insecure-requests");
    expect(headers.get("Referrer-Policy")).toBe(
      "strict-origin-when-cross-origin",
    );
    expect(headers.get("Strict-Transport-Security")).toBe(
      "max-age=31536000",
    );
    expect(headers.get("X-Content-Type-Options")).toBe("nosniff");
    expect(headers.get("X-Frame-Options")).toBe("DENY");
  });
});

describe("documentation routing", () => {
  test("serves every documentation section from the staged build", async () => {
    const { getRewrittenUrl } = await loadConfigTesting();

    for (const section of DOCS_SECTIONS) {
      const response = await responseFor(
        `https://platform.example/${section}/reference?source=test`,
      );
      const rewritten = new URL(
        getRewrittenUrl(response)!,
        "https://platform.example",
      );
      expect(rewritten.hostname).toBe("platform.example");
      expect(rewritten.pathname).toBe(
        `${DOCS_OUTPUT_PREFIX}/${section}/reference.html`,
      );
      expect(rewritten.search).toBe("?source=test");
    }
  });

  test("maps both clean section-root forms to their generated index", async () => {
    const { getRewrittenUrl } = await loadConfigTesting();

    expect(
      new URL(
        getRewrittenUrl(
          await responseFor("https://platform.example/guide"),
        )!,
        "https://platform.example",
      ).pathname,
    ).toBe(`${DOCS_OUTPUT_PREFIX}/guide/index.html`);
    expect(
      new URL(
        getRewrittenUrl(
          await responseFor("https://platform.example/guide/"),
        )!,
        "https://platform.example",
      ).pathname,
    ).toBe(`${DOCS_OUTPUT_PREFIX}/guide/index.html`);
  });

  test("does not append a second extension to legacy HTML requests", async () => {
    const { getRewrittenUrl } = await loadConfigTesting();
    const rewritten = new URL(
      getRewrittenUrl(
        await responseFor(
          "https://platform.example/guide/getting-started.html",
        ),
      )!,
      "https://platform.example",
    );

    expect(rewritten.pathname).toBe(
      `${DOCS_OUTPUT_PREFIX}/guide/getting-started.html`,
    );
  });

  test("serves VitePress bundles and every public root asset", async () => {
    const { getRewrittenUrl } = await loadConfigTesting();

    const bundleRewrite = new URL(
      getRewrittenUrl(
        await responseFor(
          "https://platform.example/assets/chunks/search.js?version=abc",
        ),
      )!,
      "https://platform.example",
    );
    expect(bundleRewrite.pathname).toBe(
      `${DOCS_OUTPUT_PREFIX}/assets/chunks/search.js`,
    );
    expect(bundleRewrite.search).toBe("?version=abc");

    for (const asset of DOCS_ROOT_ASSETS) {
      const rewritten = new URL(
        getRewrittenUrl(
          await responseFor(`https://platform.example${asset}`),
        )!,
        "https://platform.example",
      );
      expect(rewritten.pathname).toBe(`${DOCS_OUTPUT_PREFIX}${asset}`);
    }
  });

  test("serves the docs sitemap without claiming the platform sitemap", async () => {
    const { getRewrittenUrl } = await loadConfigTesting();

    expect(
      new URL(
        getRewrittenUrl(
          await responseFor("https://platform.example/docs-sitemap.xml"),
        )!,
        "https://platform.example",
      ).pathname,
    ).toBe(`${DOCS_OUTPUT_PREFIX}/sitemap.xml`);
    expect(
      getRewrittenUrl(
        await responseFor("https://platform.example/sitemap.xml"),
      ),
    ).toBeNull();
  });

  test("permanently redirects the compatibility docs URL", async () => {
    const { getRedirectUrl } = await loadConfigTesting();
    const response = await responseFor(
      "https://platform.example/docs?source=legacy",
    );
    const slashResponse = await responseFor(
      "https://platform.example/docs/?source=legacy",
    );

    expect(response.status).toBe(308);
    expect(getRedirectUrl(response)).toBe(
      "https://platform.example/guide?source=legacy",
    );
    expect(slashResponse.status).toBe(308);
    expect(getRedirectUrl(slashResponse)).toBe(
      "https://platform.example/guide?source=legacy",
    );
  });

  test("does not proxy platform or unknown routes", async () => {
    const { getRewrittenUrl } = await loadConfigTesting();

    for (const path of [
      "/",
      "/@vite/client",
      "/.vitepress/theme/index.ts",
      "/api/v1/health",
      "/_docs/guide/index.html",
      "/node_modules/vitepress/index.js",
      "/_next/static/app.js",
      "/unknown",
    ]) {
      expect(
        getRewrittenUrl(
          await responseFor(`https://platform.example${path}`),
        ),
      ).toBeNull();
    }

    expect(nextConfig.rewrites).toBeFunction();
    const rewrites = await nextConfig.rewrites!();
    expect(Array.isArray(rewrites)).toBeFalse();
    if (!Array.isArray(rewrites)) {
      expect(rewrites.beforeFiles).toContainEqual({
        destination: `${DOCS_OUTPUT_PREFIX}/assets/:path*`,
        source: "/assets/:path*",
      });
      expect(rewrites.afterFiles).toEqual([]);
      expect(rewrites.fallback).toEqual([]);
    }
  });
});
