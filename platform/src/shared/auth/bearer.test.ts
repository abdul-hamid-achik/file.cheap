import { afterEach, describe, expect, test } from "bun:test";
import { exportJWK, generateKeyPair, SignJWT } from "jose";
import {
  readPolicyFor,
  requireAuthorizedArtifact,
  requireServiceToken,
} from "@/shared/auth/bearer";
import { resetConfigForTests } from "@/shared/config/env";
import { defaultProducerMaxSizeBytes } from "@/shared/config/limits";

const originalEnvironment = { ...process.env };
const originalFetch = globalThis.fetch;
const chalupaToken = "c".repeat(43);
const nextChalupaToken = "n".repeat(43);
const cairntraceToken = "r".repeat(43);

afterEach(() => {
  for (const key of Object.keys(process.env)) if (!(key in originalEnvironment)) delete process.env[key];
  for (const [key, value] of Object.entries(originalEnvironment)) process.env[key] = value;
  globalThis.fetch = originalFetch;
  resetConfigForTests();
});

describe("private service bearer authentication", () => {
  test("accepts one producer's current and next tokens only for that producer", async () => {
    Object.assign(process.env, {
      DATABASE_URL: "postgresql://runtime",
      FILECHEAP_PUBLISHER_TOKENS: JSON.stringify({
        cairntrace: {
          kinds: ["cairntrace.run"],
          maxSizeBytes: 32 * 1024 * 1024,
          nativeSchemas: ["urn:cairntrace.dev:run:v1"],
          tokens: [cairntraceToken],
        },
        chalupa: {
          kinds: ["chalupa.log-chunk"],
          nativeSchemas: ["urn:chalupa:log-chunk:v1"],
          tokens: [chalupaToken, nextChalupaToken],
        },
      }),
      FILECHEAP_ADMIN_TOKEN: "a".repeat(32),
      FILECHEAP_OWNER_ACCOUNT_ID: "acc_owner123",
      CRON_SECRET: "z".repeat(32),
    });
    delete process.env.VERCEL;
    resetConfigForTests();
    const request = (value?: string) => new Request("https://file.cheap/api/v1/artifacts", { headers: value ? { authorization: value } : undefined });
    await expect(requireServiceToken(request(`Bearer ${chalupaToken}`), "ingest")).resolves.toEqual({
      authentication: "publisher-token",
      kinds: ["chalupa.log-chunk"],
      maxSizeBytes: defaultProducerMaxSizeBytes,
      nativeSchemas: ["urn:chalupa:log-chunk:v1"],
      producerTool: "chalupa",
    });
    await expect(requireServiceToken(request(`Bearer ${nextChalupaToken}`), "ingest")).resolves.toEqual({
      authentication: "publisher-token",
      kinds: ["chalupa.log-chunk"],
      maxSizeBytes: defaultProducerMaxSizeBytes,
      nativeSchemas: ["urn:chalupa:log-chunk:v1"],
      producerTool: "chalupa",
    });
    await expect(requireServiceToken(request(`Bearer ${cairntraceToken}`), "ingest")).resolves.toEqual({
      authentication: "publisher-token",
      kinds: ["cairntrace.run"],
      maxSizeBytes: 32 * 1024 * 1024,
      nativeSchemas: ["urn:cairntrace.dev:run:v1"],
      producerTool: "cairntrace",
    });
    await expect(requireServiceToken(request(`Bearer ${chalupaToken}`), "admin")).rejects.toThrow("valid private service credential");
    await expect(requireServiceToken(request(`Bearer ${chalupaToken}`), "cron")).rejects.toThrow("valid private service credential");
    await expect(requireServiceToken(request(`Bearer ${chalupaToken}`), "read")).rejects.toThrow("valid private service credential");
    await expect(
      requireServiceToken(request(`Bearer ${"a".repeat(32)}`), "read"),
    ).resolves.toEqual({ authentication: "admin" });
    await expect(requireServiceToken(request(`Bearer ${"x".repeat(43)}`), "ingest")).rejects.toThrow("valid private service credential");
    await expect(requireServiceToken(request(`Bearer ${"x".repeat(4_097)}`), "ingest")).rejects.toThrow("valid private service credential");
    await expect(requireServiceToken(request(), "ingest")).rejects.toThrow("valid private service credential");
  });

  test("requires a signed token with exact issuer, audience, subject, and temporal claims", async () => {
    const issuer = "https://oidc.vercel.com/filecheap-auth-test";
    const audience = "https://vercel.com/filecheap-auth-test";
    const subject = "owner:filecheap-auth-test:project:chalupa:environment:production";
    const { privateKey, publicKey } = await generateKeyPair("RS256");
    const jwk = await exportJWK(publicKey);
    jwk.kid = "test-key";
    globalThis.fetch = (async (input) => {
      const url = typeof input === "string" ? input : input instanceof URL ? input.href : input.url;
      if (url !== `${issuer}/.well-known/jwks`) {
        throw new Error(`Unexpected OIDC request: ${url}`);
      }
      return Response.json({ keys: [jwk] });
    }) as typeof fetch;
    Object.assign(process.env, {
      VERCEL: "1",
      DATABASE_URL: "postgresql://runtime",
      FILECHEAP_ADMIN_TOKEN: "a".repeat(32),
      FILECHEAP_OWNER_ACCOUNT_ID: "acc_owner123",
      CRON_SECRET: "c".repeat(32),
      FILECHEAP_PUBLISHER_TOKENS: JSON.stringify({
        chalupa: {
          kinds: ["chalupa.log-chunk"],
          nativeSchemas: ["urn:chalupa:log-chunk:v1"],
          tokens: [chalupaToken],
        },
      }),
      FILECHEAP_OIDC_ISSUER: issuer,
      FILECHEAP_OIDC_AUDIENCE: audience,
      FILECHEAP_OIDC_SUBJECTS: subject,
    });
    resetConfigForTests();
    const token = (overrides: { audience?: string; expires?: boolean; subject?: string } = {}) => {
      let signer = new SignJWT({})
        .setProtectedHeader({ alg: "RS256", kid: "test-key", typ: "JWT" })
        .setIssuer(issuer)
        .setAudience(overrides.audience ?? audience)
        .setSubject(overrides.subject ?? subject)
        .setIssuedAt()
        .setNotBefore("0s");
      if (overrides.expires !== false) signer = signer.setExpirationTime("5m");
      return signer.sign(privateKey);
    };
    const request = async (credential: string) => requireServiceToken(new Request("https://file.cheap/api/v1/artifacts/plans", { headers: { authorization: `Bearer ${credential}` } }), "ingest");

    const oidcPrincipal = await request(await token());
    expect(oidcPrincipal).toEqual({
      authentication: "oidc",
      kinds: ["chalupa.log-chunk"],
      maxSizeBytes: defaultProducerMaxSizeBytes,
      nativeSchemas: ["urn:chalupa:log-chunk:v1"],
      producerTool: "chalupa",
      subject,
    });
    expect(() => requireAuthorizedArtifact(oidcPrincipal, {
      kind: "chalupa.log-chunk",
      producer: {
        native_schema: "urn:chalupa:log-chunk:v1",
        tool: "chalupa",
      },
    })).not.toThrow();
    expect(() => requireAuthorizedArtifact(oidcPrincipal, {
      kind: "cairntrace.run",
      producer: {
        native_schema: "urn:cairntrace.dev:run:v1",
        tool: "cairntrace",
      },
    })).toThrow("valid private service credential");
    await expect(request(chalupaToken)).resolves.toEqual({
      authentication: "publisher-token",
      kinds: ["chalupa.log-chunk"],
      maxSizeBytes: defaultProducerMaxSizeBytes,
      nativeSchemas: ["urn:chalupa:log-chunk:v1"],
      producerTool: "chalupa",
    });
    const readPrincipal = await requireServiceToken(
      new Request("https://file.cheap/api/v1/artifacts/downloads", {
        headers: { authorization: `Bearer ${await token()}` },
      }),
      "read",
    );
    expect(readPrincipal).toEqual({
      authentication: "oidc",
      kinds: ["chalupa.log-chunk"],
      maxSizeBytes: defaultProducerMaxSizeBytes,
      nativeSchemas: ["urn:chalupa:log-chunk:v1"],
      producerTool: "chalupa",
      subject,
    });
    expect(readPolicyFor(readPrincipal)).toEqual({
      kinds: ["chalupa.log-chunk"],
      maxSizeBytes: defaultProducerMaxSizeBytes,
      nativeSchemas: ["urn:chalupa:log-chunk:v1"],
      producerTool: "chalupa",
    });
    await expect(
      requireServiceToken(
        new Request("https://file.cheap/api/v1/artifacts/downloads", {
          headers: { authorization: `Bearer ${chalupaToken}` },
        }),
        "read",
      ),
    ).rejects.toThrow("valid private service credential");
    await expect(request(await token({ subject: "owner:filecheap-auth-test:project:other:environment:production" }))).rejects.toThrow("valid private service credential");
    await expect(request(await token({ audience: "https://vercel.com/other" }))).rejects.toThrow("valid private service credential");
    await expect(request(await token({ expires: false }))).rejects.toThrow("valid private service credential");
  });
});
