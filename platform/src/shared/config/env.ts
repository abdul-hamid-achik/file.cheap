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
  FILECHEAP_OWNER_ACCOUNT_ID: z.string().regex(/^acc_[A-Za-z0-9_-]{8,64}$/u),
  FILECHEAP_PUBLISHER_TOKENS: z.string().min(1).max(8_192).optional(),
  CRON_SECRET: z.string().min(32).max(256).optional(),
  PLATFORM_PUBLIC_URL: z.url().default("http://127.0.0.1:3100"),
  BLOB_READ_WRITE_TOKEN: z.string().min(1).optional(),
});

/**
 * One exact `kind` ↔ `producer.native_schema` pair a publisher may use, with
 * its own resolved byte quota. A producer configured with bindings can never
 * publish the cross product of its kinds and schemas, and can carry one small
 * kind (an inference receipt) next to a larger one (a session transcript)
 * under a single credential.
 */
export type PublisherKindSchemaBinding = Readonly<{
  kind: string;
  /**
   * Resolved per-kind byte quota. Always populated and never larger than the
   * producer's own `maxSizeBytes`.
   */
  maxSizeBytes: number;
  nativeSchema: string;
}>;

export type PublisherTokenSet = Readonly<{
  /**
   * Present only for a producer configured with exact pairs. When absent the
   * producer keeps the historical `kinds` × `nativeSchemas` allowlist.
   */
  kindSchemaBindings?: readonly PublisherKindSchemaBinding[];
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
  ownerAccountId: string;
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
    ["FILECHEAP_OWNER_ACCOUNT_ID", parsed.FILECHEAP_OWNER_ACCOUNT_ID],
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
    ownerAccountId: parsed.FILECHEAP_OWNER_ACCOUNT_ID,
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
    const shape =
      typeof policy === "object" && policy !== null && !Array.isArray(policy)
        ? policyShape(Object.keys(policy))
        : null;
    if (
      !/^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$/u.test(producerTool) ||
      shape === null
    ) {
      throw invalidPublisherTokens();
    }
    const {
      kindSchemaBindings,
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
      tokens.some((token) => allTokens.has(token))
    ) {
      throw invalidPublisherTokens();
    }
    const producerMaxSizeBytes =
      typeof maxSizeBytes === "number"
        ? maxSizeBytes
        : defaultProducerMaxSizeBytes;
    const bindings = shape === "bindings"
      ? parseKindSchemaBindings(kindSchemaBindings, producerMaxSizeBytes)
      : undefined;
    const allowlists = bindings
      ? {
          kinds: bindings.map((binding) => binding.kind),
          nativeSchemas: bindings.map((binding) => binding.nativeSchema),
        }
      : parseKindAllowlists(kinds, nativeSchemas);
    for (const token of tokens) {
      allTokens.add(token);
    }
    result.push({
      ...(bindings ? { kindSchemaBindings: bindings } : {}),
      kinds: Object.freeze(allowlists.kinds),
      maxSizeBytes: producerMaxSizeBytes,
      nativeSchemas: Object.freeze(allowlists.nativeSchemas),
      producerTool,
      tokens: Object.freeze([...tokens]),
    });
  }
  return Object.freeze(result);
}

const allowlistPolicyKeys = ["kinds", "nativeSchemas", "tokens"] as const;
const bindingPolicyKeys = ["kindSchemaBindings", "tokens"] as const;
const optionalPolicyKeys = ["maxSizeBytes"] as const;
const bindingKeys = ["kind", "nativeSchema"] as const;
const optionalBindingKeys = ["maxSizeBytes"] as const;
const artifactKindPattern = /^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$/u;

/**
 * A producer policy declares its allowlist in exactly one of two shapes: the
 * historical `kinds` + `nativeSchemas` cross product, or exact
 * `kindSchemaBindings` pairs. Mixing them, or omitting `tokens`, is a
 * configuration error rather than a silently narrower policy.
 */
function policyShape(
  keys: readonly string[],
): "allowlists" | "bindings" | null {
  const unique = new Set(keys);
  const known = new Set<string>([
    ...allowlistPolicyKeys,
    ...bindingPolicyKeys,
    ...optionalPolicyKeys,
  ]);
  if (unique.size !== keys.length || keys.some((name) => !known.has(name))) {
    return null;
  }
  if (unique.has("kindSchemaBindings")) {
    return bindingPolicyKeys.every((name) => unique.has(name)) &&
      !unique.has("kinds") &&
      !unique.has("nativeSchemas")
      ? "bindings"
      : null;
  }
  return allowlistPolicyKeys.every((name) => unique.has(name))
    ? "allowlists"
    : null;
}

function parseKindAllowlists(
  kinds: unknown,
  nativeSchemas: unknown,
): { kinds: string[]; nativeSchemas: string[] } {
  if (
    !Array.isArray(kinds) ||
    kinds.length < 1 ||
    kinds.length > 8 ||
    kinds.some((kind) => !isSafeArtifactKind(kind)) ||
    new Set(kinds).size !== kinds.length ||
    !Array.isArray(nativeSchemas) ||
    nativeSchemas.length < 1 ||
    nativeSchemas.length > 8 ||
    nativeSchemas.some((nativeSchema) => !isSafeNativeSchemaValue(nativeSchema)) ||
    new Set(nativeSchemas).size !== nativeSchemas.length
  ) {
    throw invalidPublisherTokens();
  }
  return {
    kinds: [...(kinds as string[])],
    nativeSchemas: [...(nativeSchemas as string[])],
  };
}

function parseKindSchemaBindings(
  value: unknown,
  producerMaxSizeBytes: number,
): readonly PublisherKindSchemaBinding[] {
  if (!Array.isArray(value) || value.length < 1 || value.length > 8) {
    throw invalidPublisherTokens();
  }
  const bindings = value.map((entry) => {
    if (typeof entry !== "object" || entry === null || Array.isArray(entry)) {
      throw invalidPublisherTokens();
    }
    const keys = Object.keys(entry);
    const unique = new Set(keys);
    if (
      unique.size !== keys.length ||
      !bindingKeys.every((name) => unique.has(name)) ||
      keys.some(
        (name) =>
          !(bindingKeys as readonly string[]).includes(name) &&
          !(optionalBindingKeys as readonly string[]).includes(name),
      )
    ) {
      throw invalidPublisherTokens();
    }
    const { kind, maxSizeBytes, nativeSchema } = entry as Record<
      string,
      unknown
    >;
    if (
      !isSafeArtifactKind(kind) ||
      !isSafeNativeSchemaValue(nativeSchema) ||
      (maxSizeBytes !== undefined &&
        (typeof maxSizeBytes !== "number" ||
          !Number.isSafeInteger(maxSizeBytes) ||
          maxSizeBytes < 1 ||
          // A binding narrows the producer quota; it can never widen it.
          maxSizeBytes > producerMaxSizeBytes))
    ) {
      throw invalidPublisherTokens();
    }
    return Object.freeze({
      kind: kind as string,
      maxSizeBytes:
        typeof maxSizeBytes === "number" ? maxSizeBytes : producerMaxSizeBytes,
      nativeSchema: nativeSchema as string,
    });
  });
  if (
    new Set(bindings.map((binding) => binding.kind)).size !== bindings.length ||
    new Set(bindings.map((binding) => binding.nativeSchema)).size !==
      bindings.length
  ) {
    throw invalidPublisherTokens();
  }
  return Object.freeze(bindings);
}

function isSafeArtifactKind(value: unknown): value is string {
  return (
    typeof value === "string" &&
    value.length <= 128 &&
    artifactKindPattern.test(value)
  );
}

function isSafeNativeSchemaValue(value: unknown): value is string {
  return (
    typeof value === "string" && value.length <= 256 && isSafeNativeSchema(value)
  );
}

function invalidPublisherTokens(): Error {
  return new Error(
    `FILECHEAP_PUBLISHER_TOKENS must define 1-16 exact producer policies with either bounded kinds and nativeSchemas or exact kindSchemaBindings pairs, 1-2 unique 43-128 character base64url tokens, and an optional maxSizeBytes between 1 and ${maximumArtifactBytes} that every binding stays within`,
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
