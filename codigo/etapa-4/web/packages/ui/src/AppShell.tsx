import { useEffect, useRef, useState, type ReactNode } from "react";

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

/** AppShell is the frame of both apps: a navy bar, the page and a footer.
 * A skip link takes the keyboard past the bar, and Esc closes the phone menu
 * and gives the focus back to its button. */
export function AppShell({ brand, nav, actions, footer, children }: AppShellProps) {
  const [open, setOpen] = useState(false);
  const menuButton = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    if (!open) {
      return;
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        setOpen(false);
        menuButton.current?.focus();
      }
    };
    document.addEventListener("keydown", onKey);
    return () => { document.removeEventListener("keydown", onKey); };
  }, [open]);

  return (
    <div className={styles.shell}>
      <a className={styles.skip} href="#conteudo">
        Pular para o conteúdo
      </a>
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
            ref={menuButton}
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
      <main id="conteudo" tabIndex={-1} className={styles.main}>
        {children}
      </main>
      {footer !== undefined && (
        <footer className={styles.footer}>
          <div className={styles.footerInner}>{footer}</div>
        </footer>
      )}
    </div>
  );
}
