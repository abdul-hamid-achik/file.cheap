/**
 * Global and per-producer artifact size bounds.
 *
 * `maximumArtifactBytes` is the hard platform ceiling. It bounds the request
 * contract, the Postgres CHECK constraints, and the signed PUT grant. The
 * ceiling is set by the only server-side full-object read in the system: the
 * commit-time SHA-256 verification, which streams the private object through a
 * Vercel Function at O(1) memory. At a deliberately pessimistic 10 MB/s
 * Blob-to-Function read a 64 MiB object digests in roughly seven seconds, well
 * inside a conservative 60-second Node function budget, and the digest itself
 * is never the bottleneck. Raising this further would start trading the
 * `verification: "server-sha256"` guarantee against function duration, so it
 * is deliberately far below the `integer` column range.
 *
 * `defaultProducerMaxSizeBytes` is what a keyring entry gets when it does not
 * declare `maxSizeBytes`. It is intentionally conservative: a new producer must
 * opt in to a larger quota explicitly, it never inherits the global ceiling.
 */
export const maximumArtifactBytes = 64 * 1024 * 1024;
export const defaultProducerMaxSizeBytes = 8 * 1024 * 1024;
