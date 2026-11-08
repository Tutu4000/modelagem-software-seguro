import type { ReactNode } from "react";

import styles from "./Banner.module.css";
import { cx } from "./cx";

export interface BannerProps {
  tone: "info" | "warning" | "danger";
  children: ReactNode;
  /** An action, such as a "try again" button. */
  action?: ReactNode;
}

/** Banner is a message about the page. Danger interrupts (role="alert"); the
 * others are announced politely (role="status"). */
export function Banner({ tone, children, action }: BannerProps) {
  return (
    <div className={cx(styles.banner, styles[tone])} role={tone === "danger" ? "alert" : "status"}>
      <div className={styles.content}>{children}</div>
      {action}
    </div>
  );
}
