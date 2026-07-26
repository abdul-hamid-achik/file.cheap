# Changelog

All notable changes to file.cheap are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
Per-release binaries and notes are also on the
[GitHub releases page](https://github.com/abdul-hamid-achik/file.cheap/releases).

## [Unreleased]

## [0.32.1] - 2026-07-25

### Fixed

- Accepted Resend's documented `email.received` payload, which routes by
  `data.to` without guaranteeing `received_for`, while retaining exact
  recipient checks at both the signed webhook and authenticated Receiving API
  boundaries.

## [0.32.0] - 2026-07-25

### Added

- Added separate domain-scoped sending operations and a signed inbound Resend
  webhook that forwards only `hello@file.cheap` to one encrypted private
  destination.
- Added exact-recipient, anti-loop, raw-body, idempotency, and provider-failure
  coverage plus an English email operations guide.

### Fixed

- Pinned CI and release checkout actions to the reviewed Node 24 release.

## [0.31.1] - 2026-07-24

### Security

- Replaced the remaining workplace-specific ticket and provenance examples
  with neutral reserved fixtures.
- Added one repository privacy gate for tracked paths and content, enforced by
  local verification, CI, tagged releases, and the Production release gate.

## [0.31.0] - 2026-07-24

### Added
- **Private artifact transport for trusted services.** Authenticated producers
  can create immutable artifact grants, upload bytes directly to private Vercel
  Blob storage, and commit verified metadata in Neon without exposing storage
  credentials to workloads.
- **Bounded CLI publication.** `fcheap publish` sends one regular file through
  the private artifact protocol and emits a strict, credential-free
  `ArtifactRefV1` receipt.
- **Recoverable upload plans.** Exact idempotent plans can renew expired
  transfer grants, commit bytes left by an ambiguous upload, and restart after
  abandoned-plan cleanup. Retention now reclaims abandoned plans and stale
  deletion leases as well as expired committed artifacts. Exact committed plan
  replays and their original receipts recover the durable artifact without
  issuing another transfer grant.

### Changed
- **Producer-bound private authentication.** External publisher credentials are
  independently rotatable and bound to exact producer, kind, and native-schema
  allowlists. Chalupa OIDC remains the preferred Vercel boundary and can request
  a signed download only for its own retained log-chunk artifacts; publisher
  credentials cannot read, administer, or run retention. Expired artifacts are
  no longer downloadable before reconciliation, and signed GET grants cannot
  outlive artifact retention.

### Removed
- Removed the experimental browser recovery service, its feature switch, local
  object adapter, stateful routes, and archived interface styles. The public
  site remains local-first; the hosted artifact API is a private service
  boundary for explicitly trusted producers.

## [0.30.0] - 2026-07-23

### Added
- **Versioned local artifact handoff.** `fcheap artifact-ref` and the
  `fcheap_artifact_ref` MCP tool emit a strict `ArtifactRefV1` with
  credential-free transport fields for an existing local stash so Cairntrace,
  Glyphrun, Chalupa, and other tools can exchange provenance without moving
  artifact bytes.
- **Version-matched agent guidance.** `fcheap agent`, MCP
  `fcheap://agent-guide`, and the embedded documentation give clients one
  machine-readable operating contract for safe local vault workflows.
- **Unified public product site.** The landing page and VitePress documentation
  now ship from the existing `file-cheap` Vercel project at `file.cheap`.

### Changed
- The public guides now document the implemented Chalupa artifact sidecar,
  Cairntrace's raw run-ID join, Glyphrun packs, first-ingest immutability, retry
  behavior, and the local-vault availability boundary.

### Fixed
- Artifact-reference conformance now rejects mismatched cloud identities,
  empty native schema URNs, and HTTP(S) ports outside the supported range.
- CLI and MCP artifact-reference producers verify an optional entrypoint
  against saved stash content before returning a usable reference.
- Vercel Production excludes unfinished stateful browser routes, and canonical
  releases fail before publishing when the Homebrew tap credential is absent.

## [0.29.0] - 2026-07-12

### Added
- **Embedded read-only docs.** `fcheap docs list/show` and MCP `fcheap_docs`
  list/show now work from installed binaries without a source checkout.
- **Verified-recovery controls.** `restore --allow-mismatch` and MCP
  `allow_mismatch` let you explicitly accept restored files that fail manifest
  verification; strict verification remains the default.
- **Remote-secret policy.** The new `allow_remote_secrets` config key defaults to
  false and provides an explicit opt-in for remote embedding of flagged stashes.

### Changed
- **Cleanup is conservative by contract.** Applied cleanup auto-deletes only
  expired TTLs or regenerable `codemap`/`vecgrep` caches. Sweep keeps `--auto`
  dry-run unless `--apply` is also present. Smart cleanup and expired-TTL sweep
  JSON distinguish the plan from dropped, failed, and skipped operations.
- **Configuration follows XDG path semantics.** fcheap honors
  `XDG_CONFIG_HOME`, expands `~`, and resolves relative configured paths from the
  config directory. `ecosystem-status` now separates logical size from stored
  size for compressed stashes.
- Release validation now builds the documentation and scans reachable Go code
  for known vulnerabilities; release archives include the license.
- Save, drop, cleanup, and sweep now emit their complete structured result
  before returning nonzero for partial post-save, metadata, or index failures.

### Removed
- Removed the misleading cleanup `--projects-dir` / `--notes-dir` flags and
  corresponding MCP input fields. Orphan analysis now uses only the recorded
  source path; optional project and note layouts are not deletion evidence.

### Fixed
- Restore verification mismatches now produce a nonzero CLI result and an MCP
  error result unless explicitly allowed, while still returning the full status.
- `sweep --include-tag` filters the plan before deletion. Smart cleanup and
  expired-TTL sweep report planned IDs separately from IDs actually dropped.
- Search now covers mixed keyword/vector vaults, hides expired or missing
  stashes, and records indexing success only after the index is durable.
- Existing metadata databases migrate and validate safely at startup, and Studio
  clears stale previews while new or empty results load.
- `fcheap docs build --output` now sends the requested output directory to
  VitePress instead of only printing it.

### Security
- Embedded documentation readers reject absolute, traversal, and non-canonical
  page paths before lookup.
- Remote indexing blocks stashes flagged by the save-time secret scanner by
  default. BM25 and loopback Ollama remain local; non-loopback Ollama endpoints
  follow the same opt-in policy as OpenAI.
- CI and release builds require Go 1.25.12 or newer so shipped binaries include
  the latest Go 1.25 standard-library security fixes.
- Save and restore reject paths that overlap the vault, including symlinked
  paths. The vault remains private, stash IDs are collision-resistant, and file
  copies reject unsupported special files and avoid following planted links.

## [0.28.0] - 2026-07-07

### Added
- **`save --index`** auto-indexes a stash for search right after saving, so
  callers can search it without a separate `fcheap analyze` step. The MCP
  `fcheap_save` tool exposes the same `index` field.
- **Structured `line` on `connect`/`search` matches.** Vecgrep matches carry a
  clean `file` path plus a separate integer `line` field.

### Changed
- **`search`/`connect` return exit 0 when nothing is indexed.** With `--json`,
  `search` returns `[]` and `connect` returns
  `{"matches":[],"index_status":"missing"}`, reserving nonzero exits for real
  failures. MCP uses the same empty-result contract.

## [0.27.0] - 2026-07-06

### Added
- **Multi-tag AND list filtering.** `fcheap list --tag` is now repeatable (AND
  across flags, comma-separated), so `fcheap list --tag codemap-index --tag
  repo:<hash>` matches stashes containing every listed tag — the contract codemap's
  per-branch index cache needs. Mirrors `save --tag`. `ListOptions.Tags []string`
  added; the legacy single `Tag` is kept and merged. The MCP `fcheap_list` tool
  gains `tags []string` (AND) alongside the legacy `tag`.
- **`custom` in list output.** `fcheap list --json` and the MCP `stashSummary`
  (shared by `fcheap_list` and the `fcheap://stashes` resource) now surface the
  full `manifest.Custom` map, so a caller rebuilds its pointer file from `list`
  alone with no per-stash `info` calls.

### Changed
- Bumped veclite to v0.22.1 (HNSW panic fix) and v0.22.0 (lock-free read-only
  opens) for safer parallel MCP search.

## [0.26.2] - 2026-06-29

### Internal
- Pinned goreleaser to `~> v2` (was `latest`) and updated GitHub Actions to the
  latest versions (fixes Node.js 20 deprecation). No user-facing change.

## [0.26.1] - 2026-06-29

### Fixed
- Replaced `WriteString(Sprintf)` with `Fprintf` in the diff output path
  (avoids an intermediate allocation).

## [0.26.0] - 2026-06-25

### Added
- **TTL expiry and smart cleanup.** Stashes can carry a TTL (`--ttl 7d, 24h,
  30d`) and are hidden/expired automatically; `fcheap sweep` (expired) and
  `fcheap cleanup` (scoring + category-based smart analysis) reclaim space, with
  TUI integration and per-tool TTL rules.

## [0.25.0] - 2026-06-24

### Changed
- veclite `WithSharedRead` for parallel MCP search (read-only opens are
  lock-free), so concurrent `fcheap_search` no longer serializes on the index.

## [0.24.1] - 2026-06-23


### Fixed
- **Studio: video player hardening.** Pressing `p` on a compressed stash is now
  refused with a clear message instead of silently spinning a never-rendering
  decode loop; a frame that fails to decode mid-playback stops cleanly; pausing
  shows a `⏸` indicator instead of a misleading `▶`.
- **Studio: preview races.** Starting playback, or switching to the diff/timeline
  view, can no longer be clobbered by a file/result preview that finished decoding
  late.
- **Studio: narrow terminals.** The preview viewport is sized to the true panel
  interior, so image / timeline / diff content no longer wraps or overflows on
  narrow widths; terminals under 20 columns show a concise notice rather than
  broken boxes.
- **Studio: search results** — `g` / `G` jump to the first / last match (parity
  with the list and files panes).

## [0.24.0] - 2026-06-23

### Added
- **Studio: inline image previews.** PNG / JPEG / GIF files now render directly in
  the preview pane as truecolor half-block art (decoded with the Go standard
  library — no external tools), instead of "(binary file — not previewable)". Works
  in the detail Files pane and on search hits — including vidtrace per-frame hits
  (`frames/f.png @ 12s`), so searching frame OCR shows the matching frame. Renders
  in Ghostty and any truecolor terminal, with a `format · dimensions · size` caption.
- **Studio: frame-sequence video player.** Press `p` (or `space`) in a stash detail
  view to animate a vidtrace bundle's frames in the preview pane at the source
  frame rate (capped for terminal smoothness). Playback loops, shows a `frame N/M`
  counter, and the Files-pane cursor tracks along; any other key stops it.

### Fixed
- **Studio: the Files pane now scrolls.** Its scroll window was sized to the whole
  terminal rather than the panel, so once the cursor passed the fold the list
  appeared frozen. Added page (`pgdn`/`pgup`, `ctrl+d`/`ctrl+u`) and jump
  (`g`/`G`, `home`/`end`) navigation to the file list and stash list.
- **Studio: returning from a diff** no longer drops you into a detail view that
  still shows the diff text with a stale or off-screen file cursor — the pane is
  reset and the selected file reloaded.
- **Studio: preview correctness.** Navigating files quickly no longer shows the
  wrong file (out-of-order async loads are now discarded), and the preview is
  refreshed after compressing the selected stash.
- **Studio: layout.** File rows and panel titles no longer wrap/overflow on
  narrower wide-layout widths, and the timeline and diff views now fill the full
  pane width instead of half.
- **Studio: keybinding/hint consistency.** Footer hints are focus-aware (`d` drop
  vs. pager, `m` search mode); `g`/`G`/`home`/`end` work in the diff, timeline, and
  search preview panes; `h` goes back in search; the stash list gains `g`/`G`
  first/last with refresh moved to `R`. The help screen matches the actual keys.

## [0.23.0] - 2026-06-23

### Added
- **Studio: richer detail pane.** The per-stash provenance is now grouped into
  color-coded sections — IDENTITY / PROVENANCE / CONTENT / STORAGE — showing the
  tool (colored), relative + absolute created time, the compression space-saved
  ratio, and indexed status (✓ analyzed with doc count, or not indexed). The
  provenance is height-capped so the file tree and preview always stay visible.

## [0.22.0] - 2026-06-23

### Added
- **Studio: live list filter** (`f`) — type to narrow the stash list by name /
  tool / tag; `enter` keeps it, `esc` clears. The cursor and every list action
  operate on the filtered view, with a "N of M" line and a "no match" state.

### Fixed
- **Studio layout** (from an adversarial multi-agent TUI review): the list panel
  no longer drops its bottom border when the list overflows; chip-bearing rows no
  longer wrap at narrow widths; the selected-row highlight matches the panel
  interior; the footer wraps to the terminal width instead of forcing the UI
  wider than the screen; and very short terminals no longer render past the
  bottom.

## [0.21.0] - 2026-06-23

### Added
- **Studio: sort the stash list** — `o` cycles age / name / tool / files / size
  and `O` reverses the direction; the active sort and direction show in the panel
  title (`Stashes · SIZE ▼`) and the sorted column is highlighted in the header.
- **Studio: per-tool color accents** in the list (the TOOL column is colored by
  producing tool), with dimmed size/age columns for quicker scanning.

## [0.20.0] - 2026-06-23

### Added
- **Studio TUI now fills the terminal.** Every view expands to the full height
  between the header and a bottom-pinned footer (previously content sat at the
  top with most of the terminal unused). The stash list is now a proper table —
  aligned `NAME/TOOL/FILES/SIZE/AGE` columns, a NAME column that flexes to the
  available width, right-aligned numbers, chips reserved on the right, a "… N
  more" overflow indicator, and the stash count + total size in the header.
  Detail/search size their preview to the available space.

## [0.19.2] - 2026-06-23

### Security
- Restore with no `--to` now uses a fresh, unique temp directory
  (`os.MkdirTemp`) instead of a predictable `os.TempDir()/<id>` path, so nothing
  can be pre-planted at a known restore destination and repeated restores don't
  merge.
- `Extract` now enforces a total-extraction byte cap (defense-in-depth against a
  decompression bomb that would otherwise fill the disk).

## [0.19.1] - 2026-06-23

Security & robustness fixes from a focused security audit.

### Security
- **Reject path-traversal stash IDs (high).** An unvalidated `stash_id` flowed
  into `filepath.Join(rootDir, id)` then `os.RemoveAll`/restore, so an MCP agent
  calling `fcheap_drop(stash_id: "../../dir", force: true)` had an arbitrary
  directory-deletion primitive. IDs are now validated as a single, non-traversal
  path element across `Drop`/`Restore`/`Info`/`Exists` (and the MCP `diff` tool).
- **Block symlink-escape on restore (medium).** `copyFile` and `Extract` followed
  a symlink pre-planted at a restore destination (no `O_NOFOLLOW`), so a write
  could clobber a file outside the target — they now remove a pre-existing
  symlink before writing. `copyDir` drops absolute/`..`-escaping links on restore
  (matching `Extract`) while still preserving them verbatim on save.

### Changed
- **Atomic writes:** `manifest.Save` uses temp-file + fsync + rename, and
  `Compress` was reordered (rename archive → record manifest → reclaim) so an
  interrupted compress can't leave the manifest/DB diverged from disk.
- **Bounded memory:** the generic and vidtrace detectors cap `SearchableText`
  (4 MiB, via `strings.Builder` instead of O(n²) `+=`), and bundle JSON reads are
  size-capped (32 MiB).

## [0.19.0] - 2026-06-22

Fixes from an exhaustive multi-agent code/docs audit.

### Fixed
- **compress:** `Archive()` discarded final-flush/close errors, so a failed
  flush could be reported as success and `Compress()` would then delete the
  source tree (data loss). Errors are now checked before success is reported.
  `Archive` also no longer crashes on symlinks, and `Extract` recreates them
  safely (rejecting escape links) rather than dropping them.
- **stash/manifest:** `Save()` aborted on a source directory containing a
  dangling symlink; symlinks are now recreated/hashed without dereferencing.
- **analyze:** an embedding-model change bricked all search (incl. keyword);
  search now degrades to BM25 on drift. veclite access is serialized per stash
  root so concurrent MCP calls no longer fail on the file lock, and search/index
  honor context cancellation.
- **config:** `config set`/`config init` no longer bake env/flag overrides into
  `config.yaml`; `config init` requires `--force` to overwrite; `config set`
  validates compression/log_level; `config show --json` emits snake_case.
- **drop:** `drop --json`/`--quiet` without `--force` now returns a non-zero
  error and a structured object instead of a silent exit-0 no-op.
- **version:** `version --json` now emits JSON (was ignored).
- **studio:** `d`/`u` page the preview when it is focused instead of dropping.
- **secrets:** scan no longer silently truncates on an over-long single line.
- **mcp:** the `fcheap://stash/{id}` resource surfaces real read errors instead
  of masking them all as not-found.
- **detect:** vidtrace OCR/transcript text is no longer double/triple-indexed
  (raw `ocr/`/`transcript/` files are skipped when the timeline already yields
  per-frame units), which previously skewed BM25 scores.
- **analyze:** unknown search modes are rejected with a clear error (were
  silently treated as keyword); `DropIndex` no longer creates an empty index DB
  for a never-indexed stash.
- **studio:** very small terminals no longer overflow the layout (inverted
  clamp bounds are tolerated), and a diff launched from the list view is anchored
  to its stash (titled panel + `esc` returns to that stash).

### Changed
- Added a `--no-color` flag (also honors `NO_COLOR`).
- Documentation brought back in sync with the code across CLI/MCP/Studio/AGENTS,
  the in-app Studio help documents the preview pager keys, and `StashQuery`
  truncates on rune boundaries.

## [0.18.0] - 2026-06-22

### Added
- **MCP resources** — read stash data by URI without spending a tool call:
  - `fcheap://stashes` — JSON index of every stash.
  - `fcheap://stash/{id}` — a single stash's full manifest (resource template).
- **MCP prompts** — one-shot agent workflows:
  - `investigate_stash` — manifest → analyze/search → connect → summarize.
  - `find_across_stashes` — cross-stash search and synthesis.

### Changed
- `fcheap_list` and the `fcheap://stashes` resource now share a single
  `stashSummary` helper (removing a duplicated summary path and the
  `manifest` keep-alive hack in `server.go`).

### Fixed
- Unchecked `gzReader.Close()` on the decompress path (flagged by
  golangci-lint v2's `errcheck`).

### Internal
- Pinned `task lint` to golangci-lint **v2.7.2** so local lint matches CI and
  can't drift green-locally / red-in-CI again.

## [0.17.0] - 2026-06-22

### Added
- **Hybrid & semantic search** (veclite) — opt-in embedder (`ollama`/`openai`,
  CGO-free) adds a vector index alongside BM25. `search --mode keyword|semantic|hybrid`
  (auto-hybrid when an embedder is configured), embedding-profile drift detection,
  and graceful BM25 fallback.
- **`fcheap connect`** — run semantic code search (vecgrep) over a codebase using a
  stashed artifact's text to surface the `file:line` candidates most likely
  responsible for a bug. Exposed as the `fcheap_connect` MCP tool.
- **Secret detection** — a save-time scan (`internal/secrets`) flags likely
  credentials, surfaced in the CLI, MCP, and Studio (a "secrets" chip/warning).
- **`fcheap vacuum`** — remove orphaned metadata/search-index rows and compact the
  database. Exposed as the `fcheap_vacuum` MCP tool.
- **`fcheap version`** and a `doctor` embedder check.
- **SQLite metadata** via sqlc (`internal/db`), wired into stash operations with
  graceful degradation when the DB is unavailable.
- **Auto-compress on save** when a stash exceeds the configured threshold.
- **Studio TUI redesign** (charm.land bubbletea/lipgloss/bubbles v2): list, detail,
  search, diff, timeline, status, and help views; themed colors and chips; spinner
  and progress-bar animations; responsive layout; and a search-mode cycle.
- MCP server expanded to **11 tools**, including `fcheap_docs` for reading the docs.

### Changed
- The logger now writes text to **stderr** (it previously emitted JSON to stdout,
  which could corrupt `--json` output).
- Config `embedder` / `embed_model` / `ollama_url` are now wired end-to-end.

### Removed
- The dead `parallel` / `FCHEAP_JOBS` config knob.

## [0.16.0] - 2026-06-22

### Changed
- **BREAKING:** Rewrote the project as a local-first **stash tool for agent
  workflows** — save, restore, drop, list, info, compress, diff, and analyze files
  and folders, with an MCP server, a Studio TUI, and VitePress docs. The CLI binary
  is `fcheap`.

## Earlier releases

Versions **0.1.0 – 0.15.1** (January–February 2026) predate the stash rewrite.
See the [GitHub releases page](https://github.com/abdul-hamid-achik/file.cheap/releases)
for their notes and binaries.

[Unreleased]: https://github.com/abdul-hamid-achik/file.cheap/compare/v0.32.1...HEAD
[0.32.1]: https://github.com/abdul-hamid-achik/file.cheap/compare/v0.32.0...v0.32.1
[0.32.0]: https://github.com/abdul-hamid-achik/file.cheap/compare/v0.31.1...v0.32.0
[0.31.1]: https://github.com/abdul-hamid-achik/file.cheap/compare/v0.31.0...v0.31.1
[0.31.0]: https://github.com/abdul-hamid-achik/file.cheap/compare/v0.30.2...v0.31.0
[0.30.0]: https://github.com/abdul-hamid-achik/file.cheap/compare/v0.29.0...v0.30.0
[0.29.0]: https://github.com/abdul-hamid-achik/file.cheap/compare/v0.28.0...v0.29.0
[0.28.0]: https://github.com/abdul-hamid-achik/file.cheap/compare/v0.27.0...v0.28.0
[0.27.0]: https://github.com/abdul-hamid-achik/file.cheap/compare/v0.26.2...v0.27.0
[0.26.2]: https://github.com/abdul-hamid-achik/file.cheap/compare/v0.26.1...v0.26.2
[0.26.1]: https://github.com/abdul-hamid-achik/file.cheap/compare/v0.26.0...v0.26.1
[0.26.0]: https://github.com/abdul-hamid-achik/file.cheap/compare/v0.25.0...v0.26.0
[0.25.0]: https://github.com/abdul-hamid-achik/file.cheap/compare/v0.24.1...v0.25.0
[0.24.1]: https://github.com/abdul-hamid-achik/file.cheap/compare/v0.24.0...v0.24.1
[0.24.0]: https://github.com/abdul-hamid-achik/file.cheap/compare/v0.23.0...v0.24.0
[0.23.0]: https://github.com/abdul-hamid-achik/file.cheap/compare/v0.22.0...v0.23.0
[0.22.0]: https://github.com/abdul-hamid-achik/file.cheap/compare/v0.21.0...v0.22.0
[0.21.0]: https://github.com/abdul-hamid-achik/file.cheap/compare/v0.20.0...v0.21.0
[0.20.0]: https://github.com/abdul-hamid-achik/file.cheap/compare/v0.19.2...v0.20.0
[0.19.2]: https://github.com/abdul-hamid-achik/file.cheap/compare/v0.19.1...v0.19.2
[0.19.1]: https://github.com/abdul-hamid-achik/file.cheap/compare/v0.19.0...v0.19.1
[0.19.0]: https://github.com/abdul-hamid-achik/file.cheap/compare/v0.18.0...v0.19.0
[0.18.0]: https://github.com/abdul-hamid-achik/file.cheap/compare/v0.17.0...v0.18.0
[0.17.0]: https://github.com/abdul-hamid-achik/file.cheap/compare/v0.16.0...v0.17.0
[0.16.0]: https://github.com/abdul-hamid-achik/file.cheap/compare/v0.15.1...v0.16.0
