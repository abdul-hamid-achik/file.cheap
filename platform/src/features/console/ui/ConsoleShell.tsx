import type { ReactNode } from "react";

import { LogoutButton } from "@/features/console/auth/LogoutButton";

import styles from "./console.module.css";

export interface ConsoleNavigationItem {
  current?: boolean;
  href?: string;
  label: string;
}

interface ConsoleShellProps {
  children: ReactNode;
  navigation: readonly ConsoleNavigationItem[];
  sessionLabel?: string;
}

/**
 * Presentation-only shell for a future authenticated console. Its parent owns
 * all links, authentication, and data fetching so this layer has no route or
 * credential assumptions.
 */
export function ConsoleShell({
  children,
  navigation,
  sessionLabel = "Private session",
}: ConsoleShellProps) {
  return (
    <div className={styles.shell}>
      <aside className={styles.sidebar} aria-label="Console navigation">
        <div className={styles.brand}>
          <span className={styles.brandMark} aria-hidden="true">f·</span>
          <span>file.cheap</span>
        </div>
        <nav className={styles.navigation} aria-label="Primary">
          {navigation.map((item) =>
            item.href ? (
              <a
                aria-current={item.current ? "page" : undefined}
                className={item.current ? styles.navigationCurrent : styles.navigationLink}
                href={item.href}
                key={item.label}
              >
                {item.label}
              </a>
            ) : (
              <span
                aria-current={item.current ? "page" : undefined}
                className={item.current ? styles.navigationCurrent : styles.navigationLink}
                key={item.label}
              >
                {item.label}
              </span>
            ),
          )}
        </nav>
        <p className={styles.sessionLabel}>{sessionLabel}</p>
        <LogoutButton />
      </aside>
      <main className={styles.main} id="main-content" tabIndex={-1}>
        {children}
      </main>
    </div>
  );
}
