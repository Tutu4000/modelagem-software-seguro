import { useId, useState, type ReactNode } from "react";

import { cx } from "./cx";
import styles from "./FileDrop.module.css";
import { UploadIcon } from "./icons";

export interface FileDropProps {
  accept: string;
  onFile: (file: File) => void;
  disabled?: boolean;
  /** What files are accepted, in words. */
  hint: ReactNode;
}

/** FileDrop takes one file, dropped or picked with the native input: the
 * keyboard and screen readers use the input, the mouse may drop. */
export function FileDrop({ accept, onFile, disabled = false, hint }: FileDropProps) {
  const id = useId();
  const [over, setOver] = useState(false);
  return (
    <div
      className={cx(styles.drop, over && styles.active)}
      onDragOver={(e) => {
        e.preventDefault();
        setOver(!disabled);
      }}
      onDragLeave={() => { setOver(false); }}
      onDrop={(e) => {
        e.preventDefault();
        setOver(false);
        const file = e.dataTransfer.files.item(0);
        if (!disabled && file !== null) {
          onFile(file);
        }
      }}
    >
      <UploadIcon />
      <span>Arraste o arquivo aqui ou</span>
      <input
        id={id}
        className={styles.input}
        type="file"
        accept={accept}
        disabled={disabled}
        onChange={(e) => {
          const file = e.target.files?.item(0) ?? null;
          if (file !== null) {
            onFile(file);
          }
          e.target.value = "";
        }}
      />
      <label htmlFor={id} className={styles.pick}>
        Escolher arquivo
      </label>
      <small>{hint}</small>
    </div>
  );
}
