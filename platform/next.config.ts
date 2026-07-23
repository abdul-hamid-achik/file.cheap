import type { NextConfig } from "next";

import { createDocsRewrites, resolveDocsOrigin } from "./docs-routing";

const isDevelopment = process.env.NODE_ENV !== "production";
const docsOrigin = resolveDocsOrigin();
const contentSecurityPolicy = [
  "default-src 'self'",
  "base-uri 'self'",
  `connect-src 'self' https://fonts.googleapis.com https://fonts.gstatic.com https://vercel.com https://*.private.blob.vercel-storage.com${
    isDevelopment ? " ws: wss:" : ""
  }`,
  "font-src 'self' https://fonts.gstatic.com",
  "form-action 'self'",
  "frame-ancestors 'none'",
  "img-src 'self' blob: data:",
  "object-src 'none'",
  `script-src 'self' 'unsafe-inline'${
    isDevelopment ? " 'unsafe-eval'" : ""
  }`,
  "style-src 'self' 'unsafe-inline' https://fonts.googleapis.com",
].join("; ");

export const platformSecurityHeaders = [
  { key: "Content-Security-Policy", value: contentSecurityPolicy },
  {
    key: "Strict-Transport-Security",
    value: "max-age=31536000",
  },
  {
    key: "Permissions-Policy",
    value: "camera=(), geolocation=(), microphone=(), payment=(), usb=()",
  },
  { key: "Referrer-Policy", value: "no-referrer" },
  { key: "X-Content-Type-Options", value: "nosniff" },
  { key: "X-Frame-Options", value: "DENY" },
] as const;

const nextConfig: NextConfig = {
  async redirects() {
    return [
      {
        destination: "/guide",
        permanent: true,
        source: "/docs",
      },
    ];
  },
  async rewrites() {
    return {
      afterFiles: [],
      beforeFiles: createDocsRewrites(docsOrigin),
      fallback: [],
    };
  },
  async headers() {
    return [
      {
        headers: [...platformSecurityHeaders],
        source: "/:path*",
      },
    ];
  },
  poweredByHeader: false,
  skipTrailingSlashRedirect: true,
  typedRoutes: true,
};

export default nextConfig;
