# Share local artifacts across agent tools

file.cheap artifact references let Chalupa, Cairntrace, Glyphrun, and other
tools point to one saved artifact without copying its bytes into every product.
The reference is portable metadata. The referenced stash remains in the local
file.cheap vault.

Use this integration when one tool produces evidence and another tool needs to
record where that evidence can be restored.

## Reference contract

Version 1 uses this envelope:

```json
{
  "$schema": "urn:filecheap.dev:artifact-ref:v1",
  "version": 1,
  "provider": "fcheap-local",
  "uri": "fcheap://stash/<stash-id>",
  "artifact_id": "<stash-id>",
  "kind": "cairntrace.run"
}
```

| Field | Meaning |
|---|---|
| `$schema` | Exact contract identifier: `urn:filecheap.dev:artifact-ref:v1` |
| `version` | Numeric contract version, currently `1` |
| `provider` | Storage boundary; this workflow requires `fcheap-local` |
| `uri` | Stable local resource identity in the form `fcheap://stash/<stash-id>` |
| `artifact_id` | The real, opaque file.cheap stash ID |
| `kind` | Producer-defined artifact category used by the receiving interface |
| `producer` | Optional producer metadata emitted from explicit CLI or MCP input |

ArtifactRefV1 intentionally has no `integrity` field. Restore verifies the
saved bytes against the stash manifest; the reference itself does not duplicate
those hashes. The local variant also has no `web_url`, signed URL, bearer token,
recovery key, or other credential.

Generate the envelope from an existing stash instead of constructing it by
hand:

```bash
fcheap artifact-ref <stash-id> --kind cairntrace.run --json
```

See the [`artifact-ref` command reference](/cli/artifact-ref) for producer
fields and the exact output.

### Reserved provider variants

The interchange schema also defines strict `fcheap-cloud` and `link` variants
so a consumer can validate one versioned envelope type. The current CLI and MCP
constructors emit only `fcheap-local`.

- `fcheap-cloud` is reserved for a future hosted vault. It uses a canonical
  `fcheap://cloud/vaults/<vault-id>/artifacts/<artifact-id>` identity and may
  include one stable HTTPS `web_url` without credentials, query string, or
  fragment.
- `link` uses a stable HTTP(S) `uri` and omits `artifact_id` and `web_url`.

No variant permits a signed URL, secret, or `integrity` copied from the legacy
manifest content hash. The Recovery Lab does not implement the
`fcheap-cloud` provider.

## Local resolution boundary

`fcheap://stash/<stash-id>` resolves only where `fcheap` can open a vault that
contains that stash. Copying the JSON to Chalupa does not upload the artifact or
make the operator's laptop reachable from a server.

On a machine with the matching vault:

```bash
fcheap info <stash-id>
fcheap restore <stash-id>
```

On another machine, the reference can still be displayed and indexed as
metadata, but resolution is unavailable until that vault has been copied or
restored there through a separately designed recovery process. Consumers should
show that state as unavailable, not as a missing Chalupa run or a corrupt
artifact.

## End-to-end handoff

The safe sequence is:

```text
Cairntrace or Glyphrun writes a complete artifact pack
  -> fcheap save snapshots the pack
  -> fcheap artifact-ref emits metadata for that stash
  -> task report attaches the metadata on first Chalupa ingestion
  -> Chalupa stores and displays the reference
  -> an operator resolves it from the matching local vault
```

The producer should wait until its artifact pack is complete before saving it.
The run result and the artifact reference describe different facts:

- Cairntrace or Glyphrun owns the native run result and artifact schema.
- Chalupa owns the environment, suite status, duration, and run association.
- file.cheap owns the saved bytes, manifest hashes, retention, and restore.

No product needs a foreign key into another product's database.

## Cairntrace

Save the completed Cairntrace artifact directory, then emit a reference with
producer metadata that identifies the native run:

```bash
fcheap save /absolute/path/to/.cairntrace/runs/<run-id> \
  --tool cairntrace \
  --tag checkout-e2e

fcheap artifact-ref <stash-id> \
  --kind cairntrace.run \
  --producer-tool cairntrace \
  --native-schema urn:cairntrace.dev:run:v1 \
  --native-id <run-id> \
  --entrypoint run.json \
  --json
```

Keep Cairntrace's native result unchanged. The reference is an additional
handoff record; it is not a replacement for Cairntrace's stats or context
schema. Native schema identifiers must use `urn:` or `https://`; executable,
credentialed, and query-bearing URIs are rejected.

## Glyphrun

Glyphrun writes self-contained terminal run packs. Save the completed run
directory and preserve its native run ID in producer metadata:

```bash
fcheap save /absolute/path/to/.glyphrun/runs/<run-id> \
  --tool glyphrun \
  --tag terminal-e2e

fcheap artifact-ref <stash-id> \
  --kind glyphrun.run \
  --producer-tool glyphrun \
  --native-schema urn:glyphrun.dev:run:v1 \
  --native-id <run-id> \
  --entrypoint run.json \
  --json
```

The same pattern works for a rendered report or screenshot; choose a `kind`
that the receiving interface can present clearly.

## Chalupa

Chalupa should store the artifact reference as metadata on the corresponding
suite run. It must not store the artifact bytes, attempt to crawl the local URI,
or treat an unresolved local reference as a failed suite.

`task report` must attach the reference during the run's **first ingestion**.
The control plane cannot discover a local stash later. If a retry uses the same
source run ID, send the same reference again so the ingest remains idempotent.

Until the Chalupa report adapter accepts this versioned envelope, treat the JSON
as the integration contract and adapt it explicitly at the report boundary. Do
not silently drop `$schema`, `version`, `uri`, or producer metadata.

## Recovery Lab is not this boundary

The gated file.cheap Recovery Lab is a single-workspace protocol experiment. It
is not a public vault and is not the integration endpoint for Chalupa,
Cairntrace, or Glyphrun.

These integrations depend only on the shipped local CLI or MCP server and the
versioned reference contract. They continue to work as metadata handoffs when
the public website is offline and when the Recovery Lab is disabled.

## Consumer checklist

When accepting an artifact reference:

1. Require the exact `$schema` and `version`, then allowlist the provider for
   the integration; this local workflow accepts only `fcheap-local`.
2. Treat `artifact_id` as opaque and require `uri` to identify the same stash.
3. Store the complete envelope with the run metadata.
4. Never add credentials, signed URLs, or mutable run state to the reference.
5. Resolve only through a local file.cheap installation and an in-scope vault.
6. Show the restore command and an honest unavailable state when the vault is
   absent.

## See also

- [`artifact-ref` CLI reference](/cli/artifact-ref)
- [MCP server reference](/mcp/overview)
- [Workflow examples](/guide/workflows)
- [Core concepts](/guide/core-concepts)
