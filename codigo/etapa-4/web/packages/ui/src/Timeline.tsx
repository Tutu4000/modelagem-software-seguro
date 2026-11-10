import { cx } from "./cx";
import styles from "./Timeline.module.css";

export interface TimelineItem {
  key: string;
  label: string;
  state: "done" | "current" | "pending";
  at?: string;
}

/** Timeline shows the steps of an order, done, current and still ahead. */
export function Timeline({ items, label }: { items: readonly TimelineItem[]; label: string }) {
  return (
    <ol className={styles.timeline} aria-label={label}>
      {items.map((item) => (
        <li key={item.key} className={cx(styles.item, styles[item.state])} aria-current={item.state === "current" ? "step" : undefined}>
          <span className={styles.label}>{item.label}</span>
          {item.at !== undefined && <time className={styles.at}>{item.at}</time>}
        </li>
      ))}
    </ol>
  );
}
