# Getting Started

## Installation

### macOS (Homebrew)

```bash
# Use --no-quarantine to avoid Gatekeeper warnings (binary is unsigned)
brew install --no-quarantine abdul-hamid-achik/tap/fcheap
```

### Linux (deb)

```bash
curl -LO https://github.com/abdul-hamid-achik/file.cheap/releases/latest/download/fcheap_linux_amd64.deb
sudo dpkg -i fcheap_linux_amd64.deb
```

### From source

```bash
go install github.com/abdul-hamid-achik/file.cheap/cmd/fcheap@latest
```

## Quick Start

Save files to the stash vault:

```bash
fcheap save /tmp/vidtrace-artifacts --tag OPG-15061 --tool vidtrace --source ~/Downloads/OPG-15061.mp4
```

List what you've stashed:

```bash
fcheap list
```

Get details about a specific stash:

```bash
fcheap info <stash-id>
```

Restore a stash to a working directory:

```bash
fcheap restore <stash-id> --to /tmp/working/
```

Search across all indexed stashes:

```bash
fcheap search "columns not showing up"
```

Diff a stash against a live codebase:

```bash
fcheap diff <stash-id> ~/projects/graphite
```

Drop a stash when you're done:

```bash
fcheap drop <stash-id> --force
```

Check runtime health:

```bash
fcheap doctor
```

## Configuration

fcheap uses XDG directories by default:

- Config: `~/.config/fcheap/config.yaml`
- Data: `~/.local/share/fcheap/`

You can override the stash directory with the `--stash-dir` flag or `FCHEAP_STASH_DIR` env var.

### Config file

```yaml
stash_dir: ~/.local/share/fcheap
compression: zstd
compress_threshold: 10485760  # 10MB
log_level: warn
vecgrep_path: ""              # optional, for semantic search
```

### Environment variables

| Variable | Description |
|----------|-------------|
| `FCHEAP_STASH_DIR` | Override stash storage directory |
| `FCHEAP_LOG_LEVEL` | Override log level (debug, info, warn, error) |
| `FCHEAP_VECGREP_PATH` | Path to vecgrep binary for semantic search |