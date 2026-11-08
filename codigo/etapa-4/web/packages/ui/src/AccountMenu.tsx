import type { ReactNode } from "react";

import styles from "./AccountMenu.module.css";

export interface AccountMenuProps {
  username: string;
  email?: string | undefined;
  role: string;
  /** Links or buttons, rendered as the menu items. */
  children: ReactNode;
}

/** AccountMenu shows who is logged in and the account actions, in a native
 * disclosure: keyboard and screen readers work with no script. */
export function AccountMenu({ username, email, role, children }: AccountMenuProps) {
  return (
    <details className={styles.menu}>
      <summary>
        <span>{username}</span>
        <span className={styles.role}>{role}</span>
      </summary>
      <div className={styles.items}>
        {email !== undefined && <span className={styles.email}>{email}</span>}
        {children}
      </div>
    </details>
  );
}
