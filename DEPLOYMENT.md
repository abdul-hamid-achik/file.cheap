# file.cheap public-site deployment

This runbook prepares and releases the public Next.js website while preserving
the established VitePress documentation routes. It does not authorize or launch
the hosted remote vault.

Production actions require an explicit release decision. Do not set
`PLATFORM_RECOVERY_LAB_ENABLED=true` in Production.

The architectural decision behind this topology lives in the Obsidian vault at
`projects/file.cheap/ADR-003-public-site-and-docs-zones.md`.

## Project topology

| Project | Vercel Root Directory | Responsibility |
| --- | --- | --- |
| `file-cheap-platform` | `platform` | Public home, routing, root SEO files |
| existing docs project | `docs` | Immutable VitePress pages and assets |

Keep both projects in the same Vercel team and connected to the same Git
repository. Keep the existing docs project and its deployment history until the
new topology has been stable and rollback has been rehearsed.

### Inspected rollback baseline

On 2026-07-23, the docs-only production deployment inspected before this change
was:

```text
deployment: dpl_ANfHYewYPFzKkQhSDB8jSYLgZg9G
commit:     1878dc90f926c93530418dae6c09b44fc64e88b4
origin:     https://file-cheap-hziatwuzq-the-lacanians.vercel.app
```

It is a concrete pre-cutover fallback, not an instruction to use stale content.
Reinspect it and record the current known-good docs deployment at release time.

## Required platform environment

Configure these independently for Preview and Production:

| Variable | Preview | Production |
| --- | --- | --- |
| `FILECHEAP_DOCS_ORIGIN` | Reviewed immutable docs preview/production origin | Reviewed immutable docs production origin |
| `PLATFORM_PUBLIC_URL` | Preview origin when testing the lab; otherwise optional for the public site | `https://file.cheap` |
| `PLATFORM_RECOVERY_LAB_ENABLED` | `false`; use `true` only for an access-protected lab review | `false` |

`FILECHEAP_DOCS_ORIGIN` must be a bare HTTPS origin using the automatic URL of a
specific READY deployment. Do not use `file.cheap`, `www.file.cheap`, the
platform project alias, a branch alias, or a moving docs project alias.

The public website needs no Blob token, API bearer token, or signing secret while
the recovery laboratory is disabled. Never add production-shaped recovery
credentials merely to make the marketing site render.

### Provisioned but disconnected Blob store

The following team-level resource was created on 2026-07-23:

| Field | Value |
| --- | --- |
| Name | `file-cheap-private-artifacts` |
| Store ID | `store_ymiqdgWHI6Oebjz2` |
| Access | Private |
| Region | `iad1` |
| Vercel team | `The Lacanians` |
| Connected projects | None |

Do not connect this store to the docs project. Creating it does not enable the
recovery laboratory and no deployment currently receives its token. If a
controlled Blob Preview is explicitly approved later, connect it only to
`file-cheap-platform` and only to the selected Preview environment; Vercel will
then inject `BLOB_READ_WRITE_TOKEN`. Keep Production disconnected while the
laboratory remains prohibited for user traffic.

### Provisioned Neon project on the existing paid account

The metadata database was created on 2026-07-23 in the existing Neon
organization `personal`, whose plan is `launch` (the current paid
Starter-equivalent account), rather than as a Vercel-managed Free resource:

| Field | Value |
| --- | --- |
| Project | `file-cheap` |
| Project ID | `damp-tree-95479480` |
| Primary branch | `main` (`br-late-forest-avyazp2g`) |
| Compute | `ep-dawn-surf-avarkl8w` |
| Region | `aws-us-east-1` |
| Autoscaling | `0.25` to `1` CU |
| Scale to zero | After 300 seconds of inactivity |
| Vercel projects connected | None |

The short-lived Vercel Marketplace Free resource created during provisioning
was disconnected and deleted after the paid-account requirement was clarified.
Do not recreate a parallel Vercel-managed Neon project, and do not inject its
connection string merely for the current laboratory: the present implementation
does not consume Neon. Once transactional catalog code actually uses it and an
access-protected Recovery Lab Preview is explicitly approved, add the pooled
connection string only to that selected `file-cheap-platform` Preview
environment. Keep it out of the docs project and out of Production while the
laboratory remains disabled.

## 1. Local release gates

From the repository root:

```sh
go test ./...

cd docs
bun ci
bun run docs:verify
bun audit

cd ../platform
bun ci
bun run check
bun run audit:production
```

The docs verifier must report all source pages plus 404, exclude
`https://file.cheap/` from the docs sitemap, and confirm that `robots.txt`
advertises `https://file.cheap/docs-sitemap.xml`.

## 2. Prepare the docs deployment

1. Leave `file.cheap` and `www.file.cheap` on the existing docs project.
2. Set that project's Root Directory to `docs`.
3. Deploy the candidate commit.
4. Record its deployment ID, commit SHA, READY state, and automatic immutable
   `.vercel.app` URL.
5. Test the automatic URL directly:
   - `/guide/getting-started`
   - `/cli/`
   - `/assets/` resources referenced by those pages
   - `/sitemap.xml`
   - local search and navigation
6. Preserve the currently serving docs deployment URL as the topology rollback
   target.

Do not use the docs project production alias for `FILECHEAP_DOCS_ORIGIN`.

## 3. Create and verify a platform Preview

Create or update the `file-cheap-platform` project with Root Directory
`platform`. Set `FILECHEAP_DOCS_ORIGIN` to the immutable docs URL recorded above
and keep `PLATFORM_RECOVERY_LAB_ENABLED=false`.

After the Preview is READY, confirm its commit and run:

```sh
vercel inspect <platform-preview-url>
vercel curl / --deployment <platform-preview-url>
vercel curl /guide/getting-started --deployment <platform-preview-url>
vercel curl /cli/ --deployment <platform-preview-url>
vercel curl /docs --deployment <platform-preview-url>
vercel curl /robots.txt --deployment <platform-preview-url>
vercel curl /sitemap.xml --deployment <platform-preview-url>
vercel curl /docs-sitemap.xml --deployment <platform-preview-url>
vercel curl /lab --deployment <platform-preview-url>
vercel curl /api/v1/health --deployment <platform-preview-url>
vercel curl /api/v1/openapi.json --deployment <platform-preview-url>
vercel logs --deployment <platform-preview-url> --level error --limit 50
```

Expected results:

- `/` returns the public website with an indexable canonical
  `https://file.cheap/`.
- Historical docs routes return 200 with their original
  `https://file.cheap/...` canonical URLs.
- `/docs` and `/docs/` permanently redirect to `/guide`.
- VitePress JS, CSS, fonts, images, local search, and clean URLs work through the
  platform origin.
- `/robots.txt` advertises `/sitemap.xml` and `/docs-sitemap.xml`.
- `/sitemap.xml` contains the platform root; `/docs-sitemap.xml` contains docs
  routes and not the root.
- `/lab`, `/api/v1/openapi.json`, and all stateful recovery/catalog endpoints
  return 404 while the recovery switch is false.
- `/api/v1/health` returns 200 with `recoveryLab: "disabled"` and
  `storage: "disabled"` without reading recovery credentials or storage.
- Response security headers are present and the browser console has no CSP or
  mixed-content errors.
- Error logs are empty for the exercised routes.

Test keyboard navigation, mobile layout, a 404, and at least one page from every
docs section before proceeding.

## 4. Stage the Production platform artifact

Build a Production deployment using Production environment variables, but do
not assign the custom domains yet. A Preview promotion may rebuild with
Production variables; inspect and test the resulting Production deployment
rather than assuming it is byte-identical to Preview.

Record:

- platform deployment ID and immutable automatic URL;
- Git commit SHA;
- pinned docs deployment ID and origin;
- current docs rollback deployment ID and origin;
- the person approving the domain cutover.

Repeat the Preview matrix against the staged Production automatic URL.

## 5. Cut over the domains

Use Vercel's zero-downtime project-domain procedure within the same team:

1. Alias the staged platform deployment's **automatic URL** to `file.cheap`.
2. Alias the same deployment to `www.file.cheap`.
3. Verify both hostnames before changing project ownership.
4. Remove the domains from the docs project and add them to the platform
   project.
5. Configure `www.file.cheap` to redirect permanently to `file.cheap`.
6. Do not delete or rename the docs project or either recorded deployment.

Example shape—replace every placeholder with a recorded value:

```sh
vercel alias set <platform-automatic-url> file.cheap
vercel alias set <platform-automatic-url> www.file.cheap
```

Domain mutation is intentionally absent from CI.

## 6. Post-cutover verification

Repeat the complete route matrix on `https://file.cheap` and
`https://www.file.cheap`. Additionally verify:

- TLS and the `www` redirect;
- canonical, Open Graph, and Twitter metadata;
- both sitemaps and every URL in the docs sitemap;
- VitePress local search and hashed assets;
- no rewrite loop or accidental `.vercel.app` canonical;
- `/lab`, OpenAPI, and the stateful recovery/catalog endpoints remain closed;
- public health reports the lab and storage disabled;
- platform runtime error logs for at least 15 minutes.

Keep the release record and both rollback origins after the observation window.

## Rollback

Choose the smallest rollback that restores the failed boundary.

### Platform-code regression

Roll back to a previously serving platform deployment:

```sh
vercel rollback <known-good-platform-deployment>
```

Then repeat the public route and log checks.

### Docs-only regression

Create a platform deployment with `FILECHEAP_DOCS_ORIGIN` restored to the last
known-good immutable docs origin. Verify it before promoting it. Do not point the
variable at a mutable project alias.

### Routing or topology failure

Immediately alias both public hostnames back to the recorded immutable docs
deployment:

```sh
vercel alias set <known-good-docs-automatic-url> file.cheap
vercel alias set <known-good-docs-automatic-url> www.file.cheap
```

After traffic is restored, reassign the domains to the docs project in Vercel.
This is a cross-project rollback; `vercel rollback` in the platform project alone
cannot restore the former docs-only topology.

Document the failure, deployment IDs, timestamps, and verification result before
attempting another cutover.
