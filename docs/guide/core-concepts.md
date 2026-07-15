# Core concepts

file.cheap separates irreplaceable snapshot data from indexes that can be
rebuilt. Understanding that boundary makes restore, cleanup, and agent use much
safer.

## A stash is an immutable snapshot

A **stash** is a saved file or directory tree plus its manifest. Saving the same
source again creates a new stash instead of silently replacing an older one.
Each stash receives an opaque ID that commands use as its address.

```bash
fcheap save ./test-results --tag checkout --tool manual
```

Copy the returned ID rather than constructing one. Commands reject path-like or
traversal IDs.

## The manifest is the portable source of truth

Every stash contains `manifest.json`. It records information such as:

- stash ID and display name;
- creation time, tool, tags, and source provenance;
- file paths, sizes, permissions, and content hashes;
- bundle type and compression state;
- expiry policy;
- save-time secret-scan findings.

The manifest travels with the payload. SQLite and veclite make the local vault
queryable, but neither derived index replaces the manifest or saved content.

## Payloads and derived indexes have different jobs

The default layout is:

```text
~/.local/share/fcheap/
├── <stash-id>/
│   ├── manifest.json
│   └── content/
│       (or content.tar.zst, content.tar.gz, or content.tar)
├── fcheap.db
└── fcheap.veclite
```

The stash directory is durable snapshot data. `fcheap.db` is a write-through
metadata index that can self-heal from manifests. `fcheap.veclite` is the local
per-file search index. [`vacuum`](/cli/vacuum) removes derived entries whose
stash directory no longer exists.

## Saving and indexing are separate operations

`save` copies content, hashes it, writes the manifest, and scans likely secrets
by default. `analyze` extracts readable text and adds one search document per
file.

Use `--index` when you want both in one workflow:

```bash
fcheap save ./evidence --tool manual --index
```

If indexing fails after the snapshot was saved, the payload and manifest remain
available. Inspect the reported status before retrying `analyze`.

## Search addresses saved files

Keyword search uses local BM25 retrieval and needs no model. An optional Ollama
or OpenAI embedder enables semantic and hybrid modes.

```bash
fcheap search "refresh token expired" --mode keyword
```

Results identify the stash and relative file that matched. Search snippets are
useful evidence, but they are not a substitute for reading or restoring the
complete file when full context matters.

OpenAI and non-loopback Ollama endpoints receive document text during indexing
and query text during semantic or hybrid search. The save-time scanner blocks
remote indexing of flagged stashes unless you explicitly opt in; it does not
scan search queries.

## Restore includes integrity verification

Restore materializes the payload and hashes every restored file against the
manifest.

```bash
fcheap restore <stash-id>
```

Without `--to`, file.cheap creates a fresh temporary directory. With `--to`, it
merges into that directory, replacing same-named files while leaving unrelated
files in place. A mismatch exits nonzero by default but leaves the restored data
available for inspection.

## Diff and connect answer different questions

[`diff`](/cli/diff) compares the saved tree with a **corresponding current
directory**. It answers: which relative paths were added, removed, or changed
since this snapshot?

[`connect`](/cli/connect) sends text derived from a stash to vecgrep to search a
**source repository**. It answers: which code locations are plausible leads for
this evidence?

Do not diff an unrelated screenshot or vidtrace bundle against an application
repository. Their directory structures describe different things.

## Expiry is not immediate deletion

A TTL records when a stash expires. Expired stashes are hidden from the default
list, but their bytes remain until an explicit cleanup command applies a plan.

```bash
fcheap ttl <stash-id> 30d
fcheap sweep          # preview
fcheap sweep --apply  # delete the reviewed expired plan
```

`cleanup` can score or categorize additional candidates. Its automatic deletion
rules remain deliberately narrower than its recommendations. A `keep` tag
protects a stash from sweep and cleanup application.

## Local-first is not the same as backed up

The local vault avoids a hosted dependency and keeps normal reads off the
network. It does not protect against loss of the machine or disk. file.cheap has
no shipped cloud sync service today, so independently design backup and key
recovery for evidence that must survive local hardware failure.

Continue with [Workflow examples](/guide/workflows), or use the
[CLI overview](/cli/) to choose a command.
