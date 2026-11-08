import type { ComponentPropsWithoutRef } from "react";

import { cx } from "./cx";
import styles from "./Grid.module.css";

export interface GridProps extends ComponentPropsWithoutRef<"div"> {
  /** Columns on desktop. Phones always get one, tablets two when desktop has more. */
  columns?: 2 | 3 | 4 | "sidebar";
}

/** Grid is mobile first: one column, then two on tablets, then the desktop layout. */
export function Grid({ columns = 2, className, ...rest }: GridProps) {
  const desktop = columns === "sidebar" ? styles.sidebar : styles[`desktop${String(columns)}`];
  return <div className={cx(styles.grid, columns !== "sidebar" && styles.tablet2, desktop, className)} {...rest} />;
}
