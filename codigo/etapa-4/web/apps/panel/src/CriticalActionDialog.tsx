import { isStepUp } from "@nugget/api";
import { useSession } from "@nugget/auth";
import { Banner, Button, Dialog, Field, Stack, TextArea } from "@nugget/ui";
import { useId, useState, type ReactNode } from "react";

export const minJustification = 10;
export const maxJustification = 500;
/** Just under the panel's 5 minutes, so the request does not race the limit. */
const stepUpMaxAgeMs = 4.5 * 60 * 1000;

/** codePoints counts like the panel API (utf8.RuneCountInString), so the
 * limits here and there agree. */
export function codePoints(text: string): number {
  let n = 0;
  for (let i = 0; i < text.length; i++) {
    const unit = text.charCodeAt(i);
    // A high surrogate and the low one after it are one code point.
    if (unit >= 0xd800 && unit <= 0xdbff && i + 1 < text.length) {
      i++;
    }
    n++;
  }
  return n;
}

export function justificationError(text: string): string | undefined {
  const n = codePoints(text.trim());
  if (n < minJustification) {
    return `Escreva pelo menos ${String(minJustification)} caracteres.`;
  }
  if (n > maxJustification) {
    return `Use no máximo ${String(maxJustification)} caracteres.`;
  }
  return undefined;
}

export interface CriticalActionDialogProps {
  open: boolean;
  onClose: () => void;
  title: string;
  /** What will happen, in plain words. */
  children: ReactNode;
  confirmLabel: string;
  /** Runs the operation with the justification; throws an ApiError when the
   * API refuses. */
  onConfirm: (justification: string) => Promise<void>;
}

/** CriticalActionDialog is every critical operation of the panel (D02, E6.4):
 * a written justification, a login with MFA of at most 5 minutes (asked in a
 * popup when needed) and one retry if the API still asks for it. */
export function CriticalActionDialog({ open, onClose, title, children, confirmLabel, onConfirm }: CriticalActionDialogProps) {
  const session = useSession();
  const titleId = useId();
  const [text, setText] = useState("");
  const [touched, setTouched] = useState(false);
  const [busy, setBusy] = useState(false);
  const [failure, setFailure] = useState<string | undefined>();
  const error = touched ? justificationError(text) : undefined;

  const fresh = () =>
    session.state.status === "authenticated" && Date.now() - session.state.claims.authTime.getTime() < stepUpMaxAgeMs;

  const submit = async () => {
    setTouched(true);
    if (justificationError(text) !== undefined) {
      return;
    }
    setBusy(true);
    setFailure(undefined);
    try {
      if (!fresh()) {
        await session.stepUp();
      }
      try {
        await onConfirm(text.trim());
      } catch (e) {
        if (!isStepUp(e)) {
          throw e;
        }
        await session.stepUp();
        await onConfirm(text.trim());
      }
      setText("");
      setTouched(false);
      onClose();
    } catch {
      setFailure("A operação não foi concluída. Confira a mensagem e tente de novo.");
    } finally {
      setBusy(false);
    }
  };

  return (
    <Dialog open={open} onClose={onClose} labelledBy={titleId}>
      <Dialog.Header>
        <h2 id={titleId}>{title}</h2>
      </Dialog.Header>
      <Dialog.Body>
        <Stack gap={3}>
          {children}
          <Field label="Justificativa" hint={`${String(codePoints(text.trim()))}/${String(maxJustification)} · fica registrada na auditoria`} error={error}>
            {(c) => (
              <TextArea
                {...c}
                rows={4}
                value={text}
                onChange={(e) => { setText(e.target.value); }}
                onBlur={() => { setTouched(true); }}
              />
            )}
          </Field>
          {failure !== undefined && <Banner tone="danger">{failure}</Banner>}
        </Stack>
      </Dialog.Body>
      <Dialog.Footer>
        <Button onClick={onClose} disabled={busy}>
          Cancelar
        </Button>
        <Button variant="primary" onClick={() => void submit()} disabled={busy}>
          {confirmLabel}
        </Button>
      </Dialog.Footer>
    </Dialog>
  );
}
