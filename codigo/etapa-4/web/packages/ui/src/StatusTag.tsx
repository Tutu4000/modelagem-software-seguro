import { cx } from "./cx";
import styles from "./StatusTag.module.css";

export type Tone = "neutral" | "info" | "progress" | "success" | "warning" | "danger";

export interface StatusTagProps {
  tone: Tone;
  /** Always a text: the color alone never carries the meaning. */
  children: string;
}

export function StatusTag({ tone, children }: StatusTagProps) {
  return <span className={cx(styles.tag, styles[tone])}>{children}</span>;
}
