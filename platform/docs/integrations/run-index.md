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

`RunIndexV1` is deliberately small and bounded. It can describe:

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

This is a private, single-owner trial surface. It is not public sharing, team
access, billing, or continuous synchronization.
