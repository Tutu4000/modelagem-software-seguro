import type { ReactNode } from "react";

import styles from "./EmptyState.module.css";

/** EmptyState says there is nothing yet, and what to do about it. */
export function EmptyState({ icon, children, action }: { icon?: ReactNode; children: ReactNode; action?: ReactNode }) {
  return (
    <div className={styles.empty}>
      {icon}
      <p>{children}</p>
      {action}
    </div>
  );
}
