import type { NextConfig } from "next";

import { createDocsRewrites } from "./docs-routing";

const isDevelopment = process.env.NODE_ENV !== "production";
const contentSecurityPolicy = [
  "default-src 'self'",
  "base-uri 'self'",
  `connect-src 'self' https://fonts.googleapis.com https://fonts.gstatic.com https://vercel.com https://*.private.blob.vercel-storage.com${
    isDevelopment ? " ws: wss:" : ""
  }`,
  "font-src 'self' data: https://fonts.gstatic.com",
  "form-action 'self'",
  "frame-ancestors 'none'",
  "img-src 'self' blob: data:",
  "manifest-src 'self'",
  "object-src 'none'",
  `script-src 'self' 'unsafe-inline'${
    isDevelopment ? " 'unsafe-eval'" : ""
  }`,
  "style-src 'self' 'unsafe-inline' https://fonts.googleapis.com",
  "worker-src 'self' blob:",
  "upgrade-insecure-requests",
].join("; ");

export const platformSecurityHeaders = [
  { key: "Content-Security-Policy", value: contentSecurityPolicy },
  {
    key: "Strict-Transport-Security",
    value: "max-age=31536000",
  },
  {
    key: "Permissions-Policy",
    value:
      "camera=(), display-capture=(), geolocation=(), microphone=(), payment=(), publickey-credentials-get=(), usb=()",
  },
  {
    key: "Referrer-Policy",
    value: "strict-origin-when-cross-origin",
  },
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
      beforeFiles: createDocsRewrites(),
      fallback: [],
    };
  },
  async headers() {
    return [
      {
        headers: [...platformSecurityHeaders],
        source: "/:path*",
      },
      {
        headers: [{ key: "X-Robots-Tag", value: "noindex, nofollow" }],
        source: "/_docs/:path*",
      },
      {
        headers: [{ key: "X-Robots-Tag", value: "noindex, nofollow" }],
        source: "/console/:path*",
      },
      {
        headers: [
          {
            key: "Cache-Control",
            value: "public, max-age=31536000, immutable",
          },
        ],
        source: "/assets/:path*",
      },
    ];
  },
  poweredByHeader: false,
  skipTrailingSlashRedirect: true,
  typedRoutes: true,
};

export default nextConfig;
