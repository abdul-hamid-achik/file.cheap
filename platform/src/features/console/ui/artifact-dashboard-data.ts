import type { ArtifactSummary } from "@/features/artifacts/contracts";

import type { ConsoleArtifact } from "./artifact-types";

export interface ArtifactDashboardMetrics {
  cloudReady: number;
  expiringSoon: number;
  totalBytes: number;
  totalCount: number;
  verifiedCount: number;
}

const expiringSoonMilliseconds = 24 * 60 * 60 * 1_000;

/** Maps only fields guaranteed by ArtifactSummary; it never fabricates a URL. */
export function toConsoleArtifact(summary: ArtifactSummary): ConsoleArtifact {
  const artifact = summary.artifact;
  return {
    availability: artifact.state === "committed" ? "cloud-ready" : "unavailable",
    contentType: artifact.contentType,
    createdAt: artifact.committedAt,
    expiresAt: artifact.expiresAt,
    id: artifact.artifactId,
    integrity: artifact.verification,
    kind: artifact.kind,
    label: artifact.producer.native_id ?? artifact.kind,
    producer: {
      nativeId: artifact.producer.native_id,
      tool: artifact.producer.tool,
      version: artifact.producer.version,
    },
    sha256: artifact.sha256,
    sizeBytes: artifact.sizeBytes,
    state: artifact.state,
  };
}

export function deriveArtifactDashboardMetrics(
  artifacts: readonly ConsoleArtifact[],
  now: Date,
): ArtifactDashboardMetrics {
  const horizon = now.getTime() + expiringSoonMilliseconds;
  let cloudReady = 0;
  let expiringSoon = 0;
  let totalBytes = 0;
  let verifiedCount = 0;

  for (const artifact of artifacts) {
    if (artifact.availability === "cloud-ready") cloudReady += 1;
    if (artifact.integrity === "server-sha256") verifiedCount += 1;
    if (artifact.sizeBytes !== undefined) totalBytes += artifact.sizeBytes;
    if (!artifact.expiresAt) continue;
    const expiration = new Date(artifact.expiresAt).getTime();
    if (expiration > now.getTime() && expiration <= horizon) expiringSoon += 1;
  }

  return {
    cloudReady,
    expiringSoon,
    totalBytes,
    totalCount: artifacts.length,
    verifiedCount,
  };
}
