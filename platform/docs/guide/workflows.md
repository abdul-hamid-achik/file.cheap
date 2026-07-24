# Workflow examples

These workflows use the current local product. They keep the snapshot lifecycle
explicit: save coherent data, index it before search, restore when complete
bytes matter, and separate cleanup previews from deletion.

## Save, search, and restore a general artifact

Use this for logs, reports, generated output, screenshots, notes, or any other
file tree that should survive beyond one agent run.

### 1. Save coherent output

Wait for the producing tool to finish, then save the completed directory:

```bash
fcheap save /tmp/checkout-investigation \
  --name "Checkout investigation" \
  --tag checkout \
  --tag bug-123 \
  --tool manual \
  --index
```

The result reports a stash ID. `--index` adds readable files to the local search
index after the snapshot succeeds.

### 2. Search for a decisive detail

```bash
fcheap search "CART-42" --mode keyword
fcheap search "checkout stopped after refresh" --mode keyword
```

Each result identifies the stash and matching relative file. Begin with exact
identifiers; add an embedder only when paraphrase search improves a real
workflow.

### 3. Inspect provenance

```bash
fcheap info <stash-id>
```

Confirm the source, producing tool, tags, file list, secret findings, and any
expiry before using the snapshot as evidence.

### 4. Restore verified bytes

```bash
fcheap restore <stash-id>
```

The default destination is a fresh temporary directory. The command prints that
path and reports whether the restored files match their manifest hashes.

## Hand an artifact to Chalupa

For a Cairntrace run, save its completed run directory directly:

```bash
fcheap save /absolute/path/to/<completed-cairn-run-dir> \
  --tool cairntrace \
  --tag checkout-e2e \
  --json

fcheap artifact-ref <stash-id> \
  --kind cairntrace.run \
  --producer-tool cairntrace \
  --native-schema urn:cairntrace.dev:run:v1 \
  --native-id <raw-cairn-run-id> \
  --entrypoint run.json \
  --json
```

Copy `id` from the first command into `<stash-id>` in the second. Put the
complete ArtifactRefV1 result in a Chalupa artifact sidecar under that same raw
Cairn run ID, then submit both files:

```bash
task report \
  CONFIG=/absolute/path/to/chalupa.yml \
  REPORT=./cairn-stats.json \
  ARTIFACTS=./artifact-sidecar.json \
  SUITE=checkout-e2e
```

Attach the sidecar on the source run's first ingest. Retries must resend the
same normalized run and references; Chalupa rejects later changes instead of
overwriting evidence. Chalupa stores run metadata and the reference while
file.cheap keeps the bytes, manifest hashes, retention, and restore behavior.

Use the direct save until Cairntrace's `cairn stash save --format json` wrapper
forwards file.cheap's top-level `id`.

The reference does not upload anything. `fcheap://stash/<stash-id>` resolves
only on a machine whose configured vault contains that stash. See the complete
[Chalupa, Cairntrace, and Glyphrun integration guide](/integrations/local-artifact-references).

## Compare a generated tree over time

`diff` is useful when the stash and target represent the same directory shape.
For example, save a generated site, run a new generator version, and compare the
new output with the baseline:

```bash
fcheap save ./generated-site --name "Generator baseline"
# Run the generator again, changing ./generated-site.
fcheap diff <stash-id> ./generated-site
```

The result separates relative paths found only in the stash, only in the target,
and in both with changed content hashes.

Do not use `diff` to compare a video evidence bundle with an application source
repository. Use `connect` when the two trees have different structures but the
evidence text may point to related code.

## Investigate a vidtrace bundle

This workflow preserves video-derived frames, OCR, transcript text, and source
provenance, then uses them to rank related code candidates.

### 1. Extract the completed evidence

```bash
vidtrace extract ~/Downloads/checkout-bug.mp4 \
  --output /tmp/checkout-bug
```

A recognized vidtrace bundle contains `metadata.json` and `timeline.json`.

### 2. Save and index the bundle

```bash
fcheap save /tmp/checkout-bug \
  --name "Checkout refresh failure" \
  --tag checkout \
  --tag repro \
  --tool vidtrace \
  --source ~/Downloads/checkout-bug.mp4 \
  --index
```

Save only after extraction completes so paths, hashes, and timeline metadata
describe one coherent snapshot.

### 3. Search OCR and transcript text

```bash
fcheap search "payment columns disappeared" --mode keyword
```

The matching saved file, frame, or timestamp narrows the evidence before you
open the complete bundle.

### 4. Connect evidence to source code

`connect` requires the separately installed vecgrep binary:

```bash
fcheap doctor
fcheap connect <stash-id> ~/projects/storefront --index
```

file.cheap derives a bounded query from the stash and asks vecgrep to rank
source chunks. The output is a set of investigation leads with path, line,
score, and snippet—not a confirmed root cause.

Use an explicit query when the automatically extracted evidence is too broad:

```bash
fcheap connect <stash-id> ~/projects/storefront \
  --query "table columns reset after token refresh" \
  --limit 5
```

### 5. Restore when full evidence matters

```bash
fcheap restore <stash-id>
```

Inspect the verified restored frame, OCR, transcript, and metadata together
before presenting a conclusion.

## Delegate a workflow through MCP

Register the local stdio server using the [MCP client setup](/integrations/mcp-clients),
then give the agent the version-matched operating contract:

```bash
fcheap agent
```

MCP clients can also read `fcheap://agent-guide`; the server repeats the guide
in its initialization instructions.

A safe investigation sequence is:

1. read the agent guide;
2. call `fcheap_list` or `fcheap_search` narrowly;
3. inspect the selected manifest with `fcheap_info` or
   `fcheap://stash/{id}`;
4. call `fcheap_analyze` only if the relevant stash is not indexed;
5. use search snippets as leads;
6. call `fcheap_restore` to a fresh directory if complete file bodies are
   required;
7. use `fcheap_connect` only when a repository is in scope;
8. stop before `force: true` or any applied cleanup unless deletion was
   explicitly requested.

The stash resource contains metadata and a file list, not arbitrary file
bodies. An agent must not claim it read surrounding content from the manifest.

## Retain caches without silent growth

Assign a TTL when a producing tool creates regenerable snapshots:

```bash
fcheap save /tmp/codemap-snapshot \
  --tool codemap \
  --tag codemap-snapshot \
  --ttl 7d
```

Preview storage and cleanup state:

```bash
fcheap ecosystem-status
fcheap sweep
fcheap cleanup --smart
```

These commands do not delete by default. Apply an expired-stash plan only after
reviewing it:

```bash
fcheap sweep --apply
```

Use a `keep` tag for snapshots that must survive automated retention. Treat
vidtrace and similar evidence as unique unless the user established otherwise.

## Close an investigation deliberately

Choose one outcome:

- keep the stash permanently;
- assign a reviewed TTL with [`ttl`](/cli/ttl);
- compress it with [`compress`](/cli/compress);
- permanently remove it with `fcheap drop <stash-id> --force`.

Record the stash ID in the issue or investigation summary before closing the
workflow so another agent can reproduce the evidence lookup.
