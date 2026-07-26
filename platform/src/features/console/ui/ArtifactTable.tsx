import type { ConsoleArtifact } from "./artifact-types";
import {
  artifactAvailabilityLabel,
  artifactStateLabel,
  formatArtifactBytes,
  formatArtifactDate,
} from "./artifact-types";
import styles from "./console.module.css";

interface ArtifactTableProps {
  artifacts: readonly ConsoleArtifact[];
  labelledBy?: string;
  onSelect: (artifact: ConsoleArtifact, trigger: HTMLButtonElement) => void;
  selectedId?: string;
}

export function ArtifactTable({
  artifacts,
  labelledBy,
  onSelect,
  selectedId,
}: ArtifactTableProps) {
  return (
    <div className={styles.tableWrap}>
      <table aria-labelledby={labelledBy} className={styles.table}>
        <thead>
          <tr>
            <th scope="col">Artifact</th>
            <th scope="col">Producer</th>
            <th scope="col">Evidence</th>
            <th scope="col">Retention</th>
            <th scope="col"><span className={styles.srOnly}>Open details</span></th>
          </tr>
        </thead>
        <tbody>
          {artifacts.map((artifact) => (
            <tr className={selectedId === artifact.id ? styles.selectedRow : undefined} key={artifact.id}>
              <td>
                <span className={styles.kindMark} aria-hidden="true">◫</span>
                <div className={styles.artifactIdentity}>
                  <strong>{artifact.label}</strong>
                  <span>{artifact.kind} · {formatArtifactBytes(artifact.sizeBytes)}</span>
                </div>
              </td>
              <td>
                <span className={styles.mono}>{artifact.producer.tool}</span>
                {artifact.producer.version ? <span className={styles.subtle}> {artifact.producer.version}</span> : null}
              </td>
              <td>
                <span className={styles.evidence}>{artifactStateLabel(artifact.state)}</span>
                <span className={styles.subtle}>{artifactAvailabilityLabel(artifact.availability)}</span>
              </td>
              <td className={styles.mono}>{formatArtifactDate(artifact.expiresAt)}</td>
              <td className={styles.actionCell}>
                <button
                  aria-controls="artifact-detail"
                  aria-pressed={selectedId === artifact.id}
                  className={styles.rowButton}
                  onClick={(event) => onSelect(artifact, event.currentTarget)}
                  type="button"
                >
                  Details<span className={styles.srOnly}> for {artifact.label}</span>
                </button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
