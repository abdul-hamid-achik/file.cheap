# Getting Started

## Installation

### macOS (Homebrew)

```bash
# Use --no-quarantine to avoid Gatekeeper warnings (binary is unsigned)
brew install --no-quarantine abdul-hamid-achik/tap/fcheap
```

### Linux (deb)

```bash
tag="$(curl -fsSLI -o /dev/null -w '%{url_effective}' https://github.com/abdul-hamid-achik/file.cheap/releases/latest)"
tag="${tag##*/}"
version="${tag#v}"
curl -fLO "https://github.com/abdul-hamid-achik/file.cheap/releases/download/${tag}/fcheap_${version}_linux_amd64.deb"
sudo dpkg -i "fcheap_${version}_linux_amd64.deb"
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

- Config: `${XDG_CONFIG_HOME:-$HOME/.config}/fcheap/config.yaml`
- Data: `${XDG_DATA_HOME:-$HOME/.local/share}/fcheap/`

You can override the stash directory with the `--stash-dir` flag or `FCHEAP_STASH_DIR` env var.
`XDG_CONFIG_HOME` must be absolute when set. In `stash_dir` and `vecgrep_path`,
fcheap expands `~` and anchors relative values to the config directory rather
than the current working directory.

### Config file

```yaml
stash_dir: ~/.local/share/fcheap
compression: zstd
compress_threshold: 10485760  # 10MB
log_level: warn
vecgrep_path: ""              # optional, for semantic search
embedder: ""                  # optional: ollama (localhost default) or openai (remote)
embed_model: ""
ollama_url: ""                # default http://localhost:11434
allow_remote_secrets: false  # block remote indexing of scanner-flagged stashes
default_ttl: ""
ttl_rules: {}
```

OpenAI and non-loopback Ollama endpoints receive document text during indexing
and query text during semantic/hybrid search. Scanner-flagged stashes are
blocked from remote indexing unless you explicitly set
`allow_remote_secrets: true`; queries are not scanned by that guard. Ollama
defaults to localhost, and loopback endpoints remain local without the opt-in.

### Environment variables

| Variable | Description |
|----------|-------------|
| `FCHEAP_STASH_DIR` | Override stash storage directory |
| `FCHEAP_LOG_LEVEL` | Override log level (debug, info, warn, error) |
| `FCHEAP_VECGREP_PATH` | Path to vecgrep binary for semantic search |
| `XDG_CONFIG_HOME` | Absolute parent directory for `fcheap/config.yaml` |
| `XDG_DATA_HOME` | Parent directory for the default stash vault |
