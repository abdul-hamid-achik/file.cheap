# save

Save a file or directory to the stash vault.

## Usage

```bash
fcheap save <path> [flags]
```

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--name` | string | derived from path | Display name for the stash |
| `--tag` | string slice | `[]` | Tags for categorization (repeatable) |
| `--tool` | string | `""` | Tool that produced the content (e.g., vidtrace) |
| `--source` | string | `""` | Original artifact this stash derives from (provenance) |
| `--meta` | key=value | `[]` | Metadata stored in the manifest `custom` fields and returned by `list --json` (repeatable; keys `[a-z0-9_.-]`, values up to 256 bytes, at most 32 entries; file.cheap-owned keys such as `source`, `indexed`, `secrets_found`, `secrets_files_scanned` and `secrets_files_skipped` are refused) |
| `--ttl` | string | `""` | Time-to-live (e.g. `7d`, `24h`, `30d`); empty = never expires |
| `--no-scan` | bool | `false` | Skip the save-time secret scan |
| `--fail-on-secrets` | bool | `false` | Refuse to save when the scan finds likely secrets: nothing is written and the command exits with code `4`. Cannot be combined with `--no-scan` |
| `--scan-budget-mib` | int | `0` (256) | Total MiB of content the secret scan may inspect; files beyond it are counted as skipped (`budget`) |
| `--no-compress` | bool | `false` | Skip auto-compression of large stashes |
| `--index` | bool | `false` | Index the stash for search immediately after saving (no separate `analyze` step) |

## Examples

```bash
# Save a directory
fcheap save /tmp/artifacts --tag bug-123 --tool vidtrace

# Save with multiple tags
fcheap save ./report.pdf --tag evidence --tag pdf --tool manual

# Save with source provenance
fcheap save /tmp/vidtrace-output --tag EXAMPLE-1234 --tool vidtrace --source ~/Downloads/EXAMPLE-1234.mp4

# Mark the stash expired after 7 days; apply sweep separately to delete it
fcheap save /tmp/codemap-snapshot --tag codemap-snapshot --tool codemap --ttl 7d

# Save a single file
fcheap save ./config.yaml --tag config

# Save and index in one step — searchable immediately, no separate `analyze`
fcheap save /tmp/evidence --tag bug-42 --tool cortex --index

# Gate a pipeline: exit 4 and save nothing if the evidence contains likely secrets
fcheap save ./run-artifacts --tool cairntrace --fail-on-secrets --json

# Record a version and checksum you can filter and compare later with list --json
fcheap save ./dataset.archive.gz --tool bench --tag kind=dataset --tag dataset=demo \
  --meta version=v3 --meta sha256=$(shasum -a 256 dataset.archive.gz | cut -d" " -f1) --no-compress
```

## What Happens

1. fcheap resolves and validates the source path
2. Creates a stash directory at `<stash-dir>/<stash-id>/`
3. Copies the file tree into `content/`
4. Generates a `manifest.json` with metadata, provenance, file count, size, and content hashes
5. Auto-detects bundle type (`monitor.incident`, vidtrace, native run bundles, or generic)
6. Scans content for likely secrets unless you pass `--no-scan`; with `--fail-on-secrets` a finding aborts here, before the manifest is written
7. With `--index`, indexes the stash for search and records `custom.indexed`
8. Prints the generated stash ID and summary

## Path and ID safety

The source must not overlap the stash vault. fcheap rejects the vault itself, a
path inside the vault, and any parent directory that contains the vault. It also
rejects a source-root symlink; pass the resolved path instead. These checks use
canonical paths so symlinks cannot bypass them.

Stash IDs are generated, bounded, and collision-resistant. Treat an ID as an
opaque single path element and copy it from `save` or `list`; fcheap rejects path
separators and traversal values in every ID-taking operation.

## Secret scanning

On save, fcheap scans the content for likely credentials. It records the finding
count and rule names in the manifest and reports **file, rule, and line**
without exposing the secret value. Use `--no-scan` to skip, or review with
[`info`](/cli/info). A `⚠ secrets` chip also appears in
[Studio](/studio/overview).

### Rules

| Rule | Matches |
|---|---|
| `aws-access-key` | `AKIA` followed by 16 characters |
| `github-token` | `ghp_`, `gho_`, `ghu_`, `ghs_`, `ghr_` tokens |
| `slack-token` | `xoxb-`, `xoxa-`, `xoxp-`, `xoxr-`, `xoxs-` tokens |
| `google-api-key` | `AIza` keys |
| `private-key` | `-----BEGIN ... PRIVATE KEY-----` headers |
| `jwt` | Three-segment `eyJ...` tokens |
| `generic-secret` | `api_key`, `secret`, `token`, `password`, `passwd`, `access_key` assigned a value of 16 or more characters |
| `bearer-token` | `Authorization: Bearer <token>` (token of at least 20 characters), including the HAR/trace form `"value": "Bearer <token>"` |
| `cookie-session` | A `Cookie:` or `Set-Cookie:` header (or its HAR/trace JSON form) with a session-like cookie (`session`, `sessid`, `sid`, `token`, `auth` in its name) whose value has at least 16 characters |
| `sk-api-key` | `sk-...` keys of at least 20 characters that contain both letters and digits |
| `stripe-secret-key` | `sk_live_...` and `sk_test_...` |
| `digitalocean-token` | `dop_v1_` followed by 64 hex characters |
| `npm-token` | `npm_` followed by 36 alphanumeric characters |

The newer rules are deliberately strict to keep false positives low: a bare
`Bearer` word in prose, a placeholder such as `Bearer <token>`, a short or
non-session cookie such as `theme=dark`, and identifiers such as `tokenizer` do
not match. Each rule is reported at most once per line.

### How files are read

- **Streaming, no size cap.** Files are read in chunks, never loaded whole, so a
  secret past 1 MiB is found. A line that is longer than 1 MiB (minified JSON) is
  scanned in fragments that overlap by 4 KiB, so a match spanning a fragment
  boundary is still found. Line numbers stay exact.
- **Budget.** One save inspects at most 256 MiB of content (change it with
  `--scan-budget-mib`). Smaller files are scanned first, so a large video or
  trace cannot starve `.env` files. When the budget runs out, the remaining
  files are counted as skipped with reason `budget`.
- **ZIP archives.** For `*.zip` files (including Playwright trace zips), fcheap
  scans the members named `trace.network`, `trace.trace`, `*.har`, `*.ndjson`,
  `*.json`, `*.txt` and `*.log` in memory, and never extracts anything to disk.
  Findings name the file as `archive.zip!member`. Each member is capped at
  64 MiB decompressed, and a member whose decompressed:compressed ratio exceeds
  200:1 (beyond 1 MiB) is treated as a zip bomb. Either guard stops reading that
  member and counts the archive as skipped with reason `zip_guard`; findings from
  the other members are kept. Members of other types (screenshots, video) are not
  scanned. A file named `.zip` that is not a ZIP container is scanned as an
  ordinary file.
- Symlinks, FIFOs, sockets, devices, and files that cannot be opened are counted
  as skipped (`not_regular`, `unreadable`) and never followed.

### Failing a save on findings

`--fail-on-secrets` scans before anything is committed. If there is a finding,
fcheap removes the staged stash, writes nothing to the vault or its index, prints
**file, rule, and line** of each finding (never a value), and exits with code
**4**. It cannot be combined with `--no-scan`.

| Exit code | Meaning |
|---|---|
| `0` | Saved |
| `1` | Any other failure, including a save that completed with failed post-save steps |
| `4` | `--fail-on-secrets` refused the save because the scan found likely secrets |

With `--json`, the refusal prints one document on stdout (and the error text on
stderr) before exiting 4:

```json
{
  "status": "blocked_secrets_found",
  "saved": false,
  "error": "save refused: 2 potential secret(s) detected in 1 file(s) (rules: aws-access-key, generic-secret); nothing was saved",
  "exit_code": 4,
  "secrets": {
    "enabled": true,
    "found": 2,
    "rules": ["aws-access-key", "generic-secret"],
    "files_scanned": 3,
    "files_skipped": 0,
    "skipped_reasons": {},
    "findings": [{ "file": ".env", "rule": "aws-access-key", "line": 1 }],
    "findings_truncated": false
  }
}
```

The `fcheap_save` MCP tool takes the same switch as `fail_on_secrets`; a refused
save is returned as a tool error carrying the same `secrets` object.

A clean scan of a partially covered tree still passes: `--fail-on-secrets`
blocks on findings, not on skipped files. Check `secrets.files_skipped` to see
what the scan did not cover.

If you configure OpenAI or a non-loopback Ollama endpoint, flagged stashes are
blocked from remote indexing unless you explicitly set
`allow_remote_secrets: true`. Loopback Ollama remains local and exempt. With
`save --index`, a policy block leaves the stash safely saved, emits
`status: "saved_with_failures"` with an `index` failure, and exits nonzero; you
can review it before running `analyze` again. See
[`config`](/cli/config#remote-embedding-safety).

## Output

```
Saved stash: <generated-stash-id>
  Source: /tmp/artifacts
  Tool: vidtrace
  Bundle: vidtrace
  Files: 805
  Size: 45.2 MB
  Tags: [EXAMPLE-1234]
! 2 potential secret(s) detected in this stash — review before sharing or restoring elsewhere
  └─ .env:1 [aws-access-key]
  └─ config.yaml:7 [generic-secret]
```

With `--json`, manifest fields remain at the root for compatibility and the
result adds `status`, `index_requested`, `indexed`,
`auto_compression_requested`, `auto_compressed`, and a stable `failed` array.
Indexing or automatic-compression failures produce
`status: "saved_with_failures"` and a nonzero exit after the successfully saved
manifest has been printed.

The result also carries a `secrets` object describing the scan, never a value or
line content:

| Field | Meaning |
|---|---|
| `enabled` | `false` when `--no-scan` skipped the scan |
| `found` | Number of findings (the same count as `custom.secrets_found`) |
| `rules` | Distinct rule IDs that matched |
| `files_scanned` | Files the scanner inspected |
| `files_skipped` | Files it did not inspect |
| `skipped_reasons` | Counts per reason: `budget` (the scan's byte budget ran out), `zip_guard` (an archive member exceeded the size or compression-ratio guard), `not_regular` (symlink, FIFO, socket, or device), `unreadable` |
| `findings` | Up to 200 `{file, rule, line}` entries; archive members appear as `archive.zip!member` |
| `findings_truncated` | `true` when there were more findings than `findings` lists |

`found: 0` means clean only for the scanned files. Check `files_skipped` and
`enabled` before reading it as "no secrets"; files beyond the scan budget and
archives that tripped the zip guard are skipped, and the human output warns when
any file was not scanned. The same coverage is persisted in the manifest as the
custom fields `secrets_files_scanned` and `secrets_files_skipped` (both set only
when the scan ran, and refused by `--meta`), so `list --json` shows it
without re-running the scan. `custom.secrets_found` is still present only when
there were findings.
