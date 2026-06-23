# Workflow Examples

## Vidtrace Bug Investigation

This is the primary workflow fcheap was designed for: extracting video artifacts, analyzing them, and connecting them to the codebase where a bug lives.

### 1. Extract artifacts with vidtrace

```bash
# vidtrace produces a bundle in /tmp with frames, OCR, and transcripts
vidtrace extract ~/Downloads/OPG-15061.mp4 --output /tmp/vidtrace-opg-v090
```

### 2. Save artifacts to the stash vault

```bash
fcheap save /tmp/vidtrace-opg-v090/OPG-15061_artifacts_20260622_115254 \
  --tag OPG-15061 \
  --tool vidtrace \
  --source ~/Downloads/OPG-15061.mp4
```

fcheap automatically detects the vidtrace bundle and extracts searchable text (OCR + transcripts).

### 3. Analyze the stash

```bash
# Index the stash for keyword search (built-in BM25 via veclite)
fcheap analyze <stash-id>

# Search for specific content
fcheap search "Internal Migrant"
fcheap search "columns not showing"
```

### 4. Connect to the codebase

This is the connective tissue — point the stashed bug report at the live repo and
let vecgrep surface the code most likely responsible:

```bash
# Auto-extracts the bundle's OCR + transcript as the query, runs vecgrep
fcheap connect <stash-id> ~/projects/graphite --index
```

The output ranks `file:line` candidates by relevance. See [connect](/cli/connect).

### 5. Diff against the codebase

```bash
# Compare artifacts against the live codebase (which files changed, by hash)
fcheap diff <stash-id> ~/projects/graphite
```

### 6. Restore for deeper investigation

```bash
# Extract the stash to a working directory
fcheap restore <stash-id> --to /tmp/working-opg-15061/
```

### 7. Clean up when done

```bash
fcheap drop <stash-id> --force
```

## Agent Workflow with MCP

When using fcheap as an MCP tool server, an AI agent like Claude can perform the entire workflow:

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

The agent can then:
1. Call `fcheap_save` to stash artifacts (secrets are scanned automatically)
2. Call `fcheap_analyze` to index and search content (per-file BM25)
3. Call `fcheap_connect` to map the bug report to the responsible code (vecgrep)
4. Call `fcheap_diff` to compare against a codebase
5. Call `fcheap_restore` to extract files for inspection
6. Call `fcheap_drop` to clean up

## General File Stashing

fcheap works with any files, not just vidtrace bundles:

```bash
# Save a folder of log files
fcheap save /var/log/myapp --tag logs --tool manual

# Save a single file
fcheap save ./config.yaml --tag config-snapshot

# Compress a large stash to save space
fcheap compress <stash-id>

# List stashes by tag
fcheap list --tag logs
```