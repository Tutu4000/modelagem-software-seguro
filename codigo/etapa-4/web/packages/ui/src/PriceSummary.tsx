import { cx } from "./cx";
import styles from "./PriceSummary.module.css";

const brl = new Intl.NumberFormat("pt-BR", { style: "currency", currency: "BRL" });

/** money writes cents as reais. Prices travel as whole cents, never floats. */
export function money(cents: number): string {
  return brl.format(cents / 100);
}

export interface PriceLine {
  label: string;
  cents: number;
  /** Shown as a deduction. */
  discount?: boolean;
}

/** PriceSummary lists how a price is made and ends in the total. */
export function PriceSummary({ lines, totalCents, totalLabel = "Total" }: { lines: readonly PriceLine[]; totalCents: number; totalLabel?: string }) {
  return (
    <dl className={styles.summary}>
      {lines.map((l) => (
        <div key={l.label} className={cx(styles.line, l.discount === true && styles.discount)}>
          <dt>{l.label}</dt>
          <dd>{l.discount === true ? `− ${money(l.cents)}` : money(l.cents)}</dd>
        </div>
      ))}
      <div className={styles.total}>
        <dt>{totalLabel}</dt>
        <dd data-testid="total">{money(totalCents)}</dd>
      </div>
    </dl>
  );
}
