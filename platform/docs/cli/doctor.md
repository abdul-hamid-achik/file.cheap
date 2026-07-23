# doctor

Check runtime health and dependency status.

## Usage

```bash
fcheap doctor
```

## What It Checks

- **System dependencies** (looked up on `PATH`, with resolved path and version):
  - `vecgrep` — optional semantic code search
  - `zstd` — standalone zstd compression binary
  - `tar` — archive creation
- **Stash storage**: the stash directory, and the stash count + total size.
- **Indexes**: the SQLite metadata index (`fcheap.db`) and the veclite search
  index (`fcheap.veclite`), each reported once it has been created.
- **Embedder**: when an embedder is configured, pings it and reports whether it
  is reachable and its vector dimension (otherwise notes keyword-only search).

## Output

```
System Dependencies
  vecgrep    /opt/homebrew/bin/vecgrep
    vecgrep 0.x.x
  zstd       /opt/homebrew/bin/zstd
    zstd 1.5.6
  tar        /usr/bin/tar
    bsdtar 3.5.3

Stash Storage
  Stash dir: ~/.local/share/fcheap
    3 stash(es), 4.2 MB total
  metadata index (SQLite): ~/.local/share/fcheap/fcheap.db
  search index (veclite): ~/.local/share/fcheap/fcheap.veclite
  embedder: not configured (keyword search only)

fcheap doctor: ok
```

Optional dependencies that are missing are reported as warnings (e.g. `vecgrep
not found`) but do not fail the command. Use `--json` for a machine-readable
report (`{ dependencies, stash_dir, all_ok }`).