import type { ReactNode } from "react";

import styles from "./Conversation.module.css";
import { cx } from "./cx";

function ConversationRoot({ label, children }: { label: string; children: ReactNode }) {
  return (
    <ol className={styles.conversation} aria-label={label}>
      {children}
    </ol>
  );
}

export interface MessageProps {
  side: "self" | "other";
  author: string;
  at: string;
  /** Text typed by a person. It is a string on purpose: no ReactNode, so no
   * markup can travel through here, and React renders it as a text node. */
  children: string;
}

function Message({ side, author, at, children }: MessageProps) {
  return (
    <li className={cx(styles.message, styles[side])}>
      <div className={styles.meta}>
        <span className={styles.author}>{author}</span>
        <time>{at}</time>
      </div>
      <p className={styles.body}>{children}</p>
    </li>
  );
}

/** Conversation is a thread of messages between the customer and the staff. */
export const Conversation = Object.assign(ConversationRoot, { Message });
