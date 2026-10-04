/**
 * Stable, credential-free console deep link for one artifact. The path is a
 * real console route (`/console/artifacts/:id`) that redirects into the
 * catalog drawer after authentication; the ArtifactRefV1 contract forbids
 * query strings and fragments in `web_url`, so the link can carry neither.
 *
 * `publicOrigin` must be the configured bare public origin (never a request
 * Host header). Non-https origins (local development) yield `undefined`
 * because the contract only accepts https links.
 */
export function artifactWebUrl(publicOrigin: string | undefined, artifactId: string): string | undefined {
  if (!publicOrigin) return undefined;
  let origin: URL;
  try {
    origin = new URL(publicOrigin);
  } catch {
    return undefined;
  }
  if (origin.protocol !== "https:" || origin.username || origin.password) return undefined;
  return `${origin.origin}/console/artifacts/${encodeURIComponent(artifactId)}`;
}

/** In-console destination for a `web_url` visit: the run drawer for indexed runs, else the artifact drawer. */
export function consoleArtifactRedirect(artifactId: string, isIndexedRun: boolean): string {
  const id = encodeURIComponent(artifactId);
  return isIndexedRun
    ? `/console/runs?q=${id}&run=${id}`
    : `/console?q=${id}&artifact=${id}`;
}
