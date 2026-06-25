# fcheap as the per-branch index-snapshot vault (for codemap + vecgrep)

> **Status:** design / proposed (2026-06-24). Authored from a codemap-side cross-ecosystem design pass.
> fcheap is **largely unchanged** here — its `save`/`restore`/`list`/`info` already do hash-verified,
> deduped, compressible snapshots with a `Custom` metadata map and tag filtering, which is exactly what a
> per-branch code-intelligence index snapshot needs. This doc defines the **shared contract** so codemap
> and vecgrep both stash/restore their indexes through fcheap with one stable schema.

## What gets stashed
codemap and vecgrep snapshot their per-branch index into an fcheap stash, keyed by **repo + branch +
base-sha**, on `git checkout`. On switching back, the target branch's snapshot is restored instead of a
full reindex.
- **vecgrep** snapshot content = the raw per-branch `vectors.veclite` file + a `snapshot.json` header.
- **codemap** snapshot content = a portable, store-agnostic serialization of its project slice
  (`nodes.jsonl` + `edges.jsonl` + `index_state.jsonl` + `annotations.jsonl` + `vectors.jsonl`) + a
  `snapshot.json` header. (codemap has a single shared `graph.db` + `codemap.veclite` sliced by project, so
  it can't copy a file — it serializes rows.)

## The contract (what fcheap must guarantee)

### Tag + Custom schema (one stash per `repo,branch,base-sha`)
```
fcheap save <snapshotDir> \
  --tool codemap|vecgrep \
  --name <repo>@<branch> \
  --tag codemap-index|vecgrep-index \
  --tag repo:<sha1(canonicalRepoRoot)[:12]> \
  --tag branch:<sanitized-branch> \
  --source <base-sha>            # git HEAD when the snapshot was taken → manifest.Custom["source"]
```
- `manifest.Custom["source"]` = base-sha · `manifest.Custom["tool"]` = codemap|vecgrep
- `snapshot.json` (inside the dir) carries `{schema_version, embedding_profile (provider:model:dims:distance), base_sha, project_name, counts}` — the importer **refuses** to restore if `embedding_profile` ≠ the live session's profile (never mix models; a veclite dim mismatch is a hard failure).

### Required fcheap behaviors (mostly already present)
1. **`fcheap save --json` MUST emit the stash ID** (and ideally the manifest) — the callers parse it to record the branch→stash pointer. *(Confirm/guarantee this contract.)*
2. **Content-hash dedup across stashes** — `manifest.json` already carries `ContentHash` + per-file `Hash`. Branches overlap heavily, so identical graph/vector exports must hash-collide and dedup automatically. ⚠ This requires the **callers** to write deterministic output (stable row ordering); fcheap's side is just honest per-file hashing.
3. **`restore` verifies** (`RestoreResult.Verified`) — codemap/vecgrep MUST check `Verified` before importing. Restore-by-stash-ID into a target dir.
4. **Auto-compress** (`compress_threshold`, default 10MB) should apply to snapshot dirs so large codemap graph exports compress on save.
5. **`list --tag`** filtering so a caller can rebuild its local pointer file from `fcheap list --tag codemap-index --tag repo:<hash>` (Custom has base-sha + branch).
6. **Graceful `ErrStashNotFound`** — `fcheap vacuum` could GC a snapshot a caller's pointer still references; restore must fail cleanly (caller treats as absent → reindex).

## Optional fcheap-side additions (recommended, not required)
1. **Convenience flags** `--branch` / `--profile` on `save` → write `man.Custom["branch"]` / `["embedding_profile"]` so `fcheap info` surfaces them without parsing tags. Backward compatible.
2. **`fcheap index-snapshot save|restore|list`** (new subcommand group in `internal/fcheap/cli/`) — a thin convenience over `save`/`restore` that bakes in the tag scheme (`tool`, `repo:<hash>`, `branch:<name>`, `source=<sha>`), so codemap/vecgrep call **one stable verb** and the dedup/compression policy lives in fcheap. Register in `root.go`; add `e2e/flows/cli_index_snapshot.yml`.
3. **`fcheap_index_snapshot` MCP tool** (or document that `fcheap_save` + tags is the contract) so an MCP-only agent path works.

## Why fcheap and not a plain copy
Branch snapshots overlap ~90%; fcheap's content-addressing means 20 branch snapshots cost far less than 20×. It also gives hash-verified restore (no silently-corrupt index), compression, and `diff` against the live tree as a **staleness check** (no changed files in indexed paths ⇒ the restored snapshot is still valid). This is the `EI.F` idea from `codemap-integration.md` made concrete.

## No daemon work
fcheap has no daemon role. Optionally, its content-addressed stash could later back a cross-branch shared embedding cache (so switches re-embed only the diff), but the throttle's in-memory dedup + per-branch veclite is sufficient — defer.
