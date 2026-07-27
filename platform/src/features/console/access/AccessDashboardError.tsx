"use client";

import styles from "./access.module.css";

export function AccessDashboardError({ retry }: { retry: () => void }) {
  return (
    <section aria-labelledby="access-dashboard-error" className={styles.errorState}>
      <p className={styles.eyebrow}>Owner access control</p>
      <h1 id="access-dashboard-error">Access controls could not be loaded</h1>
      <p>No device was changed. Check the private service connection and try loading the owner-scoped device list again.</p>
      <button onClick={retry} type="button">Try again</button>
    </section>
  );
}
