export type ArtifactAvailability =
  | "cloud-ready"
  | "external-reference"
  | "local-vault-required"
  | "unavailable";

export type ArtifactIntegrity =
  | "server-sha256"
  | "declared"
  | "not-applicable";

export type ArtifactState =
  | "planned"
  | "committed"
  | "deleting"
  | "deleted";

export interface ConsoleArtifact {
  availability: ArtifactAvailability;
  contentType?: string;
  createdAt?: string | null;
  description?: string;
  expiresAt?: string | null;
  id: string;
  integrity: ArtifactIntegrity;
  kind: string;
  label: string;
  producer: {
    nativeId?: string;
    tool: string;
    version?: string;
  };
  sha256?: string;
  sizeBytes?: number;
  state: ArtifactState;
}

export function artifactStateLabel(state: ArtifactState): string {
  const labels: Record<ArtifactState, string> = {
    planned: "Pending upload",
    committed: "Committed",
    deleting: "Retention in progress",
    deleted: "Unavailable",
  };
  return labels[state];
}

export function artifactAvailabilityLabel(
  availability: ArtifactAvailability,
): string {
  const labels: Record<ArtifactAvailability, string> = {
    "cloud-ready": "Cloud transfer ready",
    "external-reference": "External reference",
    "local-vault-required": "Local vault required",
    unavailable: "Unavailable",
  };
  return labels[availability];
}

export function artifactIntegrityLabel(integrity: ArtifactIntegrity): string {
  const labels: Record<ArtifactIntegrity, string> = {
    "server-sha256": "Server SHA-256 verified",
    declared: "Checksum declared by producer",
    "not-applicable": "No byte verification claim",
  };
  return labels[integrity];
}

export function formatArtifactBytes(sizeBytes?: number): string {
  if (sizeBytes === undefined || !Number.isFinite(sizeBytes) || sizeBytes < 0) {
    return "Size unavailable";
  }

  const units = ["B", "KiB", "MiB", "GiB", "TiB"] as const;
  let value = sizeBytes;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit += 1;
  }
  const digits = value >= 100 || unit === 0 ? 0 : value >= 10 ? 1 : 2;
  return `${value.toFixed(digits)} ${units[unit]}`;
}

export function formatArtifactDate(value?: string | null): string {
  if (!value) return "No retention limit";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "Date unavailable";
  return new Intl.DateTimeFormat("en-US", {
    dateStyle: "medium",
    timeStyle: "short",
    timeZone: "UTC",
  }).format(date);
}

export type ArtifactGroupBy = "producer" | "kind";

export interface ConsoleArtifactGroup {
  artifacts: readonly ConsoleArtifact[];
  id: string;
  label: string;
}

export function groupConsoleArtifacts(
  artifacts: readonly ConsoleArtifact[],
  by: ArtifactGroupBy,
): readonly ConsoleArtifactGroup[] {
  const groups = new Map<string, ConsoleArtifact[]>();
  for (const artifact of artifacts) {
    const label = by === "producer" ? artifact.producer.tool : artifact.kind;
    const existing = groups.get(label);
    if (existing) {
      existing.push(artifact);
    } else {
      groups.set(label, [artifact]);
    }
  }

  return [...groups]
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([label, grouped]) => ({
      artifacts: grouped,
      id: `${by}-${label}`.replaceAll(/[^a-zA-Z0-9_-]/g, "-"),
      label,
    }));
}
