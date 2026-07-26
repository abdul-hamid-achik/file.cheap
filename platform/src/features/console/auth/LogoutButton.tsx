"use client";

import { useState } from "react";

import styles from "@/features/console/ui/console.module.css";

export function LogoutButton() {
  const [busy, setBusy] = useState(false);

  async function logout() {
    setBusy(true);
    const response = await fetch("/api/console/auth/logout", { method: "POST" });
    if (response.ok) {
      window.location.assign("/console/login");
      return;
    }
    setBusy(false);
  }

  return <button className={styles.logoutButton} disabled={busy} onClick={logout} type="button">{busy ? "Signing out…" : "Sign out"}</button>;
}
