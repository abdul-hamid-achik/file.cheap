export const LOCAL_DOCS_ORIGIN = "http://127.0.0.1:5173";

export const DOCS_SECTIONS = [
  "cli",
  "compare",
  "guide",
  "integrations",
  "learn",
  "mcp",
  "studio",
] as const;

export const DOCS_ROOT_ASSETS = [
  "/.well-known/security.txt",
  "/favicon.svg",
  "/hashmap.json",
  "/local-first-agent-stack.svg",
  "/og.png",
  "/og.svg",
  "/og-install.png",
  "/og-install.svg",
  "/og-mcp.png",
  "/og-mcp.svg",
  "/vp-icons.css",
] as const;

export type DocsRoutingEnvironment = Partial<
  Record<
    | "FILECHEAP_DOCS_ORIGIN"
    | "PLATFORM_PUBLIC_URL"
    | "VERCEL"
    | "VERCEL_BRANCH_URL"
    | "VERCEL_PROJECT_PRODUCTION_URL"
    | "VERCEL_URL",
    string
  >
>;

export type DocsRewrite = {
  destination: string;
  source: string;
};

function normalizeHostname(hostname: string) {
  return hostname.toLowerCase().replace(/\.$/, "");
}

function environmentHostname(value: string | undefined) {
  if (!value?.trim()) return undefined;

  try {
    const candidate = value.includes("://") ? value : `https://${value}`;
    return normalizeHostname(new URL(candidate).hostname);
  } catch {
    return undefined;
  }
}

function environmentOrigin(value: string | undefined) {
  if (!value?.trim()) return undefined;

  try {
    const candidate = value.includes("://") ? value : `https://${value}`;
    return new URL(candidate).origin.toLowerCase();
  } catch {
    return undefined;
  }
}

function isLoopback(hostname: string) {
  return (
    hostname === "127.0.0.1" ||
    hostname === "localhost" ||
    hostname === "::1" ||
    hostname === "[::1]"
  );
}

export function resolveDocsOrigin(
  environment: DocsRoutingEnvironment = process.env as DocsRoutingEnvironment,
) {
  const isVercel = environment.VERCEL === "1";
  const configured = environment.FILECHEAP_DOCS_ORIGIN?.trim();

  if (!configured && isVercel) {
    throw new Error(
      "FILECHEAP_DOCS_ORIGIN is required on Vercel and must identify the docs deployment",
    );
  }

  const rawOrigin = configured || LOCAL_DOCS_ORIGIN;
  let url: URL;

  try {
    url = new URL(rawOrigin);
  } catch {
    throw new Error("FILECHEAP_DOCS_ORIGIN must be a valid absolute URL");
  }

  const hostname = normalizeHostname(url.hostname);
  const localHttp = url.protocol === "http:" && isLoopback(hostname);
  if (url.protocol !== "https:" && (!localHttp || isVercel)) {
    throw new Error(
      "FILECHEAP_DOCS_ORIGIN must use HTTPS except for an explicit loopback origin",
    );
  }
  if (url.username || url.password) {
    throw new Error("FILECHEAP_DOCS_ORIGIN must not contain credentials");
  }
  if (url.pathname !== "/" || url.search || url.hash) {
    throw new Error(
      "FILECHEAP_DOCS_ORIGIN must be a bare origin without a path, query, or fragment",
    );
  }

  const forbiddenHostnames = new Set([
    "file.cheap",
    "www.file.cheap",
    environmentHostname(environment.VERCEL_URL),
    environmentHostname(environment.VERCEL_BRANCH_URL),
    environmentHostname(environment.VERCEL_PROJECT_PRODUCTION_URL),
  ]);
  forbiddenHostnames.delete(undefined);

  if (forbiddenHostnames.has(hostname)) {
    throw new Error(
      "FILECHEAP_DOCS_ORIGIN must not resolve to the file.cheap platform deployment",
    );
  }
  if (hostname.endsWith(".vercel.app") && hostname.includes("-git-")) {
    throw new Error(
      "FILECHEAP_DOCS_ORIGIN must use an immutable deployment URL, not a moving Vercel branch alias",
    );
  }

  const platformOrigin = environmentOrigin(environment.PLATFORM_PUBLIC_URL);
  if (platformOrigin && platformOrigin === url.origin.toLowerCase()) {
    throw new Error(
      "FILECHEAP_DOCS_ORIGIN must differ from PLATFORM_PUBLIC_URL to prevent a rewrite loop",
    );
  }

  return url.origin;
}

export function createDocsRewrites(origin: string): DocsRewrite[] {
  const sectionRewrites = DOCS_SECTIONS.flatMap((section) => [
    {
      source: `/${section}/`,
      destination: `${origin}/${section}/`,
    },
    {
      source: `/${section}/:path*`,
      destination: `${origin}/${section}/:path*`,
    },
  ]);

  return [
    ...sectionRewrites,
    {
      source: "/assets/:path*",
      destination: `${origin}/assets/:path*`,
    },
    ...DOCS_ROOT_ASSETS.map((asset) => ({
      source: asset,
      destination: `${origin}${asset}`,
    })),
    {
      source: "/docs-sitemap.xml",
      destination: `${origin}/sitemap.xml`,
    },
  ];
}
