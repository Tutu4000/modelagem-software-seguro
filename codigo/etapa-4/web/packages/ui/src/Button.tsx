import type { ComponentPropsWithoutRef } from "react";

import { cx } from "./cx";
import styles from "./Button.module.css";

export interface ButtonProps extends ComponentPropsWithoutRef<"button"> {
  variant?: "primary" | "secondary" | "ghost";
  /** Full width, as on phones. */
  block?: boolean;
}

/** Button is a native button: the type defaults to "button", so it never
 * submits a form by accident. */
export function Button({ variant = "secondary", block = false, type = "button", className, ...rest }: ButtonProps) {
  return <button type={type} className={cx(styles.button, styles[variant], block && styles.block, className)} {...rest} />;
}
