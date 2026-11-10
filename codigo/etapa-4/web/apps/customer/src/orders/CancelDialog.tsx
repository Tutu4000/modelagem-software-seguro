import { ApiError, unwrap } from "@nugget/api";
import { Banner, Button, Dialog, Field, Stack, TextArea } from "@nugget/ui";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";

import { useApi } from "../api";

export const maxReason = 500;

/** reasonTooLong counts code points, like the API. */
export function reasonTooLong(reason: string): boolean {
  let n = 0;
  for (let i = 0; i < reason.length; i++) {
    const unit = reason.charCodeAt(i);
    if (unit >= 0xd800 && unit <= 0xdbff && i + 1 < reason.length) {
      i++;
    }
    n++;
  }
  return n > maxReason;
}

export interface CancelDialogProps {
  orderId: string;
  open: boolean;
  onClose: () => void;
  /** A paid order is refunded: the dialog says so. */
  paid: boolean;
}

/** CancelDialog cancels the order (RF10), with an optional reason that goes
 * to the audit trail (T11). */
export function CancelDialog({ orderId, open, onClose, paid }: CancelDialogProps) {
  const api = useApi();
  const client = useQueryClient();
  const titleId = useId();
  const [reason, setReason] = useState("");
  const cancel = useMutation({
    mutationFn: () => unwrap(api.POST("/api/v1/orders/{id}/cancel", { params: { path: { id: orderId } }, body: { reason: reason.trim() } })),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: ["order", orderId] });
      await client.invalidateQueries({ queryKey: ["orders"] });
      setReason("");
      onClose();
    },
  });
  const tooLong = reasonTooLong(reason.trim());
  const refused = cancel.error instanceof ApiError && cancel.error.status === 409;

  return (
    <Dialog open={open} onClose={onClose} labelledBy={titleId}>
      <Dialog.Header>
        <h2 id={titleId}>Cancelar o pedido</h2>
      </Dialog.Header>
      <Dialog.Body>
        <Stack gap={3}>
          <p>{paid ? "O pedido já foi pago: o valor é estornado pelo gateway." : "O pedido é cancelado e não segue para a produção."}</p>
          <Field label="Motivo (opcional)" hint={`Até ${String(maxReason)} caracteres`} error={tooLong ? "O motivo está longo demais." : undefined}>
            {(c) => <TextArea {...c} rows={3} value={reason} onChange={(e) => { setReason(e.target.value); }} />}
          </Field>
          {refused && <Banner tone="warning">Este pedido não pode mais ser cancelado pelo app. Abra um chamado no suporte.</Banner>}
        </Stack>
      </Dialog.Body>
      <Dialog.Footer>
        <Button onClick={onClose} disabled={cancel.isPending}>
          Voltar
        </Button>
        <Button variant="primary" disabled={tooLong || cancel.isPending} onClick={() => { cancel.mutate(); }}>
          Cancelar pedido
        </Button>
      </Dialog.Footer>
    </Dialog>
  );
}
