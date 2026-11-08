import type { ComponentPropsWithoutRef } from "react";

import { TextInput } from "./Field";

export interface CodeInputProps extends Omit<ComponentPropsWithoutRef<"input">, "value" | "onChange" | "maxLength"> {
  value: string;
  onChange: (digits: string) => void;
  length: number;
}

/** CodeInput is a numeric code in one field: pasting works, and screen
 * readers read one value, not a box per digit. */
export function CodeInput({ value, onChange, length, ...rest }: CodeInputProps) {
  return (
    <TextInput
      {...rest}
      className="mono"
      inputMode="numeric"
      autoComplete="one-time-code"
      maxLength={length}
      value={value}
      onChange={(e) => { onChange(digitsOnly(e.target.value, length)); }}
    />
  );
}

export function digitsOnly(input: string, length: number): string {
  return input.replace(/\D/g, "").slice(0, length);
}
