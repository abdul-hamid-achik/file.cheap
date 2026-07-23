export const DOCS_OUTPUT_PREFIX = "/_docs";

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

export type DocsRewrite = {
  destination: string;
  source: string;
};

export function createDocsRewrites(
  outputPrefix = DOCS_OUTPUT_PREFIX,
): DocsRewrite[] {
  const sectionRewrites = DOCS_SECTIONS.flatMap((section) => [
    {
      source: `/${section}`,
      destination: `${outputPrefix}/${section}/index.html`,
    },
    {
      source: `/${section}/`,
      destination: `${outputPrefix}/${section}/index.html`,
    },
    {
      source: `/${section}/:path*.html`,
      destination: `${outputPrefix}/${section}/:path*.html`,
    },
    {
      source: `/${section}/:path*`,
      destination: `${outputPrefix}/${section}/:path*.html`,
    },
  ]);

  return [
    ...sectionRewrites,
    {
      source: "/assets/:path*",
      destination: `${outputPrefix}/assets/:path*`,
    },
    ...DOCS_ROOT_ASSETS.map((asset) => ({
      source: asset,
      destination: `${outputPrefix}${asset}`,
    })),
    {
      source: "/docs-sitemap.xml",
      destination: `${outputPrefix}/sitemap.xml`,
    },
  ];
}
