import type { MetadataRoute } from "next";

const publicOrigin = "https://file.cheap";

export default function robots(): MetadataRoute.Robots {
  return {
    host: publicOrigin,
    rules: {
      allow: "/",
      disallow: ["/api/", "/lab"],
      userAgent: "*",
    },
    sitemap: [
      `${publicOrigin}/sitemap.xml`,
      `${publicOrigin}/docs-sitemap.xml`,
    ],
  };
}
