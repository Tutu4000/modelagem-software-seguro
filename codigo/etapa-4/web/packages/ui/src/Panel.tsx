import type { ComponentPropsWithoutRef } from "react";

import { cx } from "./cx";
import styles from "./Panel.module.css";

export interface PanelProps extends ComponentPropsWithoutRef<"section"> {
  /** A signal-colored top edge, for the panel that holds the main action. */
  accent?: boolean;
}

function PanelRoot({ accent = false, className, ...rest }: PanelProps) {
  return <section className={cx(styles.panel, accent && styles.accent, className)} {...rest} />;
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

/** Panel is a bordered surface composed of Header, Body and Footer. */
export const Panel = Object.assign(PanelRoot, { Header, Body, Footer });
