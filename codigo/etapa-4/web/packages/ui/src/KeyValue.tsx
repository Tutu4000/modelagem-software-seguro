import type { ReactNode } from "react";

export interface Pair {
  label: string;
  value: ReactNode;
  mono?: boolean;
  testId?: string;
}

/** KeyValue is a list of labelled values: always text, never HTML. */
export function KeyValue({ pairs }: { pairs: readonly Pair[] }) {
  return (
    <dl>
      {pairs.map((p) => (
        <div key={p.label}>
          <dt>{p.label}</dt>
          <dd className={p.mono === true ? "mono" : undefined} data-testid={p.testId}>
            {p.value}
          </dd>
        </div>
      ))}
    </dl>
  );
}
