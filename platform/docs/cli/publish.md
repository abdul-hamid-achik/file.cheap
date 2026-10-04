# `fcheap publish`

`fcheap publish` is an opt-in bridge from one bounded local file to the private
file.cheap artifact service. It does not save a stash, delete the input, or
accept Vercel, Blob, Neon, or administrator credentials.

The command streams one regular file of at most 64 MiB to hash it, plans an
upload, streams the same bytes directly to the signed storage URL, and commits
only after the service reports `server-sha256` verification. 64 MiB is the
global platform ceiling; each producer also has its own smaller server-side
quota, and a file above it is rejected with `413` naming that quota. Its JSON result is
a `filecheap-publish/1` receipt containing a credential-free
`fcheap-cloud` `ArtifactRefV1`. New receipt fields are added as optional keys
(see [JSON receipt](#json-receipt)); consumers should tolerate optional keys they do not know.

## Usage

```sh
FILECHEAP_ARTIFACT_SERVICE_URL=https://file.cheap \
FILECHEAP_INGEST_TOKEN='producer-bound base64url credential' \
fcheap publish ./run.tar.gz \
  --content-type application/gzip \
  --kind cairntrace.run \
  --producer-tool cairntrace \
  --native-schema urn:cairntrace.dev:run:v1 \
  --native-id run-123 \
  --run-index ./run-index.json \
  --json
```

## Flags

| Flag | Default | Description |
|---|---|---|
| `--service-url` | `FILECHEAP_ARTIFACT_SERVICE_URL` | Bare artifact service origin: HTTPS, or HTTP on loopback for local tests. The flag takes precedence over the environment variable |
| `--content-type` | `application/octet-stream` | Content type of the file, 1 to 255 characters |
| `--expires-in` | retain | Delete the remote artifact after this duration, from `1m` to `744h` |
| `--kind` | `filecheap.artifact` | Artifact kind; must match the producer's server-side policy |
| `--producer-tool` | `fcheap` | Tool that produced the artifact; must match the producer's policy |
| `--producer-version` | none | Version of the producer tool: a portable token of at most 64 characters (letters, digits, `.`, `_`, `+`, `-`, starting with a letter or digit) |
| `--native-schema` | none | Absolute URN or HTTPS URI of the native artifact schema, without credentials or a query string, at most 256 characters |
| `--native-id` | none | Producer-native artifact ID: a portable token of at most 160 characters |
| `--entrypoint` | none | Safe, slash-separated relative path of a descriptor inside the artifact, at most 512 characters, with no `.` or `..` segments, backslashes, or leading slash. It is recorded as provenance only; the CLI never opens the archive |
| `--run-index` | none | Metadata-only `RunIndexV1` JSON sidecar for Cairntrace or Glyphrun, at most 12 KiB |

`FILECHEAP_INGEST_TOKEN` is the only accepted secret. It must be a 43-128
character base64url credential assigned to exactly one producer. The
`--producer-tool`, `--kind`, and `--native-schema` values must match that
producer's server-side policy; a token for Cairntrace cannot publish a Glyphrun
or Chalupa artifact. The command rejects a Vercel runtime or OIDC credential
environment. Vercel-to-Vercel producers use OIDC directly with the service
rather than invoking this local command.

For a Cairntrace or Glyphrun archive, `--run-index` can attach an explicit
metadata-only `RunIndexV1` sidecar. The CLI accepts only a small regular JSON
file with the exact top-level contract and verifies that its detector and
native run ID match the producer flags. It never opens or extracts the archive
to infer metadata. The platform performs the complete strict validation and
binds the sidecar digest to the immutable upload plan. The sidecar must be a
regular file of at most 12 KiB (12,288 bytes); a larger file is refused before
any request is sent. See
[Run indexes](/integrations/run-index) for the safe field boundary.

A Monitor incident is published as an ordinary immutable artifact, not as a
run index:

```sh
FILECHEAP_ARTIFACT_SERVICE_URL=https://file.cheap \
FILECHEAP_INGEST_TOKEN='monitor-bound base64url credential' \
fcheap publish ./incident.tar.zst \
  --content-type application/zstd \
  --kind monitor.incident \
  --producer-tool monitor \
  --native-schema urn:monitor.dev:incident:v1 \
  --native-id incident-123
```

The server-side Monitor policy must bind exactly that producer, kind, and
native schema. Do not pass `--run-index`: Monitor incidents remain available
under Artifacts and are searchable after local save/analyze, but they are not
misrepresented as Cairntrace or Glyphrun runs.

A Chalupa turn receipt from an operator's laptop is published the same way,
under the `chalupa-cli` producer:

```sh
FILECHEAP_ARTIFACT_SERVICE_URL=https://file.cheap \
FILECHEAP_INGEST_TOKEN='chalupa-cli-bound base64url credential' \
fcheap publish ./inference-receipt.json \
  --content-type application/json \
  --kind chalupa.inference-receipt \
  --producer-tool chalupa-cli \
  --native-schema urn:chalupa:inference-receipt:v1 \
  --native-id run-2026-08-05-17-42-01 \
  --expires-in 720h
```

That producer binds each kind to one exact native schema and its own byte
quota. See [Publish Chalupa agent
artifacts](/integrations/chalupa-agent-artifacts) for the kinds, size limits,
and the raw HTTP contract a Chalupa CLI can implement without `fcheap`.

Load the token from TinyVault into only the publisher process. Do not put it in
the command line, logs, artifact metadata, a receipt, or a child process. Token
rotation does not change the CLI: an operator activates the next credential for
that producer, updates its TinyVault value, verifies publication, and retires
the previous credential.

## Failures

When the service rejects a request, the error names the HTTP status and, when
the response is an RFC 9457 `application/problem+json` document, its `code`,
`title`, and `detail`, for example:

```text
plan artifact publication: artifact service returned unexpected status 413 (producer_quota_exceeded): Artifact exceeds the producer quota: producer 'cairntrace' allows up to 33554432 bytes; this artifact declares 41943040 bytes.
```

The detail is length-bounded and stripped of control characters, and a body that
is not a problem document is never echoed. `5xx` responses and transport errors
are treated as transient and retried once; other statuses are final.

The CLI ignores response fields it does not know, so the service can add fields
without breaking installed CLIs. It still validates every field it relies on,
including the `ArtifactRefV1`.

## Retry behavior

The plan request may be retried once with one client-generated idempotency key;
the final commit may be retried with the same opaque service receipt only until
that plan's `plan_expires_at`. Direct PUT is never retried automatically because
its result can be ambiguous and a signed URL must never appear in output. If the immutable object already exists, a
non-overwrite conflict is safe to advance to commit because the service reads
the bounded private bytes and verifies their SHA-256 before committing. The
local source remains unchanged in every failure case.

Protocol integrations that persist their own deterministic idempotency key may
repeat the exact plan after its up-to-15-minute transfer grant expires. A plan
and its upload grant are always capped at the artifact retention timestamp.
The service renews the grant for the same artifact and rejects any metadata or
content change. Once retention begins, it rejects a new grant or commit,
including a commit whose object verification crossed that boundary. An expired
plan that is currently being reconciled returns a retryable `409`; the caller
must wait and retry the same plan. The `fcheap publish` command keeps its
generated key only for the lifetime of one invocation: every plan attempt inside
that invocation reuses it, and a separate invocation publishes a separate
artifact. If the service answers a plan with `200` and an already committed
summary, the CLI treats that replay as a finished publication after the same
checks a commit receives, without uploading again.

Persist the normalized plan facts and reuse the original `expiresAt`; do not
recompute a rolling retention date on retry. Persist the opaque receipt only in
the producer's protected delivery state, never in `ArtifactRefV1`, logs, or
user-facing output. If the exact artifact already committed, a repeated plan
returns `200` with its committed summary and no upload grant. Repeating the
original receipt also returns the verified committed summary only before
`plan_expires_at`; afterward the idempotency-keyed plan request remains the
recovery mechanism for the retained artifact.

## JSON receipt

```json
{
  "version": "filecheap-publish/1",
  "artifact_ref": {
    "$schema": "urn:filecheap.dev:artifact-ref:v1",
    "version": 1,
    "provider": "fcheap-cloud",
    "uri": "fcheap://cloud/vaults/private/artifacts/art_example",
    "artifact_id": "art_example",
    "kind": "chalupa.log-chunk"
  },
  "sha256": "...",
  "size_bytes": 1234,
  "verification": "server-sha256",
  "published_at": "2026-07-24T00:00:00Z",
  "committed_at": "2026-07-24T00:00:01.482Z",
  "expires_at": "2026-08-23T00:00:01.482Z"
}
```

The receipt never contains the plan receipt, a signed URL, or credentials.

Timestamp fields:

| Field | Source | Present |
|---|---|---|
| `published_at` | The CLI's clock when the receipt was produced (RFC 3339, UTC, second precision) | Always |
| `committed_at` | The artifact service's own commit time, passed through unchanged (RFC 3339) | Only when the service reports it |
| `expires_at` | The artifact service's own retention deadline, passed through unchanged (RFC 3339) | Only when the artifact has a retention (`--expires-in`) |

`committed_at` and `expires_at` are additive and optional: they are omitted
entirely, never `null`, when the service did not report a valid RFC 3339 value,
so a consumer must treat both as optional. `version` stays `filecheap-publish/1`
and `published_at` keeps its client-clock meaning. Prefer `expires_at` over
computing a deadline from `published_at` and `--expires-in`: it is the date the
service will actually enforce. A human run (without `--json`) also prints an
`Expires` line when `expires_at` is present.
