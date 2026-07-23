# artifact-ref

Emit a stable, credential-free reference to an existing stash. The command is
read-only: it validates the stash and prints metadata without uploading,
restoring, or changing it.

## Usage

```bash
fcheap artifact-ref <stash-id> [flags]
```

## Flags

| Flag | Type | Default | Description |
|---|---|---|---|
| `--kind` | string | derived from bundle | Lowercase artifact kind override |
| `--producer-tool` | string | omitted | Tool that produced the native artifact |
| `--producer-version` | string | omitted | Version of the producer tool |
| `--native-schema` | string | omitted | Absolute schema URI for the native artifact |
| `--native-id` | string | omitted | Producer-native artifact ID |
| `--entrypoint` | string | omitted | Safe relative path to the native descriptor inside the stash |
| `--json` | bool | `false` | Emit the complete JSON envelope |

`--json` is a [global flag](/cli/#global-flags). It can appear before or after
the command.

If you supply any producer field, you must also supply `--producer-tool`.
Producer metadata is omitted completely when all producer flags are empty.

## Basic reference

```bash
fcheap artifact-ref <stash-id> --json
```

For a generic stash, the result is:

```json
{
  "$schema": "urn:filecheap.dev:artifact-ref:v1",
  "version": 1,
  "provider": "fcheap-local",
  "uri": "fcheap://stash/<stash-id>",
  "artifact_id": "<stash-id>",
  "kind": "filecheap.stash"
}
```

When `--kind` is omitted, file.cheap derives a conservative kind from the
manifest bundle type. A generic or empty bundle becomes `filecheap.stash`; a
recognized bundle such as `vidtrace` becomes `vidtrace.bundle`.

## Add native producer metadata

Use producer fields to preserve the identity of the artifact inside the stash:

```bash
fcheap artifact-ref <stash-id> \
  --kind cairntrace.run \
  --producer-tool cairntrace \
  --producer-version 1.8.0 \
  --native-schema urn:cairntrace.dev:run:v1 \
  --native-id run_01 \
  --entrypoint run.json \
  --json
```

```json
{
  "$schema": "urn:filecheap.dev:artifact-ref:v1",
  "version": 1,
  "provider": "fcheap-local",
  "uri": "fcheap://stash/<stash-id>",
  "artifact_id": "<stash-id>",
  "kind": "cairntrace.run",
  "producer": {
    "tool": "cairntrace",
    "version": "1.8.0",
    "native_schema": "urn:cairntrace.dev:run:v1",
    "native_id": "run_01",
    "entrypoint": "run.json"
  }
}
```

`entrypoint` is relative to the stash root. It cannot be absolute, contain
backslashes, or traverse with `.` or `..`. The command validates its syntax,
not whether that path exists in the saved tree. `native_schema` must be a
`urn:` or `https://` URI without embedded credentials or a query string.

## Contract rules

ArtifactRefV1 is intentionally strict:

- `$schema` is always `urn:filecheap.dev:artifact-ref:v1`;
- `version` is the number `1`;
- this command emits only `provider: "fcheap-local"`;
- `uri` must exactly equal `fcheap://stash/<artifact_id>`;
- `artifact_id` is the real opaque stash ID, not a new integration ID;
- `kind` is a bounded lowercase token such as `cairntrace.run`;
- unknown JSON fields are not part of the contract.

ArtifactRefV1 has no `integrity` field because the existing manifest content
hash is not a portable archive or tree digest. The local variant has no
`web_url` because a local stash has no stable web location. Never add a signed
URL, token, or recovery key to the envelope.

The interchange schema also reserves strict `fcheap-cloud` and `link` variants
for other adapters. `artifact-ref` and `fcheap_artifact_ref` do not construct
those variants, and the gated Recovery Lab is not a hosted provider.

## Human output

Without `--json`, the command prints a readable summary:

```text
Artifact Reference

  Schema: urn:filecheap.dev:artifact-ref:v1
  Version: 1
  Provider: fcheap-local
  URI: fcheap://stash/<stash-id>
  Artifact ID: <stash-id>
  Kind: cairntrace.run

Producer
  Tool: cairntrace
  Native Schema: urn:cairntrace.dev:run:v1
  Native ID: run_01
  Entrypoint: run.json
```

Use `--json` for Chalupa, scripts, and other machine consumers.

## Local resolution

The reference does not move bytes. `fcheap://stash/<stash-id>` resolves only on
a machine whose configured file.cheap vault contains that stash.

```bash
fcheap info <stash-id>
fcheap restore <stash-id>
```

A server may store and display the reference without being able to resolve it.
See [Local artifact references for agent tools](/integrations/local-artifact-references)
for the Chalupa, Cairntrace, and Glyphrun boundary.

## MCP equivalent

Agents can call `fcheap_artifact_ref` with the same fields:

```json
{
  "stash_id": "<stash-id>",
  "kind": "glyphrun.run",
  "producer_tool": "glyphrun",
  "native_schema": "urn:glyphrun.dev:run:v1",
  "native_id": "run_01",
  "entrypoint": "run.json"
}
```

The MCP tool is local, read-only, and idempotent. See the
[MCP server reference](/mcp/overview#fcheap-artifact-ref) for the complete
schema.
