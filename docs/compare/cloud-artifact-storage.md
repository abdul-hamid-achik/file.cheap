# file.cheap vs cloud artifact storage

Object stores, cloud drives, and CI artifact services are good at retaining and
moving bytes. file.cheap is good at giving local agent artifacts identity,
provenance, search, explicit retention, and verified restore. They are different
layers, not interchangeable brands of the same product.

The shipped file.cheap product has no cloud-storage account service. Its public
website includes a gated recovery laboratory, but the hosted remote vault is not
available to users. Everything in the local vault stays on the user's machine
unless an optional embedder is configured to receive text. Use this comparison
to decide whether local lifecycle tooling is enough or whether you also need an
independently managed remote copy.

## Compare the jobs

| Capability | file.cheap | Object storage | Cloud drive | CI artifacts |
|---|---|---|---|---|
| Local offline save/list/restore | Yes | No | Client-dependent | No |
| Arbitrary large byte retention | Local disk capacity | Core strength | Core strength | Usually bounded by plan/retention |
| Agent-native MCP operations | Built in | Requires an integration | Requires an integration | Requires an integration |
| Tool/source/tag provenance | Manifest fields | Custom metadata design | Folder and filename conventions | Workflow metadata |
| Per-file content search | BM25; optional vectors | Separate index required | Provider-dependent | Usually no |
| Verified restore | Manifest and per-file hashes | Client must implement | Provider-dependent | Download integrity is service-specific |
| Multi-device/team access | Manual today | Access policy required | Core strength | Repository/workflow scoped |
| Local disk reclamation | Compress or delete | Possible after verified upload | Sync policy dependent | Possible after upload |
| Egress/request billing | None for local reads | Common | Usually bundled into plan | Plan-dependent |

## When file.cheap is enough

Stay local when artifacts are reproducible, one machine is the working context,
network independence matters, or the evidence is too sensitive to add another
operator to the trust boundary. Compression, TTLs, cleanup planning, and search
can solve a surprising amount of "storage clutter" without sync.

## When remote storage is justified

Add a remote copy when disk failure would be unacceptable, another device or
teammate needs the exact snapshot, or local capacity is the limiting resource.
That remote layer still needs decisions that a bucket alone does not make:

- which manifest is authoritative;
- how payloads are encrypted before upload;
- how an upload is verified before local eviction;
- whether local eviction and global deletion are separate operations;
- how conflicts, tombstones, quotas, and key recovery work;
- who pays for storage and downloads.

## Avoid the dangerous shortcut

Uploading the live vault directory as a generic synchronized folder can copy
partially written state and create conflicts in derived SQLite or search index
files. The portable units are immutable stash payloads and manifests. Local
indexes can be rebuilt.

A safe offload sequence is:

```text
compress -> encrypt -> upload -> verify -> publish catalog record
         -> explicit local eviction -> hydrate and verify on demand
```

Never remove the last local payload merely because an upload request returned
success. Verify the remote object and test recovery first.

## Cost is mostly behavior

Raw storage price is only one variable. Restore/download traffic, request
counts, retention, duplicated snapshots, and support during recovery can cost
more than idle bytes. A local-first cache with selective remote offload can
reduce downloads, while an automatic two-way mirror can amplify them.

For the current product, start with [`compress`](/cli/compress),
[`cleanup`](/cli/cleanup), and [`restore`](/cli/restore). Read
[local-first vs cloud artifacts](/learn/local-first-vs-cloud-artifacts) for a
policy framework rather than a vendor comparison. A
[local artifact reference](/integrations/local-artifact-references) can let a
control plane record the stash identity, but it is metadata—not a remote copy.
