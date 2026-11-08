import { useId, type ReactNode } from "react";

import styles from "./ChipGroup.module.css";

export interface Chip<V extends string> {
  value: V;
  label: string;
  detail?: ReactNode;
}

export interface ChipGroupProps<V extends string> {
  legend: string;
  options: readonly Chip<V>[];
  value: V | undefined;
  onChange: (value: V) => void;
  disabled?: boolean;
}

/** ChipGroup is one choice among short options: native radio buttons, so the
 * arrow keys and screen readers work as they do everywhere. */
export function ChipGroup<V extends string>({ legend, options, value, onChange, disabled = false }: ChipGroupProps<V>) {
  const name = useId();
  return (
    <fieldset className={styles.group} disabled={disabled}>
      <legend>{legend}</legend>
      {options.map((o) => (
        <label key={o.value} className={styles.chip}>
          <input type="radio" name={name} value={o.value} checked={value === o.value} onChange={() => { onChange(o.value); }} />
          <span>
            {o.label}
            {o.detail !== undefined && <small>{o.detail}</small>}
          </span>
        </label>
      ))}
    </fieldset>
  );
}
