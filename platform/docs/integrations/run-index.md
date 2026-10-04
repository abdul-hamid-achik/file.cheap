---
title: Run indexes for Cairntrace and Glyphrun
description: Publish a bounded metadata-only run index so the private file.cheap console can browse execution status and evidence health without opening an archive.
---

# Run indexes

The private file.cheap console can recognize Cairntrace and Glyphrun bundles as
runs. A trusted producer adds an optional `runIndex` to the immutable artifact
plan. The archive remains the source object; the index is only a safe projection
for browsing.

The hosted service does not unpack an archive to manufacture this projection.
It validates the index, hashes its canonical JSON, and stores it in the same
database operation as the artifact plan. The run becomes visible only after the
source artifact is committed and verified.

## What the index contains

`RunIndexV1` is deliberately small and bounded: the serialized sidecar is at
most 12 KiB, and `fcheap publish --run-index` refuses a larger file before it
sends any request. It can describe:

- the native run ID, stable series key, status, timestamps, environment, and
  backend;
- aggregate step, outcome, and artifact counts;
- evidence presence and integrity health;
- at most 100 outcome IDs with status;
- at most 200 evidence entries with relative path, role, medium, presence,
  integrity, sensitivity, and `metadata-only` inspectability.

The producer's `native_id` must match the indexed run ID. A Cairntrace producer
must submit a `cairntrace-run` detector result; Glyphrun must submit
`glyphrun-run`. The plan's content SHA-256 is stored alongside the index digest,
so an idempotency key cannot silently bind to another archive or projection.

## What it never contains

Do not put raw logs, terminal output, network captures, prompts, intent,
summaries, screenshots, videos, Markdown bodies, or secret values in the index.
Those bytes stay inside the private artifact and are available only through a
short-lived, authenticated download grant. Evidence entries are inventory
metadata, not previews.

Malformed or ambiguous native run data fails closed: file.cheap can still keep
the artifact as an ordinary bundle, but it will not invent a run record.

## Console behavior

After owner-email login, open `/console/runs` to filter the first alpha by
status, producer, and evidence health. A detail panel shows Summary, Outcomes,
Evidence, and Provenance. All reads include the authenticated owner ID in the
database predicate; another owner's ID returns the same not-found response as a
missing run.

## Restore a run from the console

The Summary tab of the run panel shows a **Restore this run** command with a
copy button. For a gzip run bundle (what Cairntrace and Glyphrun publish) it is
the exact sequence, with the real artifact ID and run ID filled in:

```bash
fcheap pull art_abcdefghijklmnop --output ./run-1.tar.gz && mkdir -p ./run-1 && tar -xzf ./run-1.tar.gz -C ./run-1
```

`fcheap pull` streams the bundle to a new local file and verifies its SHA-256
before keeping it; it never overwrites an existing file and never extracts. The
`tar` step is yours to run, so extract only bundles you trust. The console
derives the file and directory name from the run ID, replacing anything outside
letters, digits, `.`, `_`, and `-`. A bundle with another content type shows
the `fcheap pull` step alone. Run `fcheap auth login` on the machine first.

## Console links (`web_url`)

Cloud `ArtifactRefV1` documents returned by the artifact service carry an
optional `web_url`: an https link to the artifact in the console, such as
`https://file.cheap/console/artifacts/art_abcdefghijklmnop`. The origin is the
service's configured public origin, never a request header, and the link has no
query, fragment, or credential. It is a sign-in link, not a share link: opening
it redirects to the run panel when the artifact is an indexed run, otherwise to
the artifact panel, after owner login. The field is omitted when the public
origin is not https (local development). Consumers that cannot use it ignore it;
the contract treats it as a convenience, never as identity.

## Browse a run in Studio

`fcheap studio` shows the same evidence for local stashes whose bundle type is
`cairntrace-run` or `glyphrun-run`. See [Studio](/studio/overview#run-view-cairntrace-and-glyphrun-bundles).

This is a private, single-owner trial surface. It is not public sharing, team
access, billing, or continuous synchronization.
