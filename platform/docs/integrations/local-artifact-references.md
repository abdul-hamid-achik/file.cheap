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
those hashes. The local variant defines no `web_url` or dedicated credential or
recovery-key field, and its `fcheap://` URI is not signed.

Credential-free is a transport property, not a DLP claim. ArtifactRefV1 rejects
URL userinfo, queries, and fragments in transport `uri` and `web_url` fields
and has no dedicated credential field. `producer.native_schema` rejects
userinfo and queries but may use a fragment. Validation does not secret-scan or
redact permitted identifiers, paths, caller-supplied `kind`, or producer
metadata. The envelope omits stash names and tags, while the artifact ID,
native ID, schema, tool, and entrypoint can still reveal operational
information. Use non-sensitive metadata and apply the receiving system's
disclosure policy before persisting or displaying a reference. Other
file.cheap CLI and MCP surfaces may return user-provided stash names and tags.

Generate the envelope from an existing stash instead of constructing it by
hand:

```bash
fcheap artifact-ref <stash-id> --kind cairntrace.run --json
```

See the [`artifact-ref` command reference](/cli/artifact-ref) for producer
fields and the exact output.

### Other provider variants

The interchange schema also defines strict `fcheap-cloud` and `link` variants
so a consumer can validate one versioned envelope type. The current CLI and MCP
constructors for an existing local stash emit only `fcheap-local`.

- `fcheap-cloud` is emitted by the private, single-owner artifact service and
  by [`fcheap publish`](/cli/publish). It uses a canonical
  `fcheap://cloud/vaults/<vault-id>/artifacts/<artifact-id>` identity and may
  include one stable HTTPS `web_url` without credentials, query string, or
  fragment. It is not a public hosted vault or multi-user account service.
- `link` uses a stable HTTP(S) `uri` and omits `artifact_id` and `web_url`.

No variant defines a dedicated secret field or permits query-bearing signed
URLs or an `integrity` value copied from the legacy manifest content hash.
Permitted identifiers and paths are not DLP-scanned.

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

## Compatibility status

| Component | Current handoff |
|---|---|
| file.cheap CLI | `fcheap artifact-ref` emits a validated local reference for an existing stash |
| file.cheap MCP | `fcheap_artifact_ref` returns the same reference as structured tool content |
| Cairntrace | Save the completed run directory with `fcheap save`; Cairntrace wrapper ID forwarding is pending |
| Glyphrun | Save the completed `.glyphrun/runs/<run-id>` pack with `fcheap save` |
| Monitor | Save a completed `monitor.incident` bundle; detection and indexing use its bounded incident projection |
| Chalupa | `task report REPORT=... ARTIFACTS=...` accepts an ArtifactRefV1 sidecar; its Production deployment is still pending |
| Private artifact service | `fcheap publish` and approved service integrations emit verified `fcheap-cloud` references; `fcheap pull` recovers their verified bytes for the paired owner; this is single-owner infrastructure, not a public vault |

The `artifact-ref` CLI and MCP constructors emit `fcheap-local`. Chalupa can
preserve and display that metadata without being able to restore the bytes from
its server. Private service responses use `fcheap-cloud` and keep all transfer
credentials outside the reference.

Artifact references are available in file.cheap `v0.30.0` and later. Confirm the
installed release with `fcheap version` and inspect the copyable command
examples with `fcheap artifact-ref --help`.

## Cairntrace

Cairntrace writes a self-contained run directory. After the pack is complete,
save the absolute directory with file.cheap and copy `id` from the JSON result:

```bash
fcheap save /absolute/path/to/<completed-cairn-run-dir> \
  --tool cairntrace \
  --tag checkout-e2e \
  --json
```

Then ask file.cheap to construct the reference:

```bash
fcheap artifact-ref <stash-id> \
  --kind cairntrace.run \
  --producer-tool cairntrace \
  --native-schema urn:cairntrace.dev:run:v1 \
  --native-id <raw-cairn-run-id> \
  --entrypoint run.json \
  --json
```

`latest` and `previous` are valid Cairntrace run selectors, but the
`producer.native_id` and the Chalupa sidecar key must use the resolved raw run
ID. They must not use Chalupa's environment-namespaced `sourceRunId`.

The current Cairntrace `cairn stash save --format json` wrapper does not
reliably forward the top-level `id` returned by file.cheap. Until its parser
accepts `data.id`, use the direct `fcheap save` command above.

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

## Monitor

Monitor writes a completed incident bundle whose `manifest.json` declares
`kind: "monitor.incident"` and `schema_version: "1"`. Save and index the bundle
after all of its files have been finalized:

```bash
fcheap save /absolute/path/to/<completed-monitor-incident> \
  --tool monitor \
  --tag incident \
  --index

fcheap artifact-ref <stash-id> \
  --kind monitor.incident \
  --producer-tool monitor \
  --native-schema urn:monitor.dev:incident:v1 \
  --native-id <incident-id> \
  --entrypoint manifest.json \
  --json
```

The local search projection includes diagnosis/context fields, code
correlations, semantic hits, and process identity. It does not synthesize text
from `snapshot.json` or `profile.json`. Cloud publication uses a Monitor-bound
publisher credential with the exact same kind/schema pair; it does not attach
a `RunIndexV1`.

## Chalupa

The Chalupa repository implements a strict artifact sidecar keyed by raw Cairn
run ID. The sidecar is a separate document so the Cairn stats schema stays
unchanged:

```json
{
  "$schema": "urn:chalupa.run:artifact-sidecar:v1",
  "version": 1,
  "runs": {
    "run_demo_20260723_001": [
      {
        "$schema": "urn:filecheap.dev:artifact-ref:v1",
        "version": 1,
        "provider": "fcheap-local",
        "uri": "fcheap://stash/<stash-id>",
        "artifact_id": "<stash-id>",
        "kind": "cairntrace.run",
        "producer": {
          "tool": "cairntrace",
          "native_schema": "urn:cairntrace.dev:run:v1",
          "native_id": "run_demo_20260723_001",
          "entrypoint": "run.json"
        }
      }
    ]
  }
}
```

Place the complete object printed by `fcheap artifact-ref` under its matching
raw Cairn run ID. Do not type a second, reduced `{kind, stashId}` shape and do
not add Chalupa fields inside the ArtifactRefV1 envelope. The file.cheap command
prints one reference; the reporting integration is responsible for assembling
the sidecar around one or more of those objects.

Before generating the report, ensure every included Cairn run was recorded with
the target suite and a grouping label, for example
`--label suite=checkout-e2e --label path=<variant>`. Then filter that suite,
group by the cohort label, and include the raw run rows:

```bash
cairn stats \
  --group-by path \
  --label suite=checkout-e2e \
  --include-runs \
  --format json > ./cairn-stats.json

jq -e --slurpfile sidecar ./artifact-sidecar.json '
  [.runs[].runId] as $run_ids
  | ($run_ids | length > 0)
    and (
      $sidecar[0].runs
      | keys
      | all(. as $id | $run_ids | index($id) != null)
    )
' ./cairn-stats.json

task report \
  CONFIG=/absolute/path/to/chalupa.yml \
  REPORT=./cairn-stats.json \
  ARTIFACTS=./artifact-sidecar.json \
  SUITE=checkout-e2e
```

The `jq` preflight requires at least one report run and verifies that every
sidecar key matches a raw run ID in that exact filtered document. Chalupa also
rejects orphan keys, unknown fields, and duplicate `provider` + `uri`
identities within one run. Its current bounds are 250 run keys, 20 references
per run, 4 MiB of raw Cairn JSON, 1 MiB of sidecar JSON, and 256 KiB for the
normalized request. Omitting `ARTIFACTS` preserves the artifact-free reporting
flow.

Chalupa stores the complete artifact reference as metadata on the corresponding
suite run. It does not store the artifact bytes, crawl a local URI, or treat an
unresolved local reference as a failed suite.

`task report` must attach the reference during the run's **first ingestion**.
The control plane cannot discover a local stash later. If a retry uses the same
source run ID, resend the exact same normalized run and sidecar. `task report`
uses a fresh HMAC nonce for the retry; identical content deduplicates.

Changing the artifacts or other run facts for the same source run returns
`409 run_ingest_conflict` and does not overwrite the stored run. The adapter is
implemented in Chalupa but not yet deployed to Chalupa Production, so complete
its database migration and release gate before relying on the hosted readback.

## The private service is an optional, separate boundary

The private file.cheap artifact service is a single-owner integration boundary,
not a public vault. An approved producer can publish one bounded immutable
artifact through its authenticated plan, direct upload, and verified commit
protocol. The resulting `fcheap-cloud` reference remains credential-free; the
signed transfer URL and receipt never belong in the reference.

The local workflow documented on this page still depends only on the shipped
CLI or MCP server and the versioned reference contract. It continues to work as
a metadata handoff when the website is offline or private-service credentials
are unavailable. Use [`fcheap publish`](/cli/publish) only when the approved
single-owner remote-retention boundary is intentionally required.

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
