import { isAbsolute, join } from "node:path";

import { z } from "zod";

const developmentApiToken = "local-development-token";
const developmentSigningSecret = "local-development-signing-secret-change-me";

const environmentSchema = z.object({
  PLATFORM_STORAGE_DRIVER: z.enum(["local", "vercel-blob"]).default("local"),
  PLATFORM_BLOB_INTEGRITY: z
    .literal("presence-size-etag-experimental")
    .optional(),
  PLATFORM_API_TOKEN: z.string().min(16).default(developmentApiToken),
  PLATFORM_SIGNING_SECRET: z
    .string()
    .min(32)
    .default(developmentSigningSecret),
  PLATFORM_DATA_DIR: z.string().min(1).optional(),
  PLATFORM_PUBLIC_URL: z.url().default("http://127.0.0.1:3100"),
  BLOB_READ_WRITE_TOKEN: z.string().min(1).optional(),
});

export type PlatformConfig = {
  apiToken: string;
  blobReadWriteToken?: string;
  dataDirectory: string;
  publicUrl: string;
  signingSecret: string;
  storageDriver: "local" | "vercel-blob";
};

let cachedConfig: PlatformConfig | undefined;

export function getConfig(): PlatformConfig {
  if (cachedConfig) {
    return cachedConfig;
  }

  const parsed = environmentSchema.parse(process.env);
  const productionEnvironment =
    process.env.NODE_ENV === "production" || Boolean(process.env.VERCEL);
  if (parsed.PLATFORM_DATA_DIR && !isAbsolute(parsed.PLATFORM_DATA_DIR)) {
    throw new Error("PLATFORM_DATA_DIR must be an absolute path when provided");
  }
  if (process.env.VERCEL && parsed.PLATFORM_STORAGE_DRIVER === "local") {
    throw new Error(
      "PLATFORM_STORAGE_DRIVER=local is intentionally disabled on Vercel; configure a private Blob store",
    );
  }
  if (productionEnvironment && parsed.PLATFORM_API_TOKEN === developmentApiToken) {
    throw new Error("PLATFORM_API_TOKEN must be replaced in production");
  }
  if (
    productionEnvironment &&
    parsed.PLATFORM_SIGNING_SECRET === developmentSigningSecret
  ) {
    throw new Error("PLATFORM_SIGNING_SECRET must be replaced in production");
  }
  if (
    parsed.PLATFORM_STORAGE_DRIVER === "vercel-blob" &&
    parsed.PLATFORM_BLOB_INTEGRITY !== "presence-size-etag-experimental"
  ) {
    throw new Error(
      "Vercel Blob direct uploads are presence-only until staging and repair exist; set PLATFORM_BLOB_INTEGRITY=presence-size-etag-experimental only for a controlled spike",
    );
  }
  const publicUrl = normalizePublicUrl(parsed.PLATFORM_PUBLIC_URL);
  if (
    productionEnvironment &&
    new URL(publicUrl).protocol !== "https:" &&
    !isLoopbackUrl(publicUrl)
  ) {
    throw new Error(
      "PLATFORM_PUBLIC_URL must use https outside loopback in production",
    );
  }
  cachedConfig = {
    apiToken: parsed.PLATFORM_API_TOKEN,
    blobReadWriteToken: parsed.BLOB_READ_WRITE_TOKEN,
    dataDirectory: parsed.PLATFORM_DATA_DIR ?? join(process.cwd(), ".data"),
    publicUrl,
    signingSecret: parsed.PLATFORM_SIGNING_SECRET,
    storageDriver: parsed.PLATFORM_STORAGE_DRIVER,
  };

  return cachedConfig;
}

function isLoopbackUrl(value: string): boolean {
  const hostname = new URL(value).hostname;
  return (
    hostname === "localhost" ||
    hostname === "127.0.0.1" ||
    hostname === "[::1]"
  );
}

export function resetConfigForTests(): void {
  cachedConfig = undefined;
}

function normalizePublicUrl(value: string): string {
  const url = new URL(value);
  if (url.protocol !== "http:" && url.protocol !== "https:") {
    throw new Error("PLATFORM_PUBLIC_URL must use http or https");
  }
  if (url.username || url.password) {
    throw new Error("PLATFORM_PUBLIC_URL must not contain credentials");
  }
  if (url.search || url.hash) {
    throw new Error("PLATFORM_PUBLIC_URL must not contain a query or fragment");
  }
  if (!/^\/+$/u.test(url.pathname)) {
    throw new Error("PLATFORM_PUBLIC_URL must not contain a path");
  }
  return url.origin;
}
