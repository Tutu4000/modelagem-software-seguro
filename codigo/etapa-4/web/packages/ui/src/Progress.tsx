import styles from "./Progress.module.css";

/** Progress is a square bar, value from 0 to 1. The width is set through
 * the CSSOM, which the CSP allows; no inline style attribute is written. */
export function Progress({ value, label }: { value: number; label: string }) {
  const pct = Math.round(Math.min(1, Math.max(0, value)) * 100);
  return (
    <div className={styles.track} role="progressbar" aria-label={label} aria-valuemin={0} aria-valuemax={100} aria-valuenow={pct}>
      <div
        className={styles.bar}
        ref={(el) => {
          if (el !== null) {
            el.style.width = `${String(pct)}%`;
          }
        }}
      />
    </div>
  );
}
