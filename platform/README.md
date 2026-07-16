# file.cheap platform prototype

An isolated, single-workspace recovery prototype for the optional file.cheap
hosted product. It is intentionally not a production SaaS and is not deployed.

The application separates a small control plane from the archive data plane:

- `POST /api/v1/sync/plans` authenticates and creates a constrained upload plan
  for the fixed protocol-v1 media type `application/vnd.filecheap.stash`.
- The client uploads an immutable archive through the returned signed URL.
- `POST /api/v1/sync/commits` records the adapter's available object evidence
  and binds the object to a stash ID.
- `POST /api/v1/sync/downloads` revalidates object presence and adapter-level
  integrity evidence before issuing a constrained download grant.
- `GET /api/v1/stashes` reads the current single-workspace catalog.

Local development uses a filesystem object-store adapter under `.data/`. The
production-shaped adapter uses Private Vercel Blob signed URLs, so large archive
bytes do not pass through a Vercel Function.

`GET /api/v1/openapi.json` exposes the OpenAPI 3.1 contract intended for a
future streaming Go client. Responses produced by the documented application
handlers carry an `X-Request-Id`, and their errors use RFC 9457-style problem
details. Request objects are strict: unknown properties are rejected instead of
silently ignored, and JSON endpoints reject other request media types with a
typed `415`. Authentication failures advertise the Bearer challenge,
while transient catalog contention carries a short `Retry-After` hint. Commit
receipts are bounded before signature verification, and authentication and
signing credentials must be distinct.

## Run locally

```sh
cp .env.example .env.local
bun install
bun run dev
```

Open <http://127.0.0.1:3100>. API examples use the development bearer token:

```sh
curl -H 'Authorization: Bearer local-development-token' \
  http://127.0.0.1:3100/api/v1/stashes
```

Development credentials fail closed whenever `NODE_ENV=production` or Vercel
is detected. `PLATFORM_PUBLIC_URL` must be a bare HTTP(S) origin without
credentials, path, query, or fragment. Production requires HTTPS except for
an explicit loopback origin used by local verification.

See
[`ADR-001: blob-first recovery prototype`](architecture/ADR-001-blob-first-recovery-prototype.md)
for scope and the safety model, and
[`ADR-002: Stripe billing boundary`](architecture/ADR-002-stripe-billing-boundary.md)
for the proposed hosted Checkout and Customer Portal, internal Neon
billing/entitlement projection, quota, and recovery-first billing design.
ADR-002 authorizes no payment integration in this laboratory.

## What this prototype proves

- deterministic SHA-256 object addressing;
- idempotent plan/commit semantics;
- explicit conflict handling;
- deadline-governed, jittered catalog CAS retries under concurrent writers;
- direct-transfer grants compatible with a future Go client;
- a catalog that can use ETag compare-and-swap without requiring Neon locally;
- a portable recovery card;
- full download, SHA-256 verification, and a selected-file equivalence report.

Protocol v1 and the browser laboratory are capped at 64 MiB because this slice
uses one non-resumable transfer and hashes whole files in browser memory. A
future streaming Go client needs a separately versioned multipart/resume
contract before larger archives are claimed as supported. The prototype never
deletes or evicts a local stash.

Browser control-plane requests have a 30-second deadline; archive transfers
have a five-minute deadline and a visible cancel action. A successful commit is
kept successful even if the subsequent catalog refresh fails, so its generated
recovery card remains usable and the user is prompted to reconnect.

The exported drill report is a local client observation, explicitly marked
`tamperEvident: false`. It records what this browser checked; it is not a signed
server receipt and cannot prove which file a person selected.

The local adapter verifies SHA-256 while accepting an upload. Vercel Blob's
direct signed upload can prove presence, byte size, and an opaque ETag at commit
time, but not the client-declared SHA-256. The catalog records this distinction,
rechecks path, media type, size, and committed ETag before granting recovery, and
bypasses the Blob CDN cache for that signed GET. Recovery is proven only by a
complete client download and hash check.

The Blob adapter therefore fails closed unless a controlled spike explicitly
sets `PLATFORM_BLOB_INTEGRITY=presence-size-etag-experimental`. Before any user
traffic, replace deterministic final-path uploads with staging plus
verification/quarantine/repair; otherwise same-size incorrect bytes can occupy
an immutable pathname that `allowOverwrite: false` cannot repair.

It does not prove multi-tenant isolation, transactional quotas, billing,
continuous synchronization, client-side encryption, or disaster recovery.
Those are hard gates before an external beta.

## Verify

```sh
bun run check
bun run audit:production
```

The check uses Bun for linting, type checking, the unit/contract suite, a fresh
production Next.js build, and an isolated HTTP recovery E2E. The E2E starts the
production server twice against a temporary vault, exercises negative upload
and API cases, confirms persistence across restart, downloads every byte, and
verifies its SHA-256 before removing the temporary data.

The registry-backed audit is intentionally separate so offline development and
CI verification stay deterministic. The lockfile overrides Next.js's pinned
PostCSS with security-patched `8.5.10`; keep the override until Next.js itself
requires a non-vulnerable release.
