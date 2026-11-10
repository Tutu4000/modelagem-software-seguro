import type { ComponentPropsWithoutRef } from "react";

import { cx } from "./cx";
import styles from "./Stack.module.css";

export type Gap = 1 | 2 | 3 | 4 | 5 | 6;

export interface StackProps extends ComponentPropsWithoutRef<"div"> {
  direction?: "column" | "row";
  gap?: Gap;
  align?: "start" | "center" | "end";
  justify?: "between" | "end";
}

const alignClass = { start: styles.alignStart, center: styles.alignCenter, end: styles.alignEnd } as const;
const justifyClass = { between: styles.justifyBetween, end: styles.justifyEnd } as const;

/** Stack lays its children out in one direction, with a gap from the scale. */
export function Stack({ direction = "column", gap = 3, align, justify, className, ...rest }: StackProps) {
  return (
    <div
      className={cx(
        styles.stack,
        direction === "row" && styles.row,
        styles[`gap${String(gap)}`],
        align !== undefined && alignClass[align],
        justify !== undefined && justifyClass[justify],
        className,
      )}
      {...rest}
    />
  );
}
