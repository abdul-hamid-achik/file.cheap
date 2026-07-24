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

## One-project topology

| Field | Value |
| --- | --- |
| Vercel team | `The Lacanians` |
| Vercel project | `file-cheap` |
| Project ID | `prj_fnjqc2T8VT2lWHeznMWk7WoH0LyG` |
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

### Rollback baseline

The known-good docs-only Production deployment that was serving before this
consolidation is:

```text
deployment: dpl_ANfHYewYPFzKkQhSDB8jSYLgZg9G
commit:     1878dc90f926c93530418dae6c09b44fc64e88b4
origin:     https://file-cheap-hziatwuzq-the-lacanians.vercel.app
```

It belongs to the same `file-cheap` project and remains a rollback target until
the unified site has completed its observation window. It stopped serving the
public aliases when the unified `main` deployment became READY.

### Prior verified unified Production baseline

Before the ecosystem UX release, the latest verified unified deployment on the
existing project's Production branch was:

```text
deployment: dpl_9Cj9jtJDAZPvf8uj3MUkebE9wQJp
commit:     283dbcc84172df65ea93020e5b39876d39fadcab
origin:     https://file-cheap-fh5autisx-the-lacanians.vercel.app
aliases:    https://file.cheap, https://www.file.cheap
status:     READY
```

Treat this as the immediate rollback target for the next release. Use
`vercel inspect https://file.cheap --scope the-lacanians --format=json` to
resolve the live deployment after `main` advances; do not assume a hard-coded
deployment remains current.

An accidental project named `file-cheap-platform`
(`prj_hkdLMzqZmccAv3myWu2mVnjL2DeX`) was created while exploring the rejected
two-project topology. A final audit found no deployments, custom domains, Git
connection, or project-owned resources. It was permanently removed on
2026-07-23; do not recreate it.

## Required environment

Configure Preview and Production independently:

| Variable | Preview | Production |
| --- | --- | --- |
| `PLATFORM_PUBLIC_URL` | Preview origin when testing the lab; otherwise optional | `https://file.cheap` |
| `PLATFORM_RECOVERY_LAB_ENABLED` | `false`; use `true` only for an access-protected lab review | `false` |

The public site and documentation need no Blob token, API bearer token, signing
secret, or database connection. Never add recovery credentials merely to make
the website render.

### Provisioned but disconnected Blob store

| Field | Value |
| --- | --- |
| Name | `file-cheap-private-artifacts` |
| Store ID | `store_ymiqdgWHI6Oebjz2` |
| Access | Private |
| Region | `iad1` |
| Connected projects | None |

Creating the store does not enable the recovery laboratory. If a controlled
Blob Preview is explicitly approved later, connect it only to the selected
Preview environment of the existing `file-cheap` project. Keep Production
disconnected while the lab remains prohibited for user traffic.

### Provisioned but disconnected Neon project

The metadata database uses the existing paid Neon organization `personal`
(`launch`, the current paid Starter-equivalent plan):

| Field | Value |
| --- | --- |
| Project | `file-cheap` |
| Project ID | `damp-tree-95479480` |
| Primary branch | `main` (`br-late-forest-avyazp2g`) |
| Compute | `ep-dawn-surf-avarkl8w` |
| Region | `aws-us-east-1` |
| Autoscaling | `0.25` to `1` CU |
| Scale to zero | After 300 seconds |
| Connected Vercel projects | None |

Do not inject its connection string until transactional catalog code consumes
it and an access-protected Preview is explicitly approved.

## 1. Local release gates

From the repository root:

```sh
go test ./...
go vet ./...
CGO_ENABLED=0 go build ./cmd/fcheap

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

The docs verifier must report 44 source pages plus 404. The integrated build
must remove and regenerate `public/_docs`, serve both sitemaps, and preserve the
historical clean routes.

## 2. Link the existing project

Run from the repository root. The project Root Directory selects `platform/`
for the remote build:

```sh
vercel link --project file-cheap --scope the-lacanians --yes
vercel project inspect file-cheap --scope the-lacanians
```

Confirm the project ID exactly matches
`prj_fnjqc2T8VT2lWHeznMWk7WoH0LyG`. Do not create another project.

The Vercel project Root Directory must remain `platform`. Automatic Git
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
vercel --scope the-lacanians --yes
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
page from each docs section.

### Verified consolidated Preview

The first one-project Preview passed this matrix on 2026-07-23:

```text
deployment: dpl_8ukQAqTTVu1uXxyqLm87nwrSjT2S
status:     READY
environment: Preview
origin:     https://file-cheap-e0sxc03e9-the-lacanians.vercel.app
```

The Preview is protected by Vercel Authentication. It returned the landing
page, every docs section, clean and legacy HTML routes, both sitemaps, immutable
hashed assets, the expected security headers, and a disabled-lab health
contract. No error-level runtime logs were present. It is evidence for this
architecture, not authorization to promote its dirty working-tree snapshot.

## 4. Production release

Do not run this section without explicit approval.

Because the existing `file-cheap` project owns both public domains and `main` is
its Production branch, the normal release action is to merge the fully verified
commit to `main` and push it. Do not also run a manual Production deployment for
the same commit.

```sh
git switch main
git merge --ff-only <verified-release-branch>
git push origin main
vercel inspect https://file.cheap --scope the-lacanians
```

Wait for both GitHub CI and the Vercel deployment to complete. Immediately
repeat the complete Preview matrix on `https://file.cheap` and
`https://www.file.cheap`. Verify TLS, both hosts, canonical and social metadata,
both sitemaps, VitePress search, the disabled lab, and runtime error logs for at
least 15 minutes.

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
exists, roll back to the inspected docs-only baseline above. Recheck `/`,
`/guide/getting-started`, `/cli/`, both public domains, TLS, and logs.

Do not delete the baseline deployment until the unified site has survived the
observation window and rollback has been rehearsed.
