import type { ReactNode } from "react";

import { cx } from "./cx";
import styles from "./DataTable.module.css";

export interface Column<T> {
  /** The field shown when there is no render. */
  key: keyof T & string;
  header: string;
  render?: (row: T) => ReactNode;
  align?: "start" | "end";
}

export interface DataTableProps<T> {
  columns: readonly Column<T>[];
  rows: readonly T[];
  rowKey: (row: T) => string;
  caption: string;
  empty?: ReactNode;
}

function cell<T>(row: T, column: Column<T>): ReactNode {
  if (column.render !== undefined) {
    return column.render(row);
  }
  const value = row[column.key];
  return typeof value === "string" || typeof value === "number" ? value : null;
}

/** DataTable is a table on tablets and desktops and a list of cards on phones. */
export function DataTable<T>({ columns, rows, rowKey, caption, empty = "Nada por aqui ainda." }: DataTableProps<T>) {
  if (rows.length === 0) {
    return <p className={styles.empty}>{empty}</p>;
  }
  return (
    <table className={styles.table}>
      <caption className="visually-hidden">{caption}</caption>
      <thead>
        <tr>
          {columns.map((c) => (
            <th key={c.key} scope="col" className={cx(c.align === "end" && styles.end)}>
              {c.header}
            </th>
          ))}
        </tr>
      </thead>
      <tbody>
        {rows.map((row) => (
          <tr key={rowKey(row)}>
            {columns.map((c) => (
              <td key={c.key} data-label={c.header} className={cx(c.align === "end" && styles.end)}>
                {cell(row, c)}
              </td>
            ))}
          </tr>
        ))}
      </tbody>
    </table>
  );
}
