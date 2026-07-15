# AGENTS.md

Guidelines for the branch-scoped hosted-platform experiment.

## Scope and status

This file applies only inside `platform/`. The root product remains a local-first
Go CLI and MCP server. The user has authorized an isolated cloud prototype on
`feat/cloud-local-prototype`; it must not be deployed or merged into `main`
without a separate explicit decision.

Within this directory only, the root prohibition on HTTP servers and cloud SDKs
is narrowed so the prototype can contain a Next.js control plane and a Vercel
Blob adapter. Do not import this application or its dependencies from the Go
local-first core.

## Technology and boundaries

- Use Bun for installs and scripts. Do not add npm or Yarn lockfiles.
- Use Next.js App Router, strict TypeScript, and Route Handlers under
  `src/app/api/v1/`.
- Route Handlers authenticate, validate, call a feature service, and translate
  errors. Business rules belong in `src/features/`.
- Provider SDKs belong behind ports in `src/platform/`. Only the Vercel Blob
  adapter may import `@vercel/blob`.
- The local adapter stores disposable development data under `platform/.data/`.
- Use immutable, SHA-256-addressed archive objects. Never silently overwrite an
  object or bind one stash ID to two different hashes.
- Large archive bytes must use direct signed transfers in production; do not
  proxy them through a Vercel Function.
- API errors use `application/problem+json` and the RFC 9457 shape.

## Prototype constraints

- This is a single-workspace recovery prototype, not a production SaaS.
- A static bearer token is acceptable only for this local prototype.
- Do not add auth providers, payments, email, teams, continuous sync, public
  sharing, background deletion, or telemetry.
- Blob is not a production multi-tenant catalog. Add a transactional database
  before an external multi-customer beta.
- Never claim remote safety from `HEAD` or ETag alone. Local eviction requires a
  complete hydrate-and-hash verification plus a recovery-key export.

## Verification

Run from `platform/`:

```sh
bun run check
```

When the UI or Route Handlers change, also run the local server and exercise the
plan, upload, commit, list, download, and hash-verification path end to end.
