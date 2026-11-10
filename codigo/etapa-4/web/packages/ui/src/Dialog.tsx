import { useEffect, useRef, type ComponentPropsWithoutRef, type ReactNode } from "react";

import { cx } from "./cx";
import styles from "./Dialog.module.css";

export interface DialogProps {
  open: boolean;
  onClose: () => void;
  /** The accessible name, usually the header's title id. */
  labelledBy: string;
  children: ReactNode;
}

/** Dialog is the native modal dialog: the browser keeps the focus inside,
 * Esc closes it, and the focus goes back to what opened it. On phones it is a
 * sheet at the bottom of the screen. */
function DialogRoot({ open, onClose, labelledBy, children }: DialogProps) {
  const ref = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    const d = ref.current;
    if (d === null) {
      return;
    }
    if (open && !d.open) {
      d.showModal();
    } else if (!open && d.open) {
      d.close();
    }
  }, [open]);
  return (
    <dialog ref={ref} className={styles.dialog} aria-labelledby={labelledBy} onClose={onClose}>
      {open && children}
    </dialog>
  );
}

function Header({ className, ...rest }: ComponentPropsWithoutRef<"header">) {
  return <header className={cx(styles.header, className)} {...rest} />;
}

function Body({ className, ...rest }: ComponentPropsWithoutRef<"div">) {
  return <div className={cx(styles.body, className)} {...rest} />;
}

function Footer({ className, ...rest }: ComponentPropsWithoutRef<"footer">) {
  return <footer className={cx(styles.footer, className)} {...rest} />;
}

export const Dialog = Object.assign(DialogRoot, { Header, Body, Footer });
