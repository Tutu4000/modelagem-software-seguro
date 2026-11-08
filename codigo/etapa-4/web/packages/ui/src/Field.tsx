import { useId, type ComponentPropsWithoutRef, type ReactNode } from "react";

import { cx } from "./cx";
import styles from "./Field.module.css";

/** What Field hands to its control, so the label, the hint and the error
 * reach assistive technology. */
export interface FieldControlProps {
  id: string;
  "aria-describedby"?: string;
  "aria-invalid"?: true;
}

export interface FieldProps {
  label: ReactNode;
  hint?: ReactNode;
  error?: ReactNode;
  children: (control: FieldControlProps) => ReactNode;
}

/** Field composes a label, any control, a hint and an error. */
export function Field({ label, hint, error, children }: FieldProps) {
  const id = useId();
  const hintId = `${id}-hint`;
  const errorId = `${id}-error`;
  const described = [hint !== undefined ? hintId : undefined, error !== undefined ? errorId : undefined].filter(
    (x): x is string => x !== undefined,
  );
  const control: FieldControlProps = { id };
  if (described.length > 0) {
    control["aria-describedby"] = described.join(" ");
  }
  if (error !== undefined) {
    control["aria-invalid"] = true;
  }
  return (
    <div className={styles.field}>
      <label htmlFor={id} className={styles.label}>
        {label}
      </label>
      {children(control)}
      {hint !== undefined && (
        <span id={hintId} className={styles.hint}>
          {hint}
        </span>
      )}
      {error !== undefined && (
        <span id={errorId} className={styles.error} role="alert">
          {error}
        </span>
      )}
    </div>
  );
}

export function TextInput({ className, ...rest }: ComponentPropsWithoutRef<"input">) {
  return <input className={cx(styles.control, className)} {...rest} />;
}

export function Select({ className, ...rest }: ComponentPropsWithoutRef<"select">) {
  return <select className={cx(styles.control, className)} {...rest} />;
}
