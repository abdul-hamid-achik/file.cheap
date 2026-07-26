import { z } from "zod";

import {
  defaultProducerMaxSizeBytes,
  maximumArtifactBytes,
} from "@/shared/config/limits";

const environmentSchema = z.object({
  DATABASE_URL: z.string().min(1).optional(),
  FILECHEAP_ADMIN_TOKEN: z.string().min(32).max(256).optional(),
  FILECHEAP_OIDC_AUDIENCE: z.string().url().optional(),
  FILECHEAP_OIDC_ISSUER: z.string().url().optional(),
  FILECHEAP_OIDC_SUBJECTS: z.string().min(1).optional(),
  FILECHEAP_PUBLISHER_TOKENS: z.string().min(1).max(8_192).optional(),
  CRON_SECRET: z.string().min(32).max(256).optional(),
  PLATFORM_PUBLIC_URL: z.url().default("http://127.0.0.1:3100"),
  BLOB_READ_WRITE_TOKEN: z.string().min(1).optional(),
});

export type PublisherTokenSet = Readonly<{
  kinds: readonly string[];
  /**
   * Per-producer byte quota. Always populated: an entry that omits
   * `maxSizeBytes` falls back to the conservative default, never to the global
   * ceiling.
   */
  maxSizeBytes: number;
  nativeSchemas: readonly string[];
  producerTool: string;
  tokens: readonly string[];
}>;

export type PlatformConfig = {
  adminToken: string;
  blobReadWriteToken?: string;
  cronSecret: string;
  databaseUrl: string;
  oidc?: { audience: string; issuer: string; subjects: string[] };
  publisherTokens: readonly PublisherTokenSet[];
  publicUrl: string;
};

let cachedConfig: PlatformConfig | undefined;

export function getDatabaseUrl(
  env: Readonly<Record<string, string | undefined>> = process.env,
): string {
  const value = z.string().min(1).safeParse(env.DATABASE_URL);
  if (!value.success) throw new Error("DATABASE_URL is required for database access");
  return value.data;
}

export function getConfig(): PlatformConfig {
  if (cachedConfig) {
    return cachedConfig;
  }

  const parsed = environmentSchema.parse(process.env);
  const missing = [
    ["DATABASE_URL", parsed.DATABASE_URL],
    ["FILECHEAP_ADMIN_TOKEN", parsed.FILECHEAP_ADMIN_TOKEN],
    ["CRON_SECRET", parsed.CRON_SECRET],
  ].filter(([, value]) => !value).map(([name]) => name);
  const oidcConfigured = Boolean(parsed.FILECHEAP_OIDC_AUDIENCE || parsed.FILECHEAP_OIDC_ISSUER || parsed.FILECHEAP_OIDC_SUBJECTS);
  if (oidcConfigured && !(parsed.FILECHEAP_OIDC_AUDIENCE && parsed.FILECHEAP_OIDC_ISSUER && parsed.FILECHEAP_OIDC_SUBJECTS)) {
    throw new Error("FILECHEAP_OIDC_ISSUER, FILECHEAP_OIDC_AUDIENCE, and FILECHEAP_OIDC_SUBJECTS must be configured together");
  }
  const oidcSubjects = parsed.FILECHEAP_OIDC_SUBJECTS?.split(",").map((value) => value.trim()).filter(Boolean) ?? [];
  if (oidcConfigured) {
    assertVercelOidcIssuer(parsed.FILECHEAP_OIDC_ISSUER!);
    assertCredentialFreeUrl("FILECHEAP_OIDC_AUDIENCE", parsed.FILECHEAP_OIDC_AUDIENCE!);
    if (
      oidcSubjects.length === 0 ||
      new Set(oidcSubjects).size !== oidcSubjects.length ||
      oidcSubjects.some((subject) => !/^owner:[^:\s]+:project:chalupa:environment:(?:development|preview|production)$/u.test(subject))
    ) {
      throw new Error("FILECHEAP_OIDC_SUBJECTS must contain unique exact Chalupa Vercel deployment subjects");
    }
    const vercelEnvironment = process.env.VERCEL_ENV;
    if (
      process.env.VERCEL &&
      /^(?:development|preview|production)$/u.test(vercelEnvironment ?? "") &&
      (
        oidcSubjects.length !== 1 ||
        !oidcSubjects[0]?.endsWith(`:environment:${vercelEnvironment}`)
      )
    ) {
      throw new Error("FILECHEAP_OIDC_SUBJECTS must contain only the exact Chalupa subject for VERCEL_ENV");
    }
  }
  const publisherTokens = parsePublisherTokens(parsed.FILECHEAP_PUBLISHER_TOKENS);
  if (process.env.VERCEL && !oidcConfigured) {
    throw new Error("Vercel private artifact routes require FILECHEAP_OIDC_*; publisher tokens are an external-producer fallback only");
  }
  if (publisherTokens.length === 0 && !oidcConfigured) {
    missing.push("FILECHEAP_PUBLISHER_TOKENS or FILECHEAP_OIDC_*");
  }
  if (missing.length > 0) {
    throw new Error(`Private artifact service is not configured: missing ${missing.join(", ")}`);
  }
  if (
    parsed.FILECHEAP_ADMIN_TOKEN === parsed.CRON_SECRET ||
    publisherTokens.some(({ tokens }) =>
      tokens.includes(parsed.FILECHEAP_ADMIN_TOKEN!) ||
      tokens.includes(parsed.CRON_SECRET!),
    )
  ) {
    throw new Error("Private service credentials must be distinct");
  }
  const publicUrl = normalizePublicUrl(parsed.PLATFORM_PUBLIC_URL);
  if (
    (process.env.NODE_ENV === "production" || Boolean(process.env.VERCEL)) &&
    new URL(publicUrl).protocol !== "https:" &&
    !isLoopbackUrl(publicUrl)
  ) {
    throw new Error(
      "PLATFORM_PUBLIC_URL must use https outside loopback in production",
    );
  }
  cachedConfig = {
    adminToken: parsed.FILECHEAP_ADMIN_TOKEN!,
    blobReadWriteToken: parsed.BLOB_READ_WRITE_TOKEN,
    cronSecret: parsed.CRON_SECRET!,
    databaseUrl: getDatabaseUrl(),
    oidc: oidcConfigured ? { audience: parsed.FILECHEAP_OIDC_AUDIENCE!, issuer: parsed.FILECHEAP_OIDC_ISSUER!, subjects: oidcSubjects } : undefined,
    publisherTokens,
    publicUrl,
  };

  return cachedConfig;
}

function parsePublisherTokens(raw: string | undefined): readonly PublisherTokenSet[] {
  if (!raw) {
    return [];
  }

  let value: unknown;
  try {
    value = JSON.parse(raw);
  } catch {
    throw invalidPublisherTokens();
  }
  if (
    typeof value !== "object" ||
    value === null ||
    Array.isArray(value)
  ) {
    throw invalidPublisherTokens();
  }

  const entries = Object.entries(value);
  if (entries.length === 0 || entries.length > 16) {
    throw invalidPublisherTokens();
  }

  const allTokens = new Set<string>();
  const result: PublisherTokenSet[] = [];
  for (const [producerTool, policy] of entries) {
    if (
      !/^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$/u.test(producerTool) ||
      typeof policy !== "object" ||
      policy === null ||
      Array.isArray(policy) ||
      !hasExactPolicyKeys(Object.keys(policy))
    ) {
      throw invalidPublisherTokens();
    }
    const {
      kinds,
      maxSizeBytes,
      nativeSchemas,
      tokens,
    } = policy as Record<string, unknown>;
    if (
      (maxSizeBytes !== undefined &&
        (typeof maxSizeBytes !== "number" ||
          !Number.isSafeInteger(maxSizeBytes) ||
          maxSizeBytes < 1 ||
          maxSizeBytes > maximumArtifactBytes)) ||
      !Array.isArray(tokens) ||
      tokens.length < 1 ||
      tokens.length > 2 ||
      tokens.some(
        (token) =>
          typeof token !== "string" ||
          !/^[A-Za-z0-9_-]{43,128}$/u.test(token),
      ) ||
      new Set(tokens).size !== tokens.length ||
      tokens.some((token) => allTokens.has(token)) ||
      !Array.isArray(kinds) ||
      kinds.length < 1 ||
      kinds.length > 8 ||
      kinds.some(
        (kind) =>
          typeof kind !== "string" ||
          !/^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$/u.test(kind),
      ) ||
      new Set(kinds).size !== kinds.length ||
      !Array.isArray(nativeSchemas) ||
      nativeSchemas.length < 1 ||
      nativeSchemas.length > 8 ||
      nativeSchemas.some(
        (nativeSchema) =>
          typeof nativeSchema !== "string" ||
          !isSafeNativeSchema(nativeSchema),
      ) ||
      new Set(nativeSchemas).size !== nativeSchemas.length
    ) {
      throw invalidPublisherTokens();
    }
    for (const token of tokens) {
      allTokens.add(token);
    }
    result.push({
      kinds: Object.freeze([...kinds]),
      maxSizeBytes:
        typeof maxSizeBytes === "number"
          ? maxSizeBytes
          : defaultProducerMaxSizeBytes,
      nativeSchemas: Object.freeze([...nativeSchemas]),
      producerTool,
      tokens: Object.freeze([...tokens]),
    });
  }
  return Object.freeze(result);
}

const requiredPolicyKeys = ["kinds", "nativeSchemas", "tokens"] as const;
const optionalPolicyKeys = ["maxSizeBytes"] as const;

function hasExactPolicyKeys(keys: readonly string[]): boolean {
  const unique = new Set(keys);
  return (
    unique.size === keys.length &&
    requiredPolicyKeys.every((name) => unique.has(name)) &&
    keys.every(
      (name) =>
        (requiredPolicyKeys as readonly string[]).includes(name) ||
        (optionalPolicyKeys as readonly string[]).includes(name),
    )
  );
}

function invalidPublisherTokens(): Error {
  return new Error(
    `FILECHEAP_PUBLISHER_TOKENS must define 1-16 exact producer policies with bounded kinds, nativeSchemas, 1-2 unique 43-128 character base64url tokens, and an optional maxSizeBytes between 1 and ${maximumArtifactBytes}`,
  );
}

function isSafeNativeSchema(value: string): boolean {
  if (!/^[\x21-\x7e]+$/u.test(value) || value.includes("?")) {
    return false;
  }
  if (value.startsWith("urn:")) {
    return value.length > "urn:".length;
  }
  try {
    const url = new URL(value);
    return (
      url.protocol === "https:" &&
      url.hostname.length > 0 &&
      url.username === "" &&
      url.password === "" &&
      url.search === ""
    );
  } catch {
    return false;
  }
}

function assertVercelOidcIssuer(value: string): void {
  const url = new URL(value);
  assertCredentialFreeUrl("FILECHEAP_OIDC_ISSUER", value);
  if (
    url.protocol !== "https:" ||
    url.hostname !== "oidc.vercel.com" ||
    url.port !== "" ||
    !/^\/(?:[A-Za-z0-9_-]+)?$/u.test(url.pathname) ||
    value !== (url.pathname === "/" ? url.origin : `${url.origin}${url.pathname}`)
  ) {
    throw new Error("FILECHEAP_OIDC_ISSUER must be the exact global or team-scoped Vercel issuer");
  }
}

function assertCredentialFreeUrl(name: string, value: string): void {
  const url = new URL(value);
  if (url.protocol !== "https:" || url.username || url.password || url.search || url.hash) {
    throw new Error(`${name} must be HTTPS and must not contain credentials, a query, or a fragment`);
  }
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
