import { ApiError, unwrap } from "@nugget/api";
import { cleanText, maxMessage, maxSubject } from "@nugget/domain";
import { Banner, Button, Field, Panel, Select, Stack, TextArea, TextInput } from "@nugget/ui";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useNavigate, useSearchParams } from "react-router";

import { useApi } from "../api";

function refusal(e: unknown): string | undefined {
  if (!(e instanceof ApiError)) {
    return undefined;
  }
  switch (e.status) {
    case 409:
      return "Você já tem 10 chamados abertos. Aguarde a resposta de algum deles.";
    case 403:
      return "Este pedido não é da sua conta.";
    case 400:
      return "Confira o assunto e a mensagem.";
    default:
      return undefined;
  }
}

/** NewTicketPage opens a ticket, about one of the customer's orders or not. */
export function NewTicketPage() {
  const api = useApi();
  const client = useQueryClient();
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const [order, setOrder] = useState(params.get("pedido") ?? "");
  const [subject, setSubject] = useState("");
  const [message, setMessage] = useState("");
  const [tried, setTried] = useState(false);
  const orders = useQuery({ queryKey: ["orders"], queryFn: () => unwrap(api.GET("/api/v1/orders")) });

  const s = cleanText(subject, maxSubject);
  const m = cleanText(message, maxMessage);
  const open = useMutation({
    mutationFn: (body: { subject: string; message: string }) =>
      unwrap(api.POST("/api/v1/tickets", { body: order === "" ? body : { ...body, order_id: order } })),
    onSuccess: async (t) => {
      await client.invalidateQueries({ queryKey: ["tickets"] });
      void navigate(`/suporte/${t.id}`);
    },
  });

  return (
    <Panel accent>
      <Panel.Header>
        <h1>Abrir chamado</h1>
      </Panel.Header>
      <Panel.Body>
        <Stack gap={4}>
          <Field label="Pedido">
            {(c) => (
              <Select {...c} value={order} onChange={(e) => { setOrder(e.target.value); }}>
                <option value="">Sem pedido</option>
                {(orders.data?.orders ?? []).map((o) => (
                  <option key={o.id} value={o.id}>
                    {o.id.slice(0, 8)}
                  </option>
                ))}
                {order !== "" && !(orders.data?.orders ?? []).some((o) => o.id === order) && <option value={order}>{order.slice(0, 8)}</option>}
              </Select>
            )}
          </Field>
          <Field label="Assunto" hint={`${String(s.ok ? s.length : subject.length)}/${String(maxSubject)}`} error={tried && !s.ok ? s.reason : undefined}>
            {(c) => <TextInput {...c} value={subject} onChange={(e) => { setSubject(e.target.value); }} />}
          </Field>
          <Field label="Mensagem" hint={`${String(m.ok ? m.length : message.length)}/${String(maxMessage)}`} error={tried && !m.ok ? m.reason : undefined}>
            {(c) => <TextArea {...c} rows={6} value={message} onChange={(e) => { setMessage(e.target.value); }} />}
          </Field>
          {refusal(open.error) !== undefined && <Banner tone="danger">{refusal(open.error)}</Banner>}
        </Stack>
      </Panel.Body>
      <Panel.Footer>
        <Button
          variant="primary"
          disabled={open.isPending}
          onClick={() => {
            setTried(true);
            if (s.ok && m.ok) {
              open.mutate({ subject: s.text, message: m.text });
            }
          }}
        >
          Enviar
        </Button>
      </Panel.Footer>
    </Panel>
  );
}
