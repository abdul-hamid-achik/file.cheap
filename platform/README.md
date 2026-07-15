# file.cheap platform prototype

An isolated, single-workspace recovery prototype for the optional file.cheap
hosted product. It is intentionally not a production SaaS and is not deployed.

The application separates a small control plane from the archive data plane:

- `POST /api/v1/sync/plans` authenticates and creates a constrained upload plan.
- The client uploads an immutable archive through the returned signed URL.
- `POST /api/v1/sync/commits` verifies the object and binds it to a stash ID.
- `POST /api/v1/sync/downloads` issues a constrained download grant.
- `GET /api/v1/stashes` reads the current single-workspace catalog.

Local development uses a filesystem object-store adapter under `.data/`. The
production-shaped adapter uses Private Vercel Blob signed URLs, so large archive
bytes do not pass through a Vercel Function.

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

See [`architecture/ADR-001-blob-first-recovery-prototype.md`](architecture/ADR-001-blob-first-recovery-prototype.md)
for scope and the safety model.

## What this prototype proves

- deterministic SHA-256 object addressing;
- idempotent plan/commit semantics;
- explicit conflict handling;
- direct-transfer grants compatible with a future Go client;
- a catalog that can use ETag compare-and-swap without requiring Neon locally;
- full download-and-hash verification before a local copy could be evicted.

It does not prove multi-tenant isolation, transactional quotas, billing,
continuous synchronization, client-side encryption, or disaster recovery.
Those are hard gates before an external beta.
