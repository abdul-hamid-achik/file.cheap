# fcheap ⇄ codemap integration

> **Status:** design / proposed (2026-06-24). Authored from a codemap-side ecosystem survey.
> **One line:** structure-aware bug localization, durable impact stashes, and one shared veclite —
> `fcheap connect` is the weak link codemap upgrades.

## Boundary
fcheap holds *what the evidence says* (ephemeral artifacts, vidtrace bundles, persisted/searchable stashes;
store at `~/.local/share/fcheap`, `fcheap.veclite` with `files`/`files_vec`). codemap holds *what the code is
and what a change touches*. The harness flow is `vidtrace (repro) → vecgrep/codemap_semantic (locate) →
codemap (structure+impact) → fcheap (persist)`, with findings pinned back onto the graph as annotations.

## Integrations

### A — `connect --impact`: structure-aware localization  ·  M · **high**
`fcheap connect` (and `cairn investigate`) today returns flat vecgrep `file:line` guesses (text similarity
only). Feed each `ConnectResult.Match` `file:line` to `codemap impact --at <file:line>` (codemap owns symbol
resolution — *codemap EI.1*), and feed failing-outcome text/URLs to `codemap_semantic`+`codemap_find`, then
re-rank by `codemap_hotspots` centrality + `codemap_callers` depth. Matches gain
`{symbol, blast_radius_size, covering_tests, callers}`. The bug usually lives where evidence-similarity AND
high fan-in coincide — the one signal vecgrep can't produce. *(codemap EI.9.)*

### B — persist codemap impact/context bundles as stashes  ·  S · medium
`codemap impact --save-to-stash` → `fcheap_save --tool codemap --tag <ticket> --source <repo>@<sha>`;
`manifest.custom` carries `{symbol, blast_radius_size, git_sha}`. Durable, content-hashed, searchable
"what we knew about X".

### C — pin stash evidence onto symbols/paths  ·  M · **high**
After A resolves a stash/run to its owning symbol, write `codemap_annotate{source:'fcheap'|'vidtrace',
data:{stash_id, bundle_type, frame:'f12@12s', ocr_snippet, content_hash, runId}}`. Heavy artifacts stay in
fcheap; codemap stores only the **pointer + summary**. Bug evidence now lives ON the graph (visible in
`codemap_context`/`path`/studio) instead of stranded in stash-land. *(codemap EI.8.)*

### D — lift `fcheap diff` from file-level to symbol/impact  ·  M · medium
Feed `DiffResult.Changed[].Path` to `codemap impact --files a,b,c` (*codemap EI.2*) → per-file symbols +
aggregate blast radius + covering tests. "These files differ" becomes "these capabilities are affected."

### E — share one veclite `EmbeddingProfile`  ·  L · medium
With a matching profile (see `veclite/CODEMAP-INTEGRATION.md`), `fcheap connect` targets
codemap's semantic index instead of spawning a vecgrep subprocess that re-indexes the same code.
`EmbeddingProfile.Compatible()` detects drift.

### F — fcheap as codemap's restorable precise-graph cache  ·  L · low
Export codemap's graph DB to a stash keyed by `git_sha`; `fcheap diff` against the live tree is the staleness
check (no changed files in indexed paths ⇒ reuse the cached precise graph instead of re-running `--precise`).

## Build order
A (upgrades the headline `connect` purpose; depends on codemap EI.1) → C (evidence pinning) → D (depends on
EI.2) → B → E (depends on the shared `EmbeddingProfile`) → F.
