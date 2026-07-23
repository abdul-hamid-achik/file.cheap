import { AsyncLocalStorage } from "node:async_hooks";

import { describe, expect, test } from "bun:test";

import {
  DOCS_ROOT_ASSETS,
  DOCS_SECTIONS,
  LOCAL_DOCS_ORIGIN,
  resolveDocsOrigin,
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
    expect(policy).toContain("https://vercel.com");
    expect(policy).toContain("https://*.private.blob.vercel-storage.com");
    expect(policy).toContain("https://fonts.googleapis.com");
    expect(policy).toContain("https://fonts.gstatic.com");
    expect(policy).toContain("font-src 'self' https://fonts.gstatic.com");
    expect(policy).toContain(
      "style-src 'self' 'unsafe-inline' https://fonts.googleapis.com",
    );
    expect(headers.get("Permissions-Policy")).toContain("payment=()");
    expect(headers.get("Referrer-Policy")).toBe("no-referrer");
    expect(headers.get("Strict-Transport-Security")).toBe(
      "max-age=31536000",
    );
    expect(headers.get("X-Content-Type-Options")).toBe("nosniff");
    expect(headers.get("X-Frame-Options")).toBe("DENY");
  });
});

describe("documentation origin", () => {
  test("uses a loopback VitePress server outside Vercel", () => {
    expect(resolveDocsOrigin({})).toBe(LOCAL_DOCS_ORIGIN);
  });

  test("requires an explicit origin on Vercel", () => {
    expect(() => resolveDocsOrigin({ VERCEL: "1" })).toThrow(
      "FILECHEAP_DOCS_ORIGIN is required on Vercel",
    );
  });

  test("accepts a distinct immutable HTTPS deployment", () => {
    expect(
      resolveDocsOrigin({
        FILECHEAP_DOCS_ORIGIN:
          "https://file-cheap-docs-4f7a9c2-the-lacanians.vercel.app/",
        PLATFORM_PUBLIC_URL: "https://file.cheap",
        VERCEL: "1",
        VERCEL_URL: "file-cheap-platform-git-abc.vercel.app",
      }),
    ).toBe("https://file-cheap-docs-4f7a9c2-the-lacanians.vercel.app");
  });

  test("rejects malformed, unsafe, and looping origins", () => {
    const invalidEnvironments = [
      { FILECHEAP_DOCS_ORIGIN: "not a URL" },
      { FILECHEAP_DOCS_ORIGIN: "http://docs.example.com" },
      {
        FILECHEAP_DOCS_ORIGIN: "http://127.0.0.1:5173",
        VERCEL: "1",
      },
      { FILECHEAP_DOCS_ORIGIN: "https://user:secret@docs.example.com" },
      { FILECHEAP_DOCS_ORIGIN: "https://docs.example.com/subpath" },
      { FILECHEAP_DOCS_ORIGIN: "https://docs.example.com?preview=1" },
      { FILECHEAP_DOCS_ORIGIN: "https://docs.example.com/#preview" },
      { FILECHEAP_DOCS_ORIGIN: "https://file.cheap" },
      { FILECHEAP_DOCS_ORIGIN: "https://www.file.cheap" },
      { FILECHEAP_DOCS_ORIGIN: "https://FILE.CHEAP." },
      {
        FILECHEAP_DOCS_ORIGIN:
          "https://file-cheap-docs-git-main-the-lacanians.vercel.app",
      },
      {
        FILECHEAP_DOCS_ORIGIN: "https://platform-preview.vercel.app",
        VERCEL: "1",
        VERCEL_URL: "platform-preview.vercel.app",
      },
      {
        FILECHEAP_DOCS_ORIGIN: "http://127.0.0.1:3100",
        PLATFORM_PUBLIC_URL: "http://127.0.0.1:3100",
      },
    ];

    for (const environment of invalidEnvironments) {
      expect(() => resolveDocsOrigin(environment)).toThrow();
    }
  });
});

describe("documentation routing", () => {
  test("proxies every documentation section and preserves path and query", async () => {
    const { getRewrittenUrl } = await loadConfigTesting();

    for (const section of DOCS_SECTIONS) {
      const response = await responseFor(
        `https://platform.example/${section}/reference.html?source=test`,
      );
      const rewritten = new URL(getRewrittenUrl(response)!);
      expect(rewritten.hostname).toBe("127.0.0.1");
      expect(rewritten.pathname).toBe(`/${section}/reference.html`);
      expect(rewritten.search).toBe("?source=test");
    }
  });

  test("preserves both clean section-root forms", async () => {
    const { getRewrittenUrl } = await loadConfigTesting();

    expect(
      new URL(
        getRewrittenUrl(
          await responseFor("https://platform.example/guide"),
        )!,
      ).pathname,
    ).toBe("/guide");
    expect(
      new URL(
        getRewrittenUrl(
          await responseFor("https://platform.example/guide/"),
        )!,
      ).pathname,
    ).toBe("/guide/");
  });

  test("proxies VitePress bundles and every public root asset", async () => {
    const { getRewrittenUrl } = await loadConfigTesting();

    const bundleRewrite = new URL(
      getRewrittenUrl(
        await responseFor(
          "https://platform.example/assets/chunks/search.js?version=abc",
        ),
      )!,
    );
    expect(bundleRewrite.pathname).toBe("/assets/chunks/search.js");
    expect(bundleRewrite.search).toBe("?version=abc");

    for (const asset of DOCS_ROOT_ASSETS) {
      const rewritten = new URL(
        getRewrittenUrl(
          await responseFor(`https://platform.example${asset}`),
        )!,
      );
      expect(rewritten.pathname).toBe(asset);
    }
  });

  test("proxies the docs sitemap without claiming the platform sitemap", async () => {
    const { getRewrittenUrl } = await loadConfigTesting();

    expect(
      new URL(
        getRewrittenUrl(
          await responseFor("https://platform.example/docs-sitemap.xml"),
        )!,
      ).pathname,
    ).toBe("/sitemap.xml");
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
        destination: `${LOCAL_DOCS_ORIGIN}/assets/:path*`,
        source: "/assets/:path*",
      });
      expect(rewrites.afterFiles).toEqual([]);
      expect(rewrites.fallback).toEqual([]);
    }
  });
});
