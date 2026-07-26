"use client";

import styles from "./console.module.css";

export function ArtifactDashboardError({ retry }: { retry: () => void }) {
  return (
    <section aria-labelledby="artifact-dashboard-error" className={styles.errorState}>
      <p className={styles.eyebrow}>Artifact registry</p>
      <h1 id="artifact-dashboard-error">The registry could not be loaded</h1>
      <p>Nothing was changed. Check the connection and try loading the owner-scoped catalog again.</p>
      <button onClick={retry} type="button">Try again</button>
    </section>
  );
}
