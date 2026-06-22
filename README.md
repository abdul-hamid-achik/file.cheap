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

# Drop a stash when done (requires --force)
fcheap drop <stash-id> --force

# Open the Studio TUI for browsing stashes
fcheap studio

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

This exposes tools: `fcheap_save`, `fcheap_list`, `fcheap_info`, `fcheap_restore`, `fcheap_drop`, `fcheap_search`, `fcheap_analyze`, `fcheap_diff`.

## Configuration

```bash
fcheap config           # print current config
```

Config file (`~/.config/fcheap/config.yaml`):

```yaml
stash_dir: ~/.local/share/fcheap
compression: zstd
compress_threshold: 10485760  # 10MB
parallel: 8
log_level: warn
vecgrep_path: ""              # optional, for semantic search
```

Environment variables: `FCHEAP_STASH_DIR`, `FCHEAP_JOBS`, `FCHEAP_LOG_LEVEL`, `FCHEAP_VECGREP_PATH`.

## Storage Layout

```
~/.local/share/fcheap/
├── <stash-id>/
│   ├── manifest.json       # metadata, provenance, tags
│   ├── content/            # file tree (or archive.tar.zst)
│   └── analysis/           # search index (if analyzed)
└── fcheap.veclite          # veclite database for keyword search
```

## Studio TUI

The Studio is a terminal UI built with Bubbletea v2 for browsing stashes:

```bash
fcheap studio
```

Navigate with `j/k`, view details with `Enter`, quit with `q`.

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
│   ├── mcp/                 # MCP server (8 tools)
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