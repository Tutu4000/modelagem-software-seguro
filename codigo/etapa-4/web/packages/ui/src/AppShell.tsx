import { useState, type ReactNode } from "react";

import styles from "./AppShell.module.css";
import { CloseIcon, MenuIcon } from "./icons";

export interface AppShellProps {
  brand: ReactNode;
  /** The navigation links, shown in the bar on desktops and in the drawer elsewhere. */
  nav: ReactNode;
  actions?: ReactNode;
  footer?: ReactNode;
  children: ReactNode;
}

/** AppShell is the frame of both apps: a navy bar, the page and a footer. */
export function AppShell({ brand, nav, actions, footer, children }: AppShellProps) {
  const [open, setOpen] = useState(false);
  return (
    <div className={styles.shell}>
      <header className={styles.bar}>
        <div className={styles.barInner}>
          <span className={styles.brand}>
            <span className={styles.brandMark} aria-hidden="true" />
            {brand}
          </span>
          <nav className={styles.nav} aria-label="Principal">
            {nav}
          </nav>
          {actions !== undefined && <div className={styles.actions}>{actions}</div>}
          <button
            type="button"
            className={styles.menuButton}
            aria-expanded={open}
            aria-controls="app-drawer"
            aria-label={open ? "Fechar menu" : "Abrir menu"}
            onClick={() => { setOpen((o) => !o); }}
          >
            {open ? <CloseIcon /> : <MenuIcon />}
          </button>
        </div>
        {open && (
          <nav id="app-drawer" className={styles.drawer} aria-label="Menu">
            {nav}
            {actions}
          </nav>
        )}
      </header>
      <main className={styles.main}>{children}</main>
      {footer !== undefined && (
        <footer className={styles.footer}>
          <div className={styles.footerInner}>{footer}</div>
        </footer>
      )}
    </div>
  );
}
