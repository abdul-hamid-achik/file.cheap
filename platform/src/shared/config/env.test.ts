import { afterEach, expect, test } from "bun:test";
import { getConfig, resetConfigForTests } from "@/shared/config/env";
import { defaultProducerMaxSizeBytes, maximumArtifactBytes } from "@/shared/config/limits";

const original = { ...process.env };

afterEach(() => {
  for (const key of Object.keys(process.env)) if (!(key in original)) delete process.env[key];
  for (const [key, value] of Object.entries(original)) process.env[key] = value;
  resetConfigForTests();
});

const publisherToken = "p".repeat(43);

test("rejects a publisher-only configuration in Vercel", () => {
  process.env.VERCEL = "1";
  process.env.DATABASE_URL = "postgresql://runtime";
  process.env.FILECHEAP_PUBLISHER_TOKENS = JSON.stringify({
    chalupa: {
      kinds: ["chalupa.log-chunk"],
      nativeSchemas: ["urn:chalupa:log-chunk:v1"],
      tokens: [publisherToken],
    },
  });
  process.env.FILECHEAP_ADMIN_TOKEN = "a".repeat(32);
  process.env.CRON_SECRET = "c".repeat(32);
  resetConfigForTests();
  expect(() => getConfig()).toThrow("require FILECHEAP_OIDC_*");
});

test("accepts an exact OIDC configuration without an ingest token", () => {
  process.env.VERCEL = "1";
  process.env.DATABASE_URL = "postgresql://runtime";
  process.env.FILECHEAP_ADMIN_TOKEN = "a".repeat(32);
  process.env.CRON_SECRET = "c".repeat(32);
  process.env.FILECHEAP_OIDC_ISSUER = "https://oidc.vercel.com/example";
  process.env.FILECHEAP_OIDC_AUDIENCE = "https://vercel.com/example";
  process.env.FILECHEAP_OIDC_SUBJECTS = "owner:example:project:chalupa:environment:production";
  resetConfigForTests();
  expect(getConfig().oidc?.subjects).toEqual(["owner:example:project:chalupa:environment:production"]);
});

test("accepts a bounded per-producer rotation keyring alongside Vercel OIDC", () => {
  process.env.VERCEL = "1";
  process.env.DATABASE_URL = "postgresql://runtime";
  process.env.FILECHEAP_ADMIN_TOKEN = "a".repeat(32);
  process.env.CRON_SECRET = "c".repeat(32);
  process.env.FILECHEAP_OIDC_ISSUER = "https://oidc.vercel.com/example";
  process.env.FILECHEAP_OIDC_AUDIENCE = "https://vercel.com/example";
  process.env.FILECHEAP_OIDC_SUBJECTS = "owner:example:project:chalupa:environment:production";
  process.env.FILECHEAP_PUBLISHER_TOKENS = JSON.stringify({
    cairntrace: {
      kinds: ["cairntrace.run"],
      maxSizeBytes: 32 * 1024 * 1024,
      nativeSchemas: ["urn:cairntrace.dev:run:v1"],
      tokens: ["r".repeat(43)],
    },
    chalupa: {
      kinds: ["chalupa.log-chunk"],
      nativeSchemas: ["urn:chalupa:log-chunk:v1"],
      tokens: [publisherToken, "n".repeat(43)],
    },
  });
  resetConfigForTests();
  expect(getConfig().publisherTokens).toEqual([
    {
      kinds: ["cairntrace.run"],
      maxSizeBytes: 32 * 1024 * 1024,
      nativeSchemas: ["urn:cairntrace.dev:run:v1"],
      producerTool: "cairntrace",
      tokens: ["r".repeat(43)],
    },
    {
      // An undeclared quota falls back to the conservative default, never to
      // the global ceiling.
      kinds: ["chalupa.log-chunk"],
      maxSizeBytes: defaultProducerMaxSizeBytes,
      nativeSchemas: ["urn:chalupa:log-chunk:v1"],
      producerTool: "chalupa",
      tokens: [publisherToken, "n".repeat(43)],
    },
  ]);
  expect(defaultProducerMaxSizeBytes).toBeLessThan(maximumArtifactBytes);
});

test("binds a Vercel deployment to one OIDC subject in its exact environment", () => {
  process.env.VERCEL = "1";
  process.env.VERCEL_ENV = "production";
  process.env.DATABASE_URL = "postgresql://runtime";
  process.env.FILECHEAP_ADMIN_TOKEN = "a".repeat(32);
  process.env.CRON_SECRET = "c".repeat(32);
  process.env.FILECHEAP_OIDC_ISSUER = "https://oidc.vercel.com/example";
  process.env.FILECHEAP_OIDC_AUDIENCE = "https://vercel.com/example";
  process.env.FILECHEAP_OIDC_SUBJECTS = "owner:example:project:chalupa:environment:preview";
  resetConfigForTests();
  expect(() => getConfig()).toThrow("exact Chalupa subject for VERCEL_ENV");

  process.env.FILECHEAP_OIDC_SUBJECTS = [
    "owner:example:project:chalupa:environment:production",
    "owner:example:project:chalupa:environment:preview",
  ].join(",");
  resetConfigForTests();
  expect(() => getConfig()).toThrow("exact Chalupa subject for VERCEL_ENV");
});

test("rejects malformed, weak, duplicated, or cross-scope publisher tokens", () => {
  delete process.env.VERCEL;
  process.env.DATABASE_URL = "postgresql://runtime";
  process.env.FILECHEAP_ADMIN_TOKEN = "a".repeat(43);
  process.env.CRON_SECRET = "c".repeat(43);

  for (const publisherTokens of [
    "{",
    JSON.stringify({}),
    JSON.stringify({
      chalupa: {
        kinds: ["chalupa.log-chunk"],
        nativeSchemas: ["urn:chalupa:log-chunk:v1"],
        tokens: ["short"],
      },
    }),
    JSON.stringify({
      chalupa: {
        kinds: ["chalupa.log-chunk"],
        nativeSchemas: ["urn:chalupa:log-chunk:v1"],
        tokens: [publisherToken, publisherToken],
      },
    }),
    JSON.stringify({
      cairntrace: {
        kinds: ["cairntrace.run"],
        nativeSchemas: ["urn:cairntrace.dev:run:v1"],
        tokens: [publisherToken],
      },
      chalupa: {
        kinds: ["chalupa.log-chunk"],
        nativeSchemas: ["urn:chalupa:log-chunk:v1"],
        tokens: [publisherToken],
      },
    }),
    JSON.stringify({
      "not a producer": {
        kinds: ["chalupa.log-chunk"],
        nativeSchemas: ["urn:chalupa:log-chunk:v1"],
        tokens: [publisherToken],
      },
    }),
    JSON.stringify({
      chalupa: {
        kinds: [],
        nativeSchemas: ["urn:chalupa:log-chunk:v1"],
        tokens: [publisherToken],
      },
    }),
    JSON.stringify({
      chalupa: {
        kinds: ["chalupa.log-chunk"],
        nativeSchemas: ["https://user:password@example.test/schema"],
        tokens: [publisherToken],
      },
    }),
    JSON.stringify({
      chalupa: {
        kinds: ["chalupa.log-chunk"],
        nativeSchemas: ["urn:chalupa:log-chunk:v1"],
        tokens: [publisherToken, "n".repeat(43), "o".repeat(43)],
      },
    }),
    JSON.stringify({
      chalupa: {
        kinds: ["chalupa.log-chunk"],
        maxSizeBytes: maximumArtifactBytes + 1,
        nativeSchemas: ["urn:chalupa:log-chunk:v1"],
        tokens: [publisherToken],
      },
    }),
    JSON.stringify({
      chalupa: {
        kinds: ["chalupa.log-chunk"],
        maxSizeBytes: 0,
        nativeSchemas: ["urn:chalupa:log-chunk:v1"],
        tokens: [publisherToken],
      },
    }),
    JSON.stringify({
      chalupa: {
        kinds: ["chalupa.log-chunk"],
        maxSizeBytes: 1.5,
        nativeSchemas: ["urn:chalupa:log-chunk:v1"],
        tokens: [publisherToken],
      },
    }),
    JSON.stringify({
      chalupa: {
        kinds: ["chalupa.log-chunk"],
        maxSizeBytes: "8388608",
        nativeSchemas: ["urn:chalupa:log-chunk:v1"],
        tokens: [publisherToken],
      },
    }),
    JSON.stringify({
      chalupa: {
        kinds: ["chalupa.log-chunk"],
        nativeSchemas: ["urn:chalupa:log-chunk:v1"],
        tokens: [publisherToken],
        unexpected: true,
      },
    }),
  ]) {
    process.env.FILECHEAP_PUBLISHER_TOKENS = publisherTokens;
    resetConfigForTests();
    expect(() => getConfig()).toThrow("FILECHEAP_PUBLISHER_TOKENS");
  }

  process.env.FILECHEAP_PUBLISHER_TOKENS = JSON.stringify({
    chalupa: {
      kinds: ["chalupa.log-chunk"],
      nativeSchemas: ["urn:chalupa:log-chunk:v1"],
      tokens: ["a".repeat(43)],
    },
  });
  resetConfigForTests();
  expect(() => getConfig()).toThrow("credentials must be distinct");
});

test("does not treat the legacy global ingest variable as server authentication", () => {
  delete process.env.VERCEL;
  process.env.DATABASE_URL = "postgresql://runtime";
  process.env.FILECHEAP_ADMIN_TOKEN = "a".repeat(32);
  process.env.CRON_SECRET = "c".repeat(32);
  process.env.FILECHEAP_INGEST_TOKEN = publisherToken;
  delete process.env.FILECHEAP_PUBLISHER_TOKENS;
  resetConfigForTests();
  expect(() => getConfig()).toThrow(
    "FILECHEAP_PUBLISHER_TOKENS or FILECHEAP_OIDC_*",
  );
});

test("rejects non-Vercel issuers, ambiguous issuer URLs, and empty subject allowlists", () => {
  process.env.VERCEL = "1";
  process.env.DATABASE_URL = "postgresql://runtime";
  process.env.FILECHEAP_ADMIN_TOKEN = "a".repeat(32);
  process.env.CRON_SECRET = "c".repeat(32);
  process.env.FILECHEAP_OIDC_AUDIENCE = "https://vercel.com/example";
  process.env.FILECHEAP_OIDC_SUBJECTS = "owner:example:project:chalupa:environment:production";

  process.env.FILECHEAP_OIDC_ISSUER = "https://issuer.example/example";
  resetConfigForTests();
  expect(() => getConfig()).toThrow("exact global or team-scoped Vercel issuer");

  process.env.FILECHEAP_OIDC_ISSUER = "https://oidc.vercel.com/example/";
  resetConfigForTests();
  expect(() => getConfig()).toThrow("exact global or team-scoped Vercel issuer");

  process.env.FILECHEAP_OIDC_ISSUER = "https://oidc.vercel.com/example";
  process.env.FILECHEAP_OIDC_SUBJECTS = " , ";
  resetConfigForTests();
  expect(() => getConfig()).toThrow("unique exact Chalupa Vercel deployment subjects");

  process.env.FILECHEAP_OIDC_SUBJECTS = "owner:example:project:other:environment:production";
  resetConfigForTests();
  expect(() => getConfig()).toThrow("unique exact Chalupa Vercel deployment subjects");
});
