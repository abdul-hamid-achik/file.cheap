import { createHash, timingSafeEqual } from "node:crypto";
import { createRemoteJWKSet, jwtVerify } from "jose";

import { getConfig } from "@/shared/config/env";
import type { PublisherTokenSet } from "@/shared/config/env";
import { defaultProducerMaxSizeBytes } from "@/shared/config/limits";
import { PlatformError } from "@/shared/errors/platform-error";

export type ServiceScope = "admin" | "cron" | "ingest" | "read";
export type ArtifactKindSchemaBinding = Readonly<{
  kind: string;
  nativeSchema: string;
}>;
export type IngestPolicy = Readonly<{
  kindSchemaBindings?: readonly ArtifactKindSchemaBinding[];
  kinds: readonly string[];
  maxSizeBytes: number;
  nativeSchemas: readonly string[];
  producerTool: string;
}>;
export type IngestPrincipal = Readonly<
  IngestPolicy & {
    authentication: "oidc" | "publisher-token";
    subject?: string;
  }
>;
export type ReadPrincipal =
  | Readonly<{ authentication: "admin" }>
  | Readonly<
      IngestPolicy & {
        authentication: "oidc";
        subject: string;
      }
    >;

const jwksByIssuer = new Map<string, ReturnType<typeof createRemoteJWKSet>>();
const chalupaOidcBindings: readonly ArtifactKindSchemaBinding[] = Object.freeze([
  Object.freeze({
    kind: "chalupa.log-chunk",
    nativeSchema: "urn:chalupa:log-chunk:v1",
  }),
  Object.freeze({
    kind: "chalupa.ci-artifact",
    nativeSchema: "urn:chalupa:ci-artifact:v1",
  }),
  Object.freeze({
    kind: "chalupa.ci-manifest",
    nativeSchema: "urn:chalupa:ci-manifest:v1",
  }),
]);
const chalupaOidcPolicy: IngestPolicy = Object.freeze({
  kindSchemaBindings: chalupaOidcBindings,
  kinds: Object.freeze(chalupaOidcBindings.map((binding) => binding.kind)),
  // Chalupa's OIDC identity is not part of the publisher keyring, so it keeps
  // the same conservative default quota an undeclared producer would get.
  // That default comfortably covers the largest authorized CI artifact part.
  maxSizeBytes: defaultProducerMaxSizeBytes,
  nativeSchemas: Object.freeze(
    chalupaOidcBindings.map((binding) => binding.nativeSchema),
  ),
  producerTool: "chalupa",
});

export function requireServiceToken(
  request: Request,
  scope: "ingest",
): Promise<IngestPrincipal>;
export function requireServiceToken(
  request: Request,
  scope: "read",
): Promise<ReadPrincipal>;
export function requireServiceToken(
  request: Request,
  scope: "admin" | "cron",
): Promise<void>;
export async function requireServiceToken(
  request: Request,
  scope: ServiceScope,
): Promise<IngestPrincipal | ReadPrincipal | void> {
  const credential = bearerCredential(request);
  if (!credential) throw unauthorized();
  if (scope === "ingest") {
    const config = getConfig();
    const subject = config.oidc
      ? await validOidcSubject(credential, config.oidc)
      : undefined;
    if (subject) {
      return { ...chalupaOidcPolicy, authentication: "oidc", subject };
    }
    const publisher = publisherForCredential(
      credential,
      config.publisherTokens,
    );
    if (publisher) {
      return {
        authentication: "publisher-token",
        kinds: publisher.kinds,
        maxSizeBytes: publisher.maxSizeBytes,
        nativeSchemas: publisher.nativeSchemas,
        producerTool: publisher.producerTool,
      };
    }
    throw unauthorized();
  }
  if (scope === "read") {
    const config = getConfig();
    if (constantTimeEqual(credential, config.adminToken)) {
      return { authentication: "admin" };
    }
    const subject = config.oidc
      ? await validOidcSubject(credential, config.oidc)
      : undefined;
    if (subject) {
      return { ...chalupaOidcPolicy, authentication: "oidc", subject };
    }
    throw unauthorized();
  }
  const configuredToken = tokenForScope(scope);
  if (!constantTimeEqual(credential, configuredToken)) throw unauthorized();
}

export function requireAuthorizedArtifact(
  principal: IngestPrincipal,
  artifact: {
    kind: string;
    producer: { native_schema?: string; tool: string };
  },
): void {
  if (
    !constantTimeEqual(artifact.producer.tool, principal.producerTool) ||
    !artifact.producer.native_schema ||
    !matchesKindAndSchema(
      principal,
      artifact.kind,
      artifact.producer.native_schema,
    )
  ) {
    throw unauthorized();
  }
}

export function ingestPolicyFor(principal: IngestPrincipal): IngestPolicy {
  return {
    ...(principal.kindSchemaBindings
      ? { kindSchemaBindings: principal.kindSchemaBindings }
      : {}),
    kinds: principal.kinds,
    maxSizeBytes: principal.maxSizeBytes,
    nativeSchemas: principal.nativeSchemas,
    producerTool: principal.producerTool,
  };
}

function matchesKindAndSchema(
  policy: IngestPolicy,
  kind: string,
  nativeSchema: string,
): boolean {
  if (policy.kindSchemaBindings) {
    return policy.kindSchemaBindings.some(
      (binding) =>
        binding.kind === kind && binding.nativeSchema === nativeSchema,
    );
  }
  return (
    policy.kinds.includes(kind) && policy.nativeSchemas.includes(nativeSchema)
  );
}

export function readPolicyFor(
  principal: ReadPrincipal,
): IngestPolicy | undefined {
  return principal.authentication === "admin"
    ? undefined
    : ingestPolicyFor(principal);
}

async function validOidcSubject(
  token: string,
  config: { audience: string; issuer: string; subjects: string[] },
): Promise<string | undefined> {
  try {
    const jwks = jwksByIssuer.get(config.issuer) ?? createRemoteJWKSet(new URL(".well-known/jwks", `${config.issuer.replace(/\/$/u, "")}/`));
    jwksByIssuer.set(config.issuer, jwks);
    const { payload } = await jwtVerify(token, jwks, {
      audience: config.audience,
      issuer: config.issuer,
      requiredClaims: ["sub", "iat", "nbf", "exp"],
    });
    return typeof payload.sub === "string" && config.subjects.includes(payload.sub)
      ? payload.sub
      : undefined;
  } catch {
    return undefined;
  }
}

function tokenForScope(
  scope: Exclude<ServiceScope, "ingest" | "read">,
): string {
  const config = getConfig();
  return scope === "admin" ? config.adminToken : config.cronSecret;
}

function publisherForCredential(
  credential: string,
  publishers: ReturnType<typeof getConfig>["publisherTokens"],
): PublisherTokenSet | undefined {
  let match: PublisherTokenSet | undefined;
  for (const publisher of publishers) {
    for (const token of publisher.tokens) {
      if (constantTimeEqual(credential, token)) {
        match = publisher;
      }
    }
  }
  return match;
}

function bearerCredential(request: Request): string | undefined {
  const authorization = request.headers.get("authorization");
  const credential = authorization
    ? /^[\t ]*Bearer[\t ]+([^\t ]+)[\t ]*$/i.exec(authorization)?.[1]
    : undefined;
  return credential && credential.length <= 4_096 ? credential : undefined;
}
function constantTimeEqual(received: string, expected: string): boolean {
  const receivedDigest = createHash("sha256").update(received).digest();
  const expectedDigest = createHash("sha256").update(expected).digest();
  return timingSafeEqual(receivedDigest, expectedDigest);
}
function unauthorized(): PlatformError { return new PlatformError({ code: "unauthorized", detail: "A valid private service credential is required.", status: 401, title: "Unauthorized" }); }
