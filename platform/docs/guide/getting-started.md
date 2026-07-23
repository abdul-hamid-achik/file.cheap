# Getting started

Install file.cheap, create a disposable snapshot, search it, and restore verified
files. The workflow takes only local filesystem access and does not require an
account.

## Install

### macOS with Homebrew

```bash
brew install --cask --no-quarantine abdul-hamid-achik/tap/fcheap
```

The release binary is not signed with an Apple Developer certificate.
`--no-quarantine` avoids the common first-run Gatekeeper block.

### Linux with a Debian package

The package name contains the release version. This example resolves the latest
tag and downloads the amd64 package:

```bash
tag="$(curl -fsSLI -o /dev/null -w '%{url_effective}' https://github.com/abdul-hamid-achik/file.cheap/releases/latest)"
tag="${tag##*/}"
version="${tag#v}"
curl -fLO "https://github.com/abdul-hamid-achik/file.cheap/releases/download/${tag}/fcheap_${version}_linux_amd64.deb"
sudo dpkg -i "fcheap_${version}_linux_amd64.deb"
```

Use the matching arm64 release artifact on an arm64 Linux system.

### Build from source

```bash
go install github.com/abdul-hamid-achik/file.cheap/cmd/fcheap@latest
```

Source installation requires Go 1.25 or newer.

## Check the installation

```bash
fcheap version
fcheap doctor
```

`doctor` reports the effective stash directory, metadata and search indexes,
and optional tools such as vecgrep or an embedder. Keyword search does not need
either optional dependency.

## Create a small artifact

Use a disposable directory so the first workflow does not depend on another
tool:

```bash
mkdir -p /tmp/fcheap-getting-started
printf 'checkout stopped after refresh\nerror code: CART-42\n' \
  > /tmp/fcheap-getting-started/incident.txt
```

## Save and index it

```bash
fcheap save /tmp/fcheap-getting-started \
  --name "Getting started incident" \
  --tag getting-started \
  --tool manual \
  --index
```

`--index` matters: a normal save creates the snapshot, while indexing makes its
readable files searchable. The result reports a generated stash ID, file count,
size, and indexing status. Copy the ID for the next commands.

The save also scans likely secrets by default. A warning identifies the file,
rule, and line without printing the suspected secret value.

## Find the stash again

List the tag you assigned:

```bash
fcheap list --tag getting-started
```

Then search the indexed file:

```bash
fcheap search "CART-42" --mode keyword
```

The result should identify the stash and `incident.txt`, with a snippet around
the matching error code. Keyword mode uses the local BM25 index and does not
contact an embedding service.

Inspect the full manifest:

```bash
fcheap info <stash-id>
```

The manifest shows provenance, tags, saved paths, hashes, compression, expiry,
and secret-scan metadata.

## Restore and verify it

```bash
fcheap restore <stash-id>
```

Without `--to`, restore creates a fresh unique temporary directory and prints
its path. The result should report `verified: true` after hashing the restored
file against `manifest.json`.

To choose a destination instead:

```bash
fcheap restore <stash-id> --to /tmp/fcheap-restored-incident
```

An existing destination is modified in place: same-named files are replaced
and unrelated files remain. Prefer the default fresh directory unless merging
is intentional.

## Clean up the exercise

Removing the temporary source does not remove the stash:

```bash
rm -rf /tmp/fcheap-getting-started /tmp/fcheap-restored-incident
```

Keep the stash for later experiments, or explicitly delete it:

```bash
fcheap drop <stash-id> --force
```

`drop` is permanent. It requires `--force` in non-interactive CLI use.

## Where the data lives

By default:

- configuration: `${XDG_CONFIG_HOME:-$HOME/.config}/fcheap/config.yaml`;
- vault: `${XDG_DATA_HOME:-$HOME/.local/share}/fcheap/`.

Show the effective values with:

```bash
fcheap config path
fcheap config show
```

Use `--stash-dir` or `FCHEAP_STASH_DIR` for an invocation-specific vault
override. Relative configured paths resolve from the configuration directory,
not the current working directory.

## Next steps

- Read [Core concepts](/guide/core-concepts) for the manifest and index model.
- Follow [Workflow examples](/guide/workflows) for evidence, diff, connect, and
  retention recipes.
- Run `fcheap agent` or read the [Agent operating guide](/guide/agent-guide)
  before delegating operations to an assistant.
- Connect an MCP client with the [MCP setup guide](/integrations/mcp-clients).
- Use [Troubleshooting](/guide/troubleshooting) if an expected result differs.
