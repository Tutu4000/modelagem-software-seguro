import { ApiError, unwrap } from "@nugget/api";
import { Banner, Button, CodeInput, Field, Panel, Stack } from "@nugget/ui";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { useApi } from "../api";

const until = new Intl.DateTimeFormat("pt-BR", { dateStyle: "short", timeStyle: "short" });

/** confirmMessage explains a refused code: wrong, or the change closed. */
export function confirmMessage(e: unknown): string | undefined {
  if (!(e instanceof ApiError)) {
    return undefined;
  }
  if (e.code === "invalid_code") {
    return "Código incorreto. Depois de 5 erros a troca é bloqueada.";
  }
  if (e.status === 409) {
    return "A troca não está mais aberta: venceu, foi bloqueada ou o pedido já foi despachado. Fale com o suporte.";
  }
  return undefined;
}

/** AddressChangePanel is the customer's half of C21: support proposed a new
 * address, and it changes only with the code sent to the registered e-mail. */
export function AddressChangePanel({ orderId }: { orderId: string }) {
  const api = useApi();
  const client = useQueryClient();
  const [code, setCode] = useState("");
  const pending = useQuery({
    queryKey: ["address-change", orderId],
    // No change is a 409: the panel then simply does not show.
    queryFn: async () => {
      try {
        return await unwrap(api.GET("/api/v1/orders/{id}/address-change", { params: { path: { id: orderId } } }));
      } catch (e) {
        if (e instanceof ApiError && e.status === 409) {
          return null;
        }
        throw e;
      }
    },
  });
  const confirm = useMutation({
    mutationFn: () => unwrap(api.POST("/api/v1/orders/{id}/address-change/confirm", { params: { path: { id: orderId } }, body: { code } })),
    onSuccess: async () => {
      setCode("");
      await client.invalidateQueries({ queryKey: ["order", orderId] });
      await client.invalidateQueries({ queryKey: ["address-change", orderId] });
    },
    onError: async (e) => {
      if (e instanceof ApiError && e.status === 409) {
        await client.invalidateQueries({ queryKey: ["address-change", orderId] });
      }
    },
  });

  if (confirm.isSuccess) {
    return <Banner tone="info">Endereço de entrega alterado. Você recebeu a confirmação por e-mail.</Banner>;
  }
  const change = pending.data;
  if (change === undefined || change === null) {
    return confirm.isError ? <Banner tone="warning">{confirmMessage(confirm.error)}</Banner> : null;
  }
  const a = change.address;
  return (
    <Panel accent>
      <Panel.Header>
        <h2>Troca de endereço proposta</h2>
      </Panel.Header>
      <Panel.Body>
        <Stack gap={3}>
          <p>O suporte registrou um pedido de troca do endereço de entrega para:</p>
          <address data-testid="proposed-address">
            {a.recipient}
            <br />
            {a.street}, {a.number}
            <br />
            {a.city} - {a.state}, {a.postal_code}
          </address>
          <Banner tone="warning">Só informe o código se foi você quem pediu a troca. Sem o código, o endereço não muda.</Banner>
          <Field label="Código enviado ao seu e-mail" hint={`Vale até ${until.format(new Date(change.expires_at))}`} error={confirmMessage(confirm.error)}>
            {(c) => <CodeInput {...c} length={8} value={code} onChange={setCode} />}
          </Field>
        </Stack>
      </Panel.Body>
      <Panel.Footer>
        <Button variant="primary" disabled={code.length !== 8 || confirm.isPending} onClick={() => { confirm.mutate(); }}>
          Confirmar troca
        </Button>
      </Panel.Footer>
    </Panel>
  );
}
