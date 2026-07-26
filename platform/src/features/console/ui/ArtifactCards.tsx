import type { ConsoleArtifact } from "./artifact-types";
import {
  artifactAvailabilityLabel,
  artifactStateLabel,
  formatArtifactBytes,
  formatArtifactDate,
} from "./artifact-types";
import styles from "./console.module.css";

interface ArtifactCardsProps {
  artifacts: readonly ConsoleArtifact[];
  onSelect: (artifact: ConsoleArtifact, trigger: HTMLButtonElement) => void;
  selectedId?: string;
}

export function ArtifactCards({
  artifacts,
  onSelect,
  selectedId,
}: ArtifactCardsProps) {
  return (
    <ul className={styles.cards}>
      {artifacts.map((artifact) => (
        <li className={selectedId === artifact.id ? styles.selectedCard : undefined} key={artifact.id}>
          <button
            aria-controls="artifact-detail"
            aria-pressed={selectedId === artifact.id}
            className={styles.cardButton}
            onClick={(event) => onSelect(artifact, event.currentTarget)}
            type="button"
          >
            <span className={styles.cardHeading}>
              <span className={styles.kindMark} aria-hidden="true">◫</span>
              <span>
                <strong>{artifact.label}</strong>
                <small>{artifact.kind} · {formatArtifactBytes(artifact.sizeBytes)}</small>
              </span>
            </span>
            <span className={styles.cardMeta}>
              <span>{artifact.producer.tool}</span>
              <span>{artifactStateLabel(artifact.state)}</span>
              <span>{artifactAvailabilityLabel(artifact.availability)}</span>
              <span>Expires: {formatArtifactDate(artifact.expiresAt)}</span>
            </span>
          </button>
        </li>
      ))}
    </ul>
  );
}
