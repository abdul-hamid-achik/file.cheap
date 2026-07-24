# file.cheap public-site deployment

This runbook releases the landing page, public platform, and VitePress
documentation as one deployment. It does not authorize or launch a hosted
multi-customer vault.

Production actions require an explicit release decision. The Vercel project is
connected to GitHub and `main` is its Production branch, so approval must happen
before merging or pushing a release commit to `main`; that push starts the
Production deployment automatically. Keep
`PLATFORM_RECOVERY_LAB_ENABLED=false` in Production.

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
| `PLATFORM_PUBLIC_URL` | Preview origin when testing the lab; otherwise optional | `https://file.cheap` |
| `PLATFORM_RECOVERY_LAB_ENABLED` | `false`; use `true` only for an access-protected lab review | Ignored and forced disabled; keep `false` or unset |

`VERCEL_ENV=production` is an unconditional deny boundary: Production keeps
the lab, OpenAPI document, and stateful routes closed even if
`PLATFORM_RECOVERY_LAB_ENABLED=true` is configured accidentally.

The public site and documentation need no Blob token, API bearer token, signing
secret, or database connection. Never add recovery credentials merely to make
the website render.

### Provisioned but disconnected Blob store

A private Blob store exists but is intentionally disconnected and is not part
of the public-site runtime. Its exact name, ID, region, and connection state
belong in the private inventory. If a controlled Blob Preview is explicitly
approved later, connect it only to the selected Preview environment. Keep
Production disconnected while the lab remains prohibited for user traffic.

### Provisioned but disconnected Neon project

A paid Neon project exists but is intentionally disconnected and is not part of
the public-site runtime. Its organization, plan, project, branch, compute,
region, scaling policy, and connection state belong in the private inventory.
Do not inject its connection string until transactional catalog code consumes
it and an access-protected Preview is explicitly approved.

## 1. Local release gates

From the repository root:

```sh
go test ./...
go vet ./...
CGO_ENABLED=0 go build ./cmd/fcheap
GOTOOLCHAIN=go1.26.5 go run github.com/abdul-hamid-achik/glyphrun/cmd/glyph@v0.15.0 \
  run e2e/flows/cli_artifact_ref.yml --format md

cd platform/docs
bun ci
bun run docs:verify
bun audit

cd ..
bun ci
bun run check
bun run build
bun run audit:production
```

The pinned Glyphrun runner requires Go 1.26.5, so `GOTOOLCHAIN` isolates that
one compatibility gate. file.cheap itself remains pinned to Go 1.25.12 by
`go.mod`.

The docs verifier must report 44 source pages plus 404. The integrated build
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

The existing Vercel project Root Directory must remain `platform`. Automatic Git
deployments are already enabled: a push to `main` creates a Production
deployment and may move the public aliases after the build becomes READY.
Changing project settings alone does not authorize a release.

The root `.vercelignore` is an allowlist for `platform/`. Keep it in place for
CLI deployments so Go binaries, release artifacts, local vault data, and other
repository-only files are never uploaded. A dry run should report roughly 150
source files and 1.2 MB before dependencies are installed remotely.

## 3. Create and verify a Preview

Keep the lab disabled and deploy from the repository root:

```sh
vercel --scope "$FILECHEAP_VERCEL_SCOPE" --yes
```

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
vercel curl /lab --deployment <preview-url>
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
- `/lab`, OpenAPI, and stateful recovery routes return 404 while the switch is
  false.
- `/api/v1/health` reports the lab and storage disabled without reading recovery
  credentials.
- Security headers are present and browser/runtime error logs are empty.

Test keyboard navigation, local search, mobile layout, a 404, and at least one
page from each docs section. Record the exact Preview evidence only in the
private inventory.

## 4. Production release

Do not run this section without explicit approval.

Because the existing private project owns both public domains and `main` is its
Production branch, the normal release action is to merge the fully verified
commit to `main` and push it. Do not also run a manual Production deployment
for the same commit.

```sh
git switch main
git merge --ff-only <verified-release-branch>
git push origin main
vercel inspect https://file.cheap --scope "$FILECHEAP_VERCEL_SCOPE"
```

Wait for both GitHub CI and the Vercel deployment to complete. Immediately
repeat the complete Preview matrix on `https://file.cheap` and
`https://www.file.cheap`. Verify TLS, both hosts, canonical and social metadata,
both sitemaps, VitePress search, the disabled lab, and runtime error logs for at
least 15 minutes.

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
