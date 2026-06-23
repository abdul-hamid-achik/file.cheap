# file.cheap

Local-first stash tool for saving, restoring, compressing, and analyzing files and folders for agent workflows. Everything runs on your machine -- no cloud, no accounts, no uploads.

## Install

```bash
# macOS (Homebrew) — use --no-quarantine to avoid Gatekeeper warnings
brew install --no-quarantine abdul-hamid-achik/tap/fcheap

# Linux (deb)
curl -LO https://github.com/abdul-hamid-achik/file.cheap/releases/latest/download/fcheap_linux_amd64.deb
sudo dpkg -i fcheap_linux_amd64.deb

# From source
go install github.com/abdul-hamid-achik/file.cheap/cmd/fcheap@latest
```

## Usage

```bash
# Save files or folders to the stash vault
# (content is scanned for likely secrets on save; pass --no-scan to skip)
fcheap save /tmp/vidtrace-artifacts --tag OPG-15061 --tool vidtrace --source ~/Downloads/OPG-15061.mp4

# List saved stashes, optionally filtered by tag
fcheap list
fcheap list --tag OPG-15061

# Get detailed info about a stash
fcheap info <stash-id>

# Restore a stash to a working directory
fcheap restore <stash-id> --to /tmp/working/

# Compress a stash to save space
fcheap compress <stash-id>

# Analyze (index) a stash for search
fcheap analyze <stash-id>

# Search across all stashes
fcheap search "Internal Migrant"

# Diff a stash against a live codebase
fcheap diff <stash-id> ~/projects/graphite

# Connect a stash to a codebase — find the code that likely owns the bug (via vecgrep)
fcheap connect <stash-id> ~/projects/graphite --index

# Drop a stash when done (requires --force)
fcheap drop <stash-id> --force

# Open the Studio TUI for browsing stashes
fcheap studio

# Reclaim space — remove orphaned index entries and compact the database
fcheap vacuum

# Check runtime health
fcheap doctor
```

## MCP Server

Use `fcheap` as an MCP tool server for AI assistants like Claude:

```json
{
  "mcpServers": {
    "file-cheap": {
      "command": "fcheap",
      "args": ["mcp", "serve"]
    }
  }
}
```

This exposes 11 **tools**: `fcheap_save`, `fcheap_list`, `fcheap_info`, `fcheap_restore`, `fcheap_drop`, `fcheap_search`, `fcheap_analyze`, `fcheap_diff`, `fcheap_connect`, `fcheap_vacuum`, `fcheap_docs` — plus **resources** (`fcheap://stashes`, `fcheap://stash/{id}`) for reading stash data by URI and **prompts** (`investigate_stash`, `find_across_stashes`) for one-shot agent workflows. See [docs/mcp/overview.md](docs/mcp/overview.md).

## Configuration

```bash
fcheap config show              # print current config
fcheap config path              # print the config file path
fcheap config get <key>         # read one key
fcheap config set <key> <value> # write one key
fcheap config init [--force]    # write a fresh default config
```

Config file (`~/.config/fcheap/config.yaml`):

```yaml
stash_dir: ~/.local/share/fcheap
compression: zstd
compress_threshold: 10485760  # 10MB — stashes larger than this auto-compress on save
log_level: warn
vecgrep_path: ""              # optional, for semantic code search via vecgrep
embedder: ""                  # optional: "ollama" or "openai" — enables semantic/hybrid search
embed_model: ""               # e.g. nomic-embed-text (ollama)
ollama_url: ""                # default http://localhost:11434
```

With an `embedder` configured, `analyze` indexes a vector per document and
`search --mode semantic|hybrid` finds related meaning even with no shared
keywords (default `hybrid`). Embedders are HTTP-based, so the binary stays
CGO-free. See [search](https://file.cheap/cli/search).

Stashes larger than `compress_threshold` are compressed automatically on `save`
(opt out with `fcheap save --no-compress`).

Pass `--log-level debug` (or set `log_level`) to print operation traces to stderr
for troubleshooting — stdout and `--json` output stay clean.

Environment variables: `FCHEAP_STASH_DIR`, `FCHEAP_LOG_LEVEL`, `FCHEAP_VECGREP_PATH`.

## Storage Layout

```
~/.local/share/fcheap/
├── <stash-id>/
│   ├── manifest.json       # metadata, provenance, tags (source of truth)
│   └── content/            # file tree, OR content.tar.zst when compressed
├── fcheap.db               # SQLite metadata index (sqlc, CGO-free)
└── fcheap.veclite          # veclite per-file BM25 search index
```

The `manifest.json` in each stash directory is the portable source of truth;
`fcheap.db` is a write-through index that self-heals from the manifests, and
`fcheap.veclite` holds the per-file keyword search index.

## Studio TUI

The Studio is a terminal UI built with Bubbletea v2 for browsing, searching, and
acting on stashes:

```bash
fcheap studio
```

| Key | Action |
|-----|--------|
| `j` / `k` | Move cursor up/down |
| `enter` / `l` | Open stash detail (provenance, file tree, live preview) |
| `esc` / `h` | Back to list |
| `/` | Search stash content (keyword) |
| `tab` | Cycle pane focus (query ↔ results ↔ preview) |
| `r` | Restore the stash to a temp dir (with hash verification) |
| `c` | Compress the stash (zstd) |
| `a` | Analyze / index the stash for search |
| `x` | Diff the stash against a directory |
| `t` | View the vidtrace evidence timeline (frame → OCR → transcript) |
| `d` | Drop the stash (with `y/n` confirm) |
| `s` | Status view · `?` Help · `q` Quit |

## Project Structure

```
file.cheap/
├── cmd/fcheap/              # CLI entry point
├── internal/
│   ├── stash/               # Core domain: Save, Restore, Drop, List, Info
│   ├── manifest/            # Stash metadata and provenance
│   ├── compress/            # tar+zstd archiving
│   ├── detect/              # Bundle type detection (vidtrace, generic)
│   ├── analyze/             # BM25 search + vecgrep subprocess
│   ├── diff/                # Stash-to-directory comparison
│   ├── db/                  # SQLite metadata storage
│   ├── mcp/                 # MCP server (11 tools + resources + prompts)
│   ├── studio/              # Bubbletea v2 TUI
│   ├── fcheap/cli/             # Cobra commands
│   ├── fcheap/config/          # YAML config
│   ├── fcheap/output/           # Printer, progress bars, tables
│   ├── fcheap/version/          # Build-time version
│   ├── apperror/            # Error types
│   └── logger/              # slog wrapper
├── e2e/                     # glyphrun e2e test specs
└── testdata/                # Test fixtures
```

## Tech Stack

- **Go 1.25**, single static binary, `CGO_ENABLED=0`
- **CLI**: `spf13/cobra`, `fatih/color`
- **MCP**: `modelcontextprotocol/go-sdk` (official SDK)
- **TUI**: `charm.land/bubbletea/v2`, `charm.land/lipgloss/v2`
- **Compression**: `klauspost/compress/zstd`
- **Database**: `modernc.org/sqlite` (CGO-free SQLite)
- **E2E**: glyphrun

## License

MIT