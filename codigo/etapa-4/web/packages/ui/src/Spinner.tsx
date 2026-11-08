import styles from "./Spinner.module.css";

/** Spinner says what is loading, in words, next to a square that turns. */
export function Spinner({ label }: { label: string }) {
  return (
    <span className={styles.spinner} role="status">
      <span className={styles.box} aria-hidden="true" />
      {label}
    </span>
  );
}

/** Skeleton holds the place of a line of text while it loads. */
export function Skeleton({ lines = 1 }: { lines?: number }) {
  return (
    <span aria-hidden="true">
      {Array.from({ length: lines }, (_, i) => (
        <span key={i} className={styles.skeleton} />
      ))}
    </span>
  );
}
