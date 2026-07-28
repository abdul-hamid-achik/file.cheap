# file.cheap platform deployment

This runbook releases the landing page, VitePress documentation, and private
single-owner artifact service as one platform deployment. It does not authorize
or launch a public, hosted, multi-customer vault.

Production actions require an explicit release decision. The Vercel project is
connected to GitHub and `main` is its Production branch. A push starts both the
Vercel build for that exact commit and an ordered GitHub Actions release that
verifies the same SHA and migrates Neon. The GitHub job named
`Production verification` and the job named `Production migration gate` must
both be required Vercel Deployment Checks. Automatic Production aliasing then
keeps the new build away from the public domains unless verification and the
migration both succeed.

The architectural decision behind this topology lives in the Obsidian vault at
`projects/file.cheap/ADR-003-public-site-and-docs-zones.md`.

Exact Vercel, Blob, and Neon organization, project, deployment, branch, store,
and compute identifiers are private operational inventory. They live in
`projects/file.cheap/2026-07-23-launch-readiness-and-chalupa-alignment.md` in
the private Obsidian vault and must not be copied into this public repository.

## One-project topology

| Field | Value |
| --- | --- |
| Vercel scope | Existing private scope; see the private inventory |
| Vercel project | Existing private project; see the private inventory |
| Root Directory | `platform` |
| Framework | Next.js |
| Public domains | `file.cheap`, `www.file.cheap` |

The `platform` build performs two steps:

1. build `platform/docs` with VitePress into `platform/public/_docs`;
2. build Next.js, which serves `/` and rewrites the historical documentation
   routes to that local static artifact.

`/_docs` is an internal namespace. Canonical URLs remain `/guide`, `/cli`,
`/mcp`, `/integrations`, `/learn`, `/compare`, and `/studio`.

There is no `FILECHEAP_DOCS_ORIGIN`, separate docs project, cross-project proxy,
or domain transfer.

### Rollback inventory

Before each release, inspect the current `file.cheap` deployment and record its
deployment ID, commit, immutable deployment URL, status, and verification
timestamp in the private inventory. Treat that inspected deployment as the
immediate rollback target. Never assume that an identifier copied from an old
runbook is still current.

## Required environment

Configure Preview and Production independently:

| Variable | Preview | Production |
| --- | --- | --- |
| `PLATFORM_PUBLIC_URL` | Preview origin | `https://file.cheap` |
| `DATABASE_URL` | Pooled Neon runtime URL for authenticated artifact routes | Pooled Neon runtime URL |
| `FILECHEAP_PLAN_RECEIPT_ACTIVE_KID` | Active Preview receipt key ID | Active Production receipt key ID |
| `FILECHEAP_PLAN_RECEIPT_SIGNING_KEYS` | Preview kid-to-base64url signing key map | Production kid-to-base64url signing key map |
| `FILECHEAP_PLAN_RECEIPT_LOOKUP_KEYS` | Preview kid-to-base64url lookup key map | Production kid-to-base64url lookup key map |
| `FILECHEAP_OIDC_*` | Exact Chalupa Preview issuer, audience, and subject | Exact Chalupa Production issuer, audience, and subject |
| `FILECHEAP_PUBLISHER_TOKENS` | Preview producer policies and credentials | Production producer policies and credentials |
| `FILECHEAP_ADMIN_TOKEN` | Distinct private administrator credential | Distinct private administrator credential |
| `FILECHEAP_OWNER_ACCOUNT_ID` | Opaque Preview owner ID | Stable opaque Production owner ID |
| `FILECHEAP_OWNER_EMAIL` | Allowlisted Preview owner email | Exact single-owner email |
| `FILECHEAP_AUTH_SECRET` | Distinct 32+ byte Preview HMAC secret | Distinct 32+ byte Production HMAC secret |
| `FILECHEAP_VERIFICATION_DELIVERY_LEASE_SECONDS` | Optional 30–300 second delivery lease | Optional 30–300 second delivery lease |
| `CRON_SECRET` | Distinct Vercel Cron credential | Distinct Vercel Cron credential |
| `BLOB_READ_WRITE_TOKEN` | Preview private Blob store credential | Production private Blob store credential |
| `RESEND_RECEIVE_API_KEY` | Not configured | Full-access key read only by current signed-webhook code |
| `RESEND_WEBHOOK_SECRET` | Not configured | Endpoint-specific Svix signing secret |
| `RESEND_FORWARD_TO` | Not configured | One private forwarding destination |
| `RESEND_AUTH_SEND_API_KEY` | Dedicated Preview key only while testing auth | Dedicated send-only console auth key |
| `RESEND_AUTH_FROM` | Verified Preview sender | Verified Production auth sender |

The public site and documentation do not read Blob, Neon, or private-service
credentials. The authenticated artifact service initializes them lazily.
Chalupa's Vercel service uses OIDC. External producers receive one
producer-bound credential as `FILECHEAP_INGEST_TOKEN`; the Vercel service keeps
the complete policy keyring in `FILECHEAP_PUBLISHER_TOKENS`. The legacy global
server-side `FILECHEAP_INGEST_TOKEN` is not accepted.

Receipt signing and lookup keys are separate JSON maps with exactly the same
1-16 safe key IDs. Every value is canonical unpadded base64url for 32-64 random
bytes, and no signing or lookup value may be reused anywhere in the keyring.
`FILECHEAP_PLAN_RECEIPT_ACTIVE_KID` selects the writer; readers try every loaded
key in a bounded set. To rotate, load the new distinct pair alongside the old
pair, deploy, and switch the active kid. Remove the old pair only after no
`planned` row still names that kid and every committed row for it has passed
`plan_expires_at`; an expired pending plan can renew until retention cleanup
claims it. There is no development or production fallback key.

Migration `0011_plan_receipt_hmac` is deliberately an expansion release. New
plans store the keyed receipt envelope and temporarily dual-write the original
UUID into `plan_token` so an older deployment can roll back safely. It does not
yet remove raw receipt storage. A later separately verified contraction may
null and eventually drop that column only after old instances and legacy rows
have been drained or keyed-backfilled.

The same exact Chalupa OIDC identity may request a signed download only for one
known committed `chalupa.log-chunk` produced by Chalupa with the allowlisted
native schema. The administrator credential retains unrestricted single-owner
operator access. Publisher credentials never authorize downloads, artifact
listing, metadata reads, administration, or retention.

Download authorization and retention are separate gates. The service returns an
expired committed artifact as not found even before the hourly reconciler has
deleted its object, and caps every signed GET grant at the artifact's own
`expiresAt`. The Vercel Blob adapter also rejects a presigned URL whose host,
operation path, object identity, or signed query is inconsistent with the
requested immutable artifact.

`FILECHEAP_PUBLISHER_TOKENS` is a compact JSON object keyed by the exact
`producer.tool`. Each value contains:

- `tokens`: one current 43-128 character base64url token, plus at most one next
  token during rotation;
- `kinds`: the exact artifact kinds that producer may plan and commit;
- `nativeSchemas`: the exact credential-free native schemas it may use;
- `maxSizeBytes` (optional): that producer's byte quota, between 1 and the
  67108864-byte global ceiling.

A producer that omits `maxSizeBytes` gets the conservative 8388608-byte default,
never the global ceiling; a larger quota is always an explicit decision. The
quota is enforced at plan time and rechecked at commit, so lowering it in the
keyring also stops an already-planned oversized upload. An over-quota request
returns `413` with a detail naming the producer and its exact quota; a request
above the global ceiling fails schema validation with `422`. The approved
allocation is Cairntrace 33554432; Glyphrun and Monitor each use 8388608 (the
default); and Chalupa's OIDC identity uses 8388608 (the default; it is not a
keyring entry). Monitor activation still requires its exact keyring entry.
Raising a quota is a configuration change, not a deployment: update
`FILECHEAP_PUBLISHER_TOKENS` for that producer only, and raise the matching
client-side constant in that producer's repository in the same release.

Keep the keyring bounded to configured producers only. Cairntrace currently
uses `cairntrace.run` with `urn:cairntrace.dev:run:v1`; Glyphrun uses
`glyphrun.evidence-pack` with `urn:glyphrun.dev:run:v1`; Monitor uses
`monitor.incident` with `urn:monitor.dev:incident:v1`. Add `fcheap` only when a
concrete private publishing workflow and exact kind/schema have been approved.
Do not create a wildcard producer, kind, or schema policy.

Rotate one producer independently: add its next token, deploy the keyring,
update only that producer's TinyVault secret, verify one complete
plan/upload/commit, then remove the old token and deploy again. Never place the
keyring in a producer, pass a token on a command line, or reuse a publisher
token as the administrator or cron credential.

The protected GitHub `production` Environment is restricted to `main` and
requires a reviewer. It holds only `MIGRATIONS_DATABASE_URL` as an encrypted
Environment secret. The release job validates that direct connection and
migrates the exact pushed SHA after one approval. Vercel's Git integration
builds that SHA. Its required `Production verification` and
`Production migration gate` Deployment Checks block domain assignment unless
both jobs succeed. GitHub Actions needs no Vercel token, organization ID, or
project ID. Keep the direct database URL out of Vercel and pull-request jobs.

### Email delivery

Email is optional and Production-only. Resend receives
`hello@file.cheap` through `POST /api/webhooks/resend` and forwards it to the
fixed `RESEND_FORWARD_TO` destination after raw-body signature verification,
exact-recipient filtering, loop prevention, durable digest replay suppression,
and provider idempotency. The route is a private provider callback exception,
not part of `/api/v1`. It fetches only bounded rendered metadata, omits all
attachments, and never stores or logs content, provider IDs, or the private
destination. This lowers duplicate risk but does not make an exactly-once
delivery promise.

Console authentication uses a dedicated `RESEND_AUTH_SEND_API_KEY` and
`RESEND_AUTH_FROM`; it never reuses the inbound receiving credential. Resend
idempotency keys bind retries to the exact authorization and send attempt. The
verification endpoint persists a fenced delivery claim before returning
`202`, then asks Next.js `after()` to dispatch it. That response confirms only
the durable claim, not provider acceptance. There is no autonomous outbox
consumer in this release: if deferred execution is lost, repeat the same
verification request after its 30–300 second lease expires. The regenerated OTP
and Resend idempotency key are identical, and an older worker cannot activate a
delivery after a newer worker reclaims it.

Keep open and click tracking disabled for the authentication sending domain in
Resend. Login messages are small transactional emails with both HTML and plain
text bodies, and every link must remain on the same `file.cheap` domain used by
the verified sender. Before promoting a template change, run Resend's
Deliverability Insights and test at least one iCloud recipient without exposing
local hostnames or other device identifiers in the message body.

The receiving-forward API requires a
team-wide full-access `RESEND_RECEIVE_API_KEY`; Resend cannot scope it to one
domain. Isolate it per product and keep all three runtime values Sensitive and
Production-only; current code reads them only in the signed webhook path.
Vercel Production environment variables are project/server-runtime scoped, not
per-route: other same-project server code could access `process.env`. True
route isolation requires a separate deployment or secret broker. Never copy
these values into Preview.

Before enabling mail, publish the exact Resend SPF, DKIM, MX, and DMARC records
for the selected receiving domain. Review the existing root MX first; if the
root domain needs a normal mailbox provider later, move reception to a
dedicated subdomain. Deploy the route, create one webhook at
`https://file.cheap/api/webhooks/resend` subscribed only to `email.received`,
and store its signing secret, the team-wide receiving key, and the destination
as Production-only Sensitive values. Apply the accompanying Drizzle migration
through the normal reviewed release gate before enabling the webhook.

Send one test message to `hello@file.cheap` after deployment. Confirm Resend
records the delivery, the callback returns `200`, the private recipient gets a
body-only copy with a valid Reply-To, and application logs contain no mail
metadata or content. A `401` indicates invalid signed bytes or secret; `413`
means the webhook envelope exceeded its limit; `502` means forwarding, a 2xx
provider receipt, or durable replay finalization was not completed (the
provider may already have accepted the message); and `503` indicates missing
email configuration. Do not put
the receiving credentials in Preview, browsers, CLI/MCP processes, or artifact
producers.

### Private Blob store

A private Blob store holds immutable artifact bytes. Its exact name, ID, region,
and connection state belong in the private inventory. It is used only by the
authenticated artifact service; public routes must never receive its credential.

### Neon metadata

A paid Neon project owns transactional artifact metadata. Its organization,
plan, project, branch, compute, region, scaling policy, and connection state
belong in the private inventory. Runtime uses `DATABASE_URL`; protected GitHub
Actions migration jobs use `MIGRATIONS_DATABASE_URL`, which must never be placed
in Vercel.

Before the first authenticated console test, after an owner-email recovery, and
before promoting changed owner configuration, run the read-only owner preflight
from `platform/` with the stable account ID, normalized owner email, and direct
migration connection:

```sh
FILECHEAP_OWNER_ACCOUNT_ID=acc_... \
FILECHEAP_OWNER_EMAIL=owner@example.com \
MIGRATIONS_DATABASE_URL=<direct-database-url> \
bun run db:check-console-owner
```

The command fails unless those values identify exactly one `console_users` row.
If migration credentials are unavailable, use a dedicated read-only
`CONSOLE_OWNER_CHECK_DATABASE_URL` instead; never configure either operational
connection in Vercel. Keep the account ID stable during email rotation, perform
the database change through the reviewed backfill procedure, and rerun this
preflight before promotion.

For the current low-traffic Launch baseline, keep both the project defaults and
the production endpoint at 0.25–1 CU with a 300-second autosuspend. Setting only
the existing endpoint is insufficient because a future branch or replacement
compute can inherit the project defaults. Increase those limits only from
observed latency, queueing, or compute-utilization evidence.

## 1. Local release gates

From the repository root:

```sh
go test ./...
go vet ./...
CGO_ENABLED=0 go build ./cmd/fcheap
GOTOOLCHAIN=go1.26.5 go run github.com/abdul-hamid-achik/glyphrun/cmd/glyph@v0.15.0 \
  run e2e/flows/cli_artifact_ref.yml --format md
GOTOOLCHAIN=go1.26.5 go run github.com/abdul-hamid-achik/glyphrun/cmd/glyph@v0.15.0 \
  run e2e/flows/cli_pull.yml --format md

cd platform/docs
bun ci
bun run docs:verify
bun audit

cd ..
bun ci
bun run test:postgres
bun run check
bun run db:check
bun run build
bun run audit:production
```

The pinned Glyphrun runner requires Go 1.26.5, so `GOTOOLCHAIN` isolates that
one compatibility gate. file.cheap itself remains pinned to Go 1.25.12 by
`go.mod`.

The docs verifier must report 48 source pages plus 404. The integrated build
must remove and regenerate `public/_docs`, serve both sitemaps, and preserve the
historical clean routes.

Canonical tags also require the `HOMEBREW_TAP_TOKEN` GitHub Actions secret.
The release workflow fails before GoReleaser when it is absent so a GitHub
release cannot silently omit the Homebrew cask. Fork tag workflows validate but
do not publish upstream; local GoReleaser snapshots may omit tap credentials.

## 2. Link the existing project

Run from the repository root. The project Root Directory selects `platform/`
for the remote build:

```sh
export FILECHEAP_VERCEL_PROJECT="<existing-project>"
export FILECHEAP_VERCEL_SCOPE="<existing-scope>"
vercel link --project "$FILECHEAP_VERCEL_PROJECT" --scope "$FILECHEAP_VERCEL_SCOPE" --yes
vercel project inspect "$FILECHEAP_VERCEL_PROJECT" --scope "$FILECHEAP_VERCEL_SCOPE"
```

Resolve both values from the private inventory. Confirm the returned project ID
matches that inventory exactly. Do not create another project.

The existing Vercel project Root Directory must remain `platform`. Preview Git
deployments and `main` Production builds remain enabled; `platform/vercel.json`
does not override Git deployment behavior. In the Production environment
settings, keep automatic aliasing enabled and add both GitHub jobs,
`Production verification` and `Production migration gate`, as required
Deployment Checks. Requiring both is important: if verification fails, the
dependent migration job is skipped and the verification check still blocks the
release. Vercel may build the pushed SHA while both gates run, but it must not
assign the Production domains unless both checks succeed. These check names are
a cross-system contract: if either workflow job is renamed, update the Vercel
Deployment Checks before merging the rename. Changing project settings alone
does not authorize a release.

The root `.vercelignore` is an allowlist for `platform/`. Keep it in place for
CLI deployments so Go binaries, release artifacts, local vault data, and other
repository-only files are never uploaded. A dry run should report roughly 150
source files and 1.2 MB before dependencies are installed remotely.

## 3. Create and verify a Preview

Use an isolated Preview database and Blob store. Apply its migration through a
reviewed, direct database connection before exercising private routes, then
deploy from the repository root:

```sh
cd platform
MIGRATIONS_DATABASE_URL=<direct-preview-url> bun run db:migrate
FILECHEAP_OWNER_ACCOUNT_ID=acc_<preview-owner> \
FILECHEAP_OWNER_EMAIL=<preview-owner-email> \
MIGRATIONS_DATABASE_URL=<direct-preview-url> \
bun run db:backfill-console-owner
cd ..

vercel --scope "$FILECHEAP_VERCEL_SCOPE" --yes
```

The historical upgrade gate for a database with legacy artifacts is: apply
`0003` through `0007`, run the owner backfill, verify zero null owners, then
apply `0008`. Migration `0008` makes ownership `NOT NULL` and adds foreign keys,
so it intentionally fails against an unbackfilled legacy database. A new empty
Preview can apply the complete graph before the idempotent backfill shown above.
Console queries never expose null-owner rows. Do not enable additional owner
accounts in this release.

After the Preview is READY, record its deployment ID, commit SHA, and automatic
URL. Exercise:

```sh
vercel inspect <preview-url>
vercel curl / --deployment <preview-url>
vercel curl /guide/getting-started --deployment <preview-url>
vercel curl /guide/getting-started.html --deployment <preview-url>
vercel curl /cli/ --deployment <preview-url>
vercel curl /mcp/overview --deployment <preview-url>
vercel curl /docs --deployment <preview-url>
vercel curl /assets/<reviewed-hashed-asset> --deployment <preview-url>
vercel curl /robots.txt --deployment <preview-url>
vercel curl /sitemap.xml --deployment <preview-url>
vercel curl /docs-sitemap.xml --deployment <preview-url>
vercel curl /api/v1/health --deployment <preview-url>
vercel curl /api/v1/openapi.json --deployment <preview-url>
vercel logs --deployment <preview-url> --level error --limit 50
```

Expected results:

- `/` is the public landing page with canonical `https://file.cheap/`.
- Every historical docs route returns its VitePress page and canonical URL.
- `/docs` and `/docs/` permanently redirect to `/guide`.
- VitePress local search, client navigation, CSS, fonts, images, and direct
  refreshes work on the Preview origin.
- `/sitemap.xml` contains the platform root.
- `/docs-sitemap.xml` contains docs routes and excludes the root.
- Direct `/_docs/*` responses carry `X-Robots-Tag: noindex, nofollow`.
- Hashed `/assets/*` responses are immutable.
- `/api/v1/health` reports the public site healthy without reading private
  storage or database credentials.
- `/api/v1/openapi.json` exposes the versioned private artifact contract without
  credentials.
- Private artifact routes reject missing or unapproved credentials. Exercise a
  complete plan, direct upload, commit, and signed download only from an
  explicitly allowlisted Preview OIDC subject; never paste a bearer credential
  into shell history or deployment evidence.
- Replay the exact committed plan and its original receipt before the receipt's
  `plan_expires_at`. Both recover the same verified artifact; after that bound,
  the plan replay still returns `200` without a new upload grant while the
  commit receipt returns `invalid_receipt`.
- Security headers are present and browser/runtime error logs are empty.

Test keyboard navigation, local search, mobile layout, a 404, and at least one
page from each docs section. Record the exact Preview evidence only in the
private inventory.

## 4. Production release

Do not run this section without explicit approval.

Because the existing private project owns both public domains and `main` is its
Production branch, the normal release action is to merge the fully verified
commit to `main` and push it. Vercel's Git integration builds that exact commit.
The `Production release` workflow serializes verification and the direct Neon
migration. Its `Production verification` and `Production migration gate` jobs
are the required Deployment Checks that release Vercel's automatic Production
alias only after both succeed. Do not run a second manual migration or
Production deployment for the same commit.

```sh
git switch main
git merge --ff-only <verified-release-branch>
git push origin main
gh run watch --workflow "Production release"
vercel inspect https://file.cheap --scope "$FILECHEAP_VERCEL_SCOPE"
```

Wait for GitHub CI, the ordered Production release, and Vercel's checked
Production deployment to complete.
Immediately repeat the complete Preview matrix on `https://file.cheap` and
`https://www.file.cheap`. Verify TLS, both hosts, canonical and social metadata,
both sitemaps, VitePress search, private-route authentication, one approved
artifact lifecycle, retention status, and runtime error logs for at least 15
minutes.

After that exact `main` commit is green, create the annotated release tag that
triggers `.github/workflows/release.yml`:

```sh
export FILECHEAP_RELEASE_TAG="v<semver>"
test "$(git rev-parse HEAD)" = "$(git rev-parse origin/main)"
git tag -a "$FILECHEAP_RELEASE_TAG" -m "file.cheap $FILECHEAP_RELEASE_TAG"
git push origin "$FILECHEAP_RELEASE_TAG"
```

Wait for the tag's Release workflow. Confirm that the GitHub release assets,
checksums, Debian/RPM packages, and Homebrew cask all carry the same version
before announcing it.

Record the deployment ID, Git commit, approver, timestamp, and verification
result.

Use `vercel --prod` only for an explicitly approved manual release or recovery
that cannot follow the normal Git path. Record why it was necessary.

Vercel's `Force Promote` action bypasses the required Deployment Checks,
including the migration gate. Treat it as emergency break-glass only, never as
a routine release path, and separately confirm schema compatibility before use.
Record the operator, approver, reason, commit, and result in the private
inventory.

## Rollback

Rollback stays inside the same Vercel project:

```sh
vercel rollback <known-good-deployment>
```

If the unified release fails before a newer known-good platform deployment
exists, roll back to the exact inspected baseline recorded in the private
inventory. Recheck `/`, `/guide/getting-started`, `/cli/`, both public domains,
TLS, and logs.

Do not delete the baseline deployment until the unified site has survived the
observation window and rollback has been rehearsed.
