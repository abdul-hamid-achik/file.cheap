# ADR-001: Blob-first recovery prototype

- Status: accepted for `feat/cloud-local-prototype`
- Date: 2026-07-15
- Deployment: prohibited for this experiment unless separately authorized

## Context

file.cheap is local-first. The hosted hypothesis is not a generic cloud drive;
it is a verified remote-vault hypothesis for immutable agent artifacts. This
local experiment validates the protocol and recovery behavior before any claim
about off-device durability or reclaiming local disk.

The first technical question is whether file.cheap's archive format can move to
private object storage, recover reliably, and preserve integrity without pulling
cloud dependencies into the Go domain.

## Decision

Create one top-level `platform/` Next.js application. Use Bun, versioned REST,
Private Vercel Blob in a production-shaped adapter, and a filesystem adapter for
local development. Start with one workspace, a static development bearer token,
immutable SHA-256-addressed objects, and a small versioned catalog.

The control plane issues narrow transfer grants. In production, signed URLs move
archive bytes directly between the client and Blob. Route Handlers never proxy
large archive bodies.

Do not add Neon to this local recovery prototype. Blob plus an ETag-protected
catalog is enough to test one-workspace plan/upload/commit/download behavior.

## Safety invariants

1. The local product remains fully useful offline.
2. A stash ID may be committed repeatedly to the same hash, but never silently
   rebound to a different hash.
3. Uploads and catalog commits are separate, idempotent steps.
4. A failed network operation cannot mutate a local stash.
5. `HEAD`, byte count, and ETag prove object presence, not complete recoverability.
6. This prototype never evicts. A future encrypted client may consider eviction
   only after a complete hydrate-and-SHA-256 check and recovery-key export.
7. Local `drop` never implies remote deletion.

## Why `platform/`

`platform/` describes the optional hosted control plane and recovery surface
without confusing it with the existing public web property in `docs/` or with
Next.js's internal `app/` directory. It is one top-level application, not an
`apps/cloud` monorepo hierarchy. Vercel can later use `platform/` as the project
Root Directory for `cloud.file.cheap`.

## Neon decision

Neon is deferred, not rejected. Add a transactional database before multiple
external customers because object listing cannot safely coordinate ownership,
quota reservations, usage accounting, idempotency, tombstones, or billing.

Minimum beta tables would cover accounts/devices, vaults, immutable revisions,
upload reservations, a usage ledger, and tombstones. File bytes, plaintext
manifests, filenames, search indexes, and encryption keys do not belong there.

## Consequences

- The first vertical slice stays small and works without external credentials.
- A portable recovery card and local selected-file report make clean-session
  drills reproducible without telemetry or browser persistence. The report is
  deliberately marked non-tamper-evident and is not a server receipt.
- The object-store port keeps the Vercel choice replaceable.
- The catalog is explicitly unsuitable for production multi-tenancy.
- Auth, payments, teams, public sharing, background sync, and remote search stay
  out of scope.
- The Blob adapter is compile- and contract-tested but fails closed by default.
  A founder-only spike needs an explicit presence-only acknowledgement. User
  traffic remains blocked until staging and poisoned-path repair are designed.
