import type { NextConfig } from "next";

const isDevelopment = process.env.NODE_ENV !== "production";
const contentSecurityPolicy = [
  "default-src 'self'",
  "base-uri 'self'",
  `connect-src 'self' https://vercel.com https://*.private.blob.vercel-storage.com${
    isDevelopment ? " ws: wss:" : ""
  }`,
  "font-src 'self'",
  "form-action 'self'",
  "frame-ancestors 'none'",
  "img-src 'self' blob: data:",
  "object-src 'none'",
  `script-src 'self' 'unsafe-inline'${
    isDevelopment ? " 'unsafe-eval'" : ""
  }`,
  "style-src 'self' 'unsafe-inline'",
].join("; ");

export const platformSecurityHeaders = [
  { key: "Content-Security-Policy", value: contentSecurityPolicy },
  {
    key: "Permissions-Policy",
    value: "camera=(), geolocation=(), microphone=(), payment=(), usb=()",
  },
  { key: "Referrer-Policy", value: "no-referrer" },
  { key: "X-Content-Type-Options", value: "nosniff" },
  { key: "X-Frame-Options", value: "DENY" },
] as const;

const nextConfig: NextConfig = {
  async headers() {
    return [
      {
        headers: [...platformSecurityHeaders],
        source: "/:path*",
      },
    ];
  },
  poweredByHeader: false,
  typedRoutes: true,
};

export default nextConfig;
