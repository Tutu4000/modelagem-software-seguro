import type { ComponentPropsWithoutRef } from "react";

import { cx } from "./cx";
import styles from "./Toolbar.module.css";

function ToolbarRoot({ className, ...rest }: ComponentPropsWithoutRef<"div">) {
  return <div role="toolbar" className={cx(styles.toolbar, className)} {...rest} />;
}

/** Spacer pushes what follows to the end of the toolbar. */
function Spacer() {
  return <span className={styles.spacer} aria-hidden="true" />;
}

/** Toolbar holds the actions above a list: filters on the left, buttons on the right. */
export const Toolbar = Object.assign(ToolbarRoot, { Spacer });
