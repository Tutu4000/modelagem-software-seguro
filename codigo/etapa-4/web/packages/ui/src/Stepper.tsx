import type { FieldControlProps } from "./Field";
import styles from "./Stepper.module.css";

export interface StepperProps extends Partial<FieldControlProps> {
  value: number;
  min: number;
  max: number;
  onChange: (value: number) => void;
  /** Accessible names of the two buttons. */
  decreaseLabel?: string;
  increaseLabel?: string;
}

function clamp(n: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, n));
}

/** Stepper is a whole number between min and max, typed or stepped. */
export function Stepper({ value, min, max, onChange, decreaseLabel = "Diminuir", increaseLabel = "Aumentar", ...control }: StepperProps) {
  return (
    <div className={styles.stepper}>
      <button type="button" aria-label={decreaseLabel} disabled={value <= min} onClick={() => { onChange(clamp(value - 1, min, max)); }}>
        −
      </button>
      <input
        {...control}
        type="number"
        inputMode="numeric"
        min={min}
        max={max}
        value={value}
        onChange={(e) => {
          const n = Number.parseInt(e.target.value, 10);
          if (!Number.isNaN(n)) {
            onChange(clamp(n, min, max));
          }
        }}
      />
      <button type="button" aria-label={increaseLabel} disabled={value >= max} onClick={() => { onChange(clamp(value + 1, min, max)); }}>
        +
      </button>
    </div>
  );
}
