# ArtifactRefV1 conformance

`urn:filecheap.dev:artifact-ref:v1` is an immutable, credential-free
interchange contract. Backward-incompatible changes require a new schema URI
and a new versioned directory.

The contract has three complementary sources:

1. `schema.json` describes the strict JSON shape and provider variants.
2. `internal/artifactref` enforces semantic invariants that JSON Schema cannot
   express clearly, including exact URI-to-artifact identity and valid network
   port ranges.
3. Every document under `valid/` and `invalid/` is part of the conformance
   corpus. Consumers should run the complete corpus through their own
   validators.

Important semantic invariants include:

- local `uri` must equal `fcheap://stash/<artifact_id>`;
- cloud `uri` must identify the same `artifact_id`;
- HTTP(S) references cannot contain credentials, query strings, fragments, or
  ports outside `0..65535`;
- `native_schema` is either a non-empty opaque `urn:` identifier or an HTTPS
  URI without credentials or a query string;
- optional fields must be omitted instead of encoded as empty strings or
  `null`;
- local references never contain `web_url`, and link references contain
  neither `artifact_id` nor `web_url`.

Downstream repositories should pin a reviewed file.cheap commit and record the
SHA-256 checksum of `schema.json` plus the fixture corpus in their own contract
parity test. A passing structural JSON Schema check alone is not conformance.
