import { cx } from "./cx";
import styles from "./Steps.module.css";

export interface StepsProps {
  steps: readonly string[];
  /** Index of the current step. */
  current: number;
  label: string;
}

/** Steps is the trail of a flow: done, current and ahead. */
export function Steps({ steps, current, label }: StepsProps) {
  return (
    <ol className={styles.steps} aria-label={label}>
      {steps.map((s, i) => (
        <li key={s} className={cx(styles.step, i < current && styles.done, i === current && styles.current)} aria-current={i === current ? "step" : undefined}>
          <span className={styles.number}>{String(i + 1).padStart(2, "0")}</span>
          {s}
        </li>
      ))}
    </ol>
  );
}
