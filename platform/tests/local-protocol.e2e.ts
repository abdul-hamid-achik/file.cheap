import { createHash } from "node:crypto";

const baseUrl = process.env.PLATFORM_PUBLIC_URL ?? "http://127.0.0.1:3100";
const apiToken = process.env.PLATFORM_API_TOKEN ?? "local-development-token";
const bytes = new TextEncoder().encode(`file.cheap recovery e2e ${Date.now()}\n`);
const sha256 = hash(bytes);
const stashId = `local-e2e-${Date.now()}`;
const contentType = "application/vnd.filecheap.stash";

const health = await fetch(`${baseUrl}/api/v1/health`);
assert(health.ok, "health endpoint is unavailable");

const unauthorized = await fetch(`${baseUrl}/api/v1/stashes`);
assert(unauthorized.status === 401, "stashes endpoint must reject missing auth");

const plan = await api<{ receipt: string; state: string; upload: TransferGrant | null }>(
  "/api/v1/sync/plans",
  { contentType, sha256, sizeBytes: bytes.byteLength, stashId },
);
assert(plan.state === "upload_required", `unexpected plan state: ${plan.state}`);
assert(plan.upload, "new object did not receive an upload grant");

const upload = await fetch(plan.upload.url, {
  body: bytes,
  headers: plan.upload.headers,
  method: plan.upload.method,
});
assert(upload.ok, `upload failed: ${upload.status} ${await upload.text()}`);

const commit = await api<{ requiresFullVerification: boolean }>(
  "/api/v1/sync/commits",
  { receipt: plan.receipt },
);
assert(commit.requiresFullVerification, "commit did not require deep verification");

const repeated = await api<{ state: string; upload: TransferGrant | null }>(
  "/api/v1/sync/plans",
  { contentType, sha256, sizeBytes: bytes.byteLength, stashId },
);
assert(repeated.state === "already_committed", "idempotent plan was not recognized");
assert(repeated.upload === null, "idempotent plan issued an unnecessary upload");

const listing = await get<{ stashes: Array<{ stashId: string }> }>("/api/v1/stashes");
assert(listing.stashes.some((stash) => stash.stashId === stashId), "committed stash is not listed");

const download = await api<{
  expected: { sha256: string; sizeBytes: number };
  grant: TransferGrant;
}>("/api/v1/sync/downloads", { stashId });
const recoveredResponse = await fetch(download.grant.url, {
  headers: download.grant.headers,
  method: download.grant.method,
});
assert(recoveredResponse.ok, `download failed: ${recoveredResponse.status}`);
const recovered = new Uint8Array(await recoveredResponse.arrayBuffer());
assert(recovered.byteLength === download.expected.sizeBytes, "recovered size differs");
assert(hash(recovered) === download.expected.sha256, "recovered SHA-256 differs");
assert(hash(recovered) === sha256, "recovered bytes differ from the original upload");

console.log(
  JSON.stringify(
    {
      bytes: recovered.byteLength,
      deepVerification: "passed",
      sha256,
      stashId,
      storage: (await health.json()).storage,
    },
    null,
    2,
  ),
);

type TransferGrant = {
  headers: Record<string, string>;
  method: "GET" | "PUT";
  url: string;
};

async function api<T>(path: string, body: unknown): Promise<T> {
  const response = await fetch(`${baseUrl}${path}`, {
    body: JSON.stringify(body),
    headers: {
      authorization: `Bearer ${apiToken}`,
      "content-type": "application/json",
    },
    method: "POST",
  });
  if (!response.ok) {
    throw new Error(`${path} failed: ${response.status} ${await response.text()}`);
  }
  return response.json() as Promise<T>;
}

async function get<T>(path: string): Promise<T> {
  const response = await fetch(`${baseUrl}${path}`, {
    headers: { authorization: `Bearer ${apiToken}` },
  });
  if (!response.ok) {
    throw new Error(`${path} failed: ${response.status} ${await response.text()}`);
  }
  return response.json() as Promise<T>;
}

function hash(value: Uint8Array): string {
  return createHash("sha256").update(value).digest("hex");
}

function assert(condition: unknown, message: string): asserts condition {
  if (!condition) {
    throw new Error(message);
  }
}
