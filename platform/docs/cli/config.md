# config

Manage fcheap configuration.

## Usage

```bash
fcheap config <subcommand> [flags]
```

## Subcommands

| Command | Description |
|---------|-------------|
| `fcheap config show` | Show current configuration |
| `fcheap config init` | Create default config file |
| `fcheap config set <key> <value>` | Set a config value |
| `fcheap config get <key>` | Get a config value |
| `fcheap config path` | Show config file path |

## Config Keys

| Key | Type | Description |
|-----|------|-------------|
| `stash_dir` | string | Stash storage directory |
| `compression` | string | Compression algorithm (`zstd`, `gzip`, or `none`) |
| `compress_threshold` | int | Auto-compress threshold in bytes |
| `log_level` | string | Log level (debug, info, warn, error) |
| `vecgrep_path` | string | Path to vecgrep binary |
| `embedder` | string | Embedding provider (for semantic search) |
| `embed_model` | string | Embedding model name |
| `ollama_url` | string | Ollama base URL |
| `allow_remote_secrets` | bool | Allow remote indexing for stashes flagged by the secret scanner (default `false`) |
| `default_ttl` | string | Default stash TTL; empty or `never` means permanent |
| `ttl_rules` | map | Per-tool TTL rules, evaluated before `default_ttl` |

## File and path resolution

The config file lives at
`${XDG_CONFIG_HOME:-$HOME/.config}/fcheap/config.yaml`. If you set
`XDG_CONFIG_HOME`, use an absolute path.

For `stash_dir` and `vecgrep_path`, fcheap expands `~` and `~/...`. It resolves a
relative value against the directory containing `config.yaml`, not your current
working directory. This keeps CLI, MCP, and Studio behavior consistent.

## Examples

```bash
# Show current config
fcheap config show

# Initialize default config
fcheap config init

# Set stash directory
fcheap config set stash_dir ~/.local/share/fcheap

# Preserve a tilde for fcheap to expand
fcheap config set stash_dir '~/fcheap-stashes'

# Resolve to <config-dir>/bin/vecgrep from every working directory
fcheap config set vecgrep_path bin/vecgrep

# Set compression
fcheap config set compression zstd

# Get a value
fcheap config get stash_dir

# Show config file path
fcheap config path
```

## Remote embedding safety

The default is `allow_remote_secrets: false`. OpenAI and non-loopback Ollama
endpoints receive document text during indexing. If the save-time scanner
flagged a stash, fcheap blocks that remote indexing before content is sent.
Review the findings first, then either:

- use `embedder: ollama` with its default localhost URL; or
- explicitly run `fcheap config set allow_remote_secrets true` to opt in to
  remote indexing of flagged stashes.

Semantic and hybrid search also send query text to the configured embedder. The
secret guard uses the save-time stash flag and does not scan queries. Ollama
defaults to localhost; URLs whose hostname is not loopback are treated as remote
and follow the same indexing guard as OpenAI. Keep scanning enabled and avoid
secrets in remote queries. BM25 keyword indexing and search need no embedder.
