#!/usr/bin/env bun
/**
 * Test-only harness, not part of the shipped product.
 *
 * Boots file.cheap's REAL Next.js Route Handlers for
 * `/api/v1/artifacts/plans` and `/api/v1/artifacts/commits` -- unmodified
 * production code, including the real `requireServiceToken` /
 * `requireAuthorizedArtifact` authorization decision in
 * `src/shared/auth/bearer.ts` -- against an in-memory repository so no real
 * Postgres is needed. The object store is a thin fake: `issueUploadGrant`
 * fabricates a URL shaped exactly like a real Vercel Blob upload grant (so a
 * caller's own response schema, which checks the shape of that URL, is
 * satisfied) but nothing is ever actually PUT there and bytes are never
 * verified. That is the only thing this harness stubs; the authorization
 * decision that chalupa's production incident depended on runs for real.
 *
 * Consumed by chalupa's `cloud/tests/ci-artifact-engine-parity.e2e.test.ts`,
 * which spawns this script as a subprocess (the sibling checkout's own
 * tsconfig/node_modules apply, avoiding an "@/*" alias collision between the
 * two independent repos) and talks to it over a real loopback HTTP server.
 *
 * Config arrives as a single JSON argv, matching this repo's own
 * `ci-artifact-engine-driver.py` convention on the chalupa side:
 *   { port, adminToken, cronSecret, ownerAccountId, issuer, audience,
 *     subject, jwk }
 * The bound origin is printed as one JSON line on stdout:
 *   { "url": "http://127.0.0.1:PORT" }
 * The process stays alive until it receives SIGTERM/SIGINT (the parent kills
 * it directly, the same way the Python engine driver's subprocess is reaped).
 */
import { POST as commitsPost } from "@/app/api/v1/artifacts/commits/route";
import { POST as plansPost } from "@/app/api/v1/artifacts/plans/route";
import { InMemoryArtifactRepository } from "@/features/artifacts/repository";
import { setArtifactServiceForTests } from "@/features/artifacts/factory";
import { testPlanReceiptKeyring } from "@/features/artifacts/plan-receipts.test-helper";
import { ArtifactService } from "@/features/artifacts/service";
import type {
  ArtifactObjectMetadata,
  ArtifactObjectStore,
  ArtifactTransferGrant,
} from "@/platform/artifacts/object-store";
import { resetConfigForTests } from "@/shared/config/env";

type HarnessConfig = {
  adminToken: string;
  audience: string;
  cronSecret: string;
  issuer: string;
  jwk: Record<string, unknown>;
  ownerAccountId: string;
  port: number;
  subject: string;
};

const config = JSON.parse(process.argv[2] ?? "{}") as HarnessConfig;
if (
  typeof config.port !== "number" ||
  !config.adminToken ||
  !config.cronSecret ||
  !config.ownerAccountId ||
  !config.issuer ||
  !config.audience ||
  !config.subject ||
  !config.jwk
) {
  throw new Error(
    "ci-parity-harness-server requires a JSON argv with port, adminToken, cronSecret, ownerAccountId, issuer, audience, subject, and jwk",
  );
}

// `getConfig()` only needs DATABASE_URL to be a non-empty string: the
// InMemoryArtifactRepository wired in below never touches it.
Object.assign(process.env, {
  DATABASE_URL: "postgresql://unused/ci-parity-harness",
  FILECHEAP_ADMIN_TOKEN: config.adminToken,
  FILECHEAP_OWNER_ACCOUNT_ID: config.ownerAccountId,
  CRON_SECRET: config.cronSecret,
  FILECHEAP_OIDC_ISSUER: config.issuer,
  FILECHEAP_OIDC_AUDIENCE: config.audience,
  FILECHEAP_OIDC_SUBJECTS: config.subject,
});
delete process.env.VERCEL;
resetConfigForTests();

// The only network call `requireServiceToken`'s OIDC path makes is jose's
// `createRemoteJWKSet` fetching `${issuer}/.well-known/jwks`. Intercept
// exactly that URL with the public JWK the parent process generated (and
// signed the test token with); let anything else fall through to the real
// fetch, matching the pattern file.cheap's own bearer.test.ts already uses.
const originalFetch = globalThis.fetch;
const jwksUrl = `${config.issuer}/.well-known/jwks`;
globalThis.fetch = (async (input, init) => {
  const url =
    typeof input === "string"
      ? input
      : input instanceof URL
        ? input.href
        : input.url;
  if (url === jwksUrl) {
    return Response.json({ keys: [config.jwk] });
  }
  return originalFetch(input as Parameters<typeof fetch>[0], init);
}) as typeof fetch;

/**
 * A real Vercel Blob upload grant is never issued or fetched here: this
 * fabricates a URL with the exact shape a caller's own response schema
 * expects (see `isExactVercelBlobUpload` on chalupa's side), scoped to the
 * real object key file.cheap's own `ArtifactService` computed. No bytes are
 * ever stored, downloaded, or verified.
 */
class HarnessArtifactObjectStore implements ArtifactObjectStore {
  readonly driver = "ci-parity-harness";
  async delete(): Promise<void> {}
  async inspect(): Promise<ArtifactObjectMetadata | null> {
    return null;
  }
  async verifySha256(): Promise<boolean> {
    return false;
  }
  async issueDownloadGrant(input: {
    key: string;
    validUntil: Date;
  }): Promise<ArtifactTransferGrant> {
    return {
      expiresAt: input.validUntil.toISOString(),
      headers: {},
      method: "GET",
      url: fakeVercelBlobUrl(input.key, 0),
    };
  }
  async issueUploadGrant(input: {
    contentType: string;
    key: string;
    sizeBytes: number;
    validUntil: Date;
  }): Promise<ArtifactTransferGrant> {
    return {
      expiresAt: input.validUntil.toISOString(),
      headers: { "content-type": input.contentType },
      method: "PUT",
      url: fakeVercelBlobUrl(input.key, input.sizeBytes),
    };
  }
}

function fakeVercelBlobUrl(objectKey: string, sizeBytes: number): string {
  const url = new URL("https://vercel.com/api/blob/");
  url.searchParams.set("pathname", objectKey);
  url.searchParams.set("vercel-blob-add-random-suffix", "false");
  url.searchParams.set("vercel-blob-allow-overwrite", "false");
  url.searchParams.set(
    "vercel-blob-maximum-size-in-bytes",
    String(sizeBytes),
  );
  url.searchParams.set("vercel-blob-delegation", "ci-parity-harness-delegation");
  url.searchParams.set("vercel-blob-signature", "ci-parity-harness-signature");
  return url.toString();
}

setArtifactServiceForTests(
  new ArtifactService(
    new HarnessArtifactObjectStore(),
    new InMemoryArtifactRepository(),
    testPlanReceiptKeyring,
  ),
);

const server = Bun.serve({
  hostname: "127.0.0.1",
  port: config.port,
  fetch: async (request) => {
    const pathname = new URL(request.url).pathname;
    if (pathname === "/api/v1/artifacts/plans") return plansPost(request);
    if (pathname === "/api/v1/artifacts/commits") return commitsPost(request);
    return new Response("not found", { status: 404 });
  },
});

process.stdout.write(`${JSON.stringify({ url: new URL(server.url).origin })}\n`);

// `Bun.serve` keeps the event loop alive on its own; the process exits only
// when the parent explicitly signals it (matching how the parent test
// terminates the subprocess it spawned).
function shutdown(): void {
  server.stop(true);
  process.exit(0);
}
process.on("SIGTERM", shutdown);
process.on("SIGINT", shutdown);
