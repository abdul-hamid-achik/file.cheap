import type { RunSummary } from "@/features/runs/contracts";

import { runEvidenceCountLabel, runHealthLabel, runStatusLabel } from "./run-presentation";
import styles from "./runs.module.css";

interface RunCardsProps {
  onSelect: (run: RunSummary, trigger: HTMLButtonElement) => void;
  runs: readonly RunSummary[];
  selectedId?: string;
}

export function RunCards({ onSelect, runs, selectedId }: RunCardsProps) {
  return (
    <ul className={styles.cards}>
      {runs.map((run) => (
        <li className={selectedId === run.artifactId ? styles.selectedCard : undefined} key={run.artifactId}>
          <button aria-controls="run-detail" aria-pressed={selectedId === run.artifactId} className={styles.cardButton} onClick={(event) => onSelect(run, event.currentTarget)} type="button">
            <span className={styles.cardHeading}><span aria-hidden="true" className={styles.runMark}>◇</span><span><strong>{run.run.specName ?? run.run.nativeId}</strong><small>{run.producer.tool}{run.run.environment ? ` · ${run.run.environment}` : ""}</small></span></span>
            <span className={styles.cardMeta}><span>{runStatusLabel(run.run.status)}</span><span>{runHealthLabel(run.health.state)}</span><span>{runEvidenceCountLabel(run)}</span></span>
          </button>
        </li>
      ))}
    </ul>
  );
}
