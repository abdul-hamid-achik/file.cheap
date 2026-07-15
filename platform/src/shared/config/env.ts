import { join } from "node:path";

import { z } from "zod";

const environmentSchema = z.object({
  PLATFORM_STORAGE_DRIVER: z.enum(["local", "vercel-blob"]).default("local"),
  PLATFORM_API_TOKEN: z.string().min(16).default("local-development-token"),
  PLATFORM_SIGNING_SECRET: z
    .string()
    .min(32)
    .default("local-development-signing-secret-change-me"),
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
  if (process.env.VERCEL && parsed.PLATFORM_STORAGE_DRIVER === "local") {
    throw new Error(
      "PLATFORM_STORAGE_DRIVER=local is intentionally disabled on Vercel; configure a private Blob store",
    );
  }
  if (
    process.env.VERCEL &&
    parsed.PLATFORM_API_TOKEN === "local-development-token"
  ) {
    throw new Error("PLATFORM_API_TOKEN must be replaced before a Vercel build");
  }
  cachedConfig = {
    apiToken: parsed.PLATFORM_API_TOKEN,
    blobReadWriteToken: parsed.BLOB_READ_WRITE_TOKEN,
    dataDirectory: join(process.cwd(), ".data"),
    publicUrl: parsed.PLATFORM_PUBLIC_URL.replace(/\/$/, ""),
    signingSecret: parsed.PLATFORM_SIGNING_SECRET,
    storageDriver: parsed.PLATFORM_STORAGE_DRIVER,
  };

  return cachedConfig;
}

export function resetConfigForTests(): void {
  cachedConfig = undefined;
}
