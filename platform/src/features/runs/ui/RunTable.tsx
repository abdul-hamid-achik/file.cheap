import type { RunSummary } from "@/features/runs/contracts";

import {
  formatRunDate,
  runEvidenceCountLabel,
  runHealthLabel,
  runStatusLabel,
} from "./run-presentation";
import styles from "./runs.module.css";

interface RunTableProps {
  onSelect: (run: RunSummary, trigger: HTMLButtonElement) => void;
  runs: readonly RunSummary[];
  selectedId?: string;
}

export function RunTable({ onSelect, runs, selectedId }: RunTableProps) {
  return (
    <div className={styles.tableWrap}>
      <table aria-label="Recorded runs" className={styles.table}>
        <thead><tr><th scope="col">Run</th><th scope="col">Status</th><th scope="col">Health</th><th scope="col">Indexed evidence</th><th scope="col">Updated</th><th scope="col"><span className={styles.srOnly}>Open details</span></th></tr></thead>
        <tbody>
          {runs.map((run) => (
            <tr className={selectedId === run.artifactId ? styles.selectedRow : undefined} key={run.artifactId}>
              <td><div className={styles.runIdentity}><strong>{run.run.specName ?? run.run.nativeId}</strong><span>{run.producer.tool}{run.run.environment ? ` · ${run.run.environment}` : ""}</span></div></td>
              <td><span className={`${styles.status} ${styles[`status${run.run.status[0].toUpperCase()}${run.run.status.slice(1)}`]}`}>{runStatusLabel(run.run.status)}</span></td>
              <td><span className={styles.health}>{runHealthLabel(run.health.state)}</span></td>
              <td>{runEvidenceCountLabel(run)}</td>
              <td className={styles.mono}>{formatRunDate(run.updatedAt)}</td>
              <td className={styles.actionCell}><button aria-controls="run-detail" aria-pressed={selectedId === run.artifactId} className={styles.rowButton} onClick={(event) => onSelect(run, event.currentTarget)} type="button">Details<span className={styles.srOnly}> for {run.run.specName ?? run.run.nativeId}</span></button></td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
