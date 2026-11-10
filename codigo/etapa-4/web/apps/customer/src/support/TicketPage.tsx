import { ApiError, unwrap } from "@nugget/api";
import { cleanText, maxMessage, ticketStatus } from "@nugget/domain";
import { Banner, Button, Conversation, Field, Panel, Skeleton, Stack, StatusTag, TextArea } from "@nugget/ui";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { Link, useParams } from "react-router";

import { useApi } from "../api";

const when = new Intl.DateTimeFormat("pt-BR", { dateStyle: "short", timeStyle: "short" });

/** TicketPage is one conversation with the support. The staff's answer shows
 * up by itself while the ticket is open. */
export function TicketPage() {
  const { id = "" } = useParams();
  const api = useApi();
  const client = useQueryClient();
  const [reply, setReply] = useState("");
  const [tried, setTried] = useState(false);
  const ticket = useQuery({
    queryKey: ["ticket", id],
    queryFn: () => unwrap(api.GET("/api/v1/tickets/{id}", { params: { path: { id } } })),
    refetchInterval: (q) => (q.state.data === undefined || q.state.data.status === "closed" ? false : 20_000),
  });
  const send = useMutation({
    mutationFn: (message: string) => unwrap(api.POST("/api/v1/tickets/{id}/messages", { params: { path: { id } }, body: { message } })),
    onSuccess: async () => {
      setReply("");
      setTried(false);
      await client.invalidateQueries({ queryKey: ["ticket", id] });
      await client.invalidateQueries({ queryKey: ["tickets"] });
    },
  });

  if (ticket.isPending) {
    return <Skeleton lines={8} />;
  }
  if (ticket.isError) {
    const missing = ticket.error instanceof ApiError && ticket.error.status === 403;
    return (
      <Panel>
        <Panel.Body>
          <Stack gap={3}>
            <h1>{missing ? "Chamado não encontrado" : "Não foi possível carregar o chamado"}</h1>
            <Link to="/suporte">Voltar ao suporte</Link>
          </Stack>
        </Panel.Body>
      </Panel>
    );
  }

  const t = ticket.data;
  const v = ticketStatus(t.status);
  const r = cleanText(reply, maxMessage);
  const closed = t.status === "closed" || (send.error instanceof ApiError && send.error.status === 409);
  return (
    <Stack gap={4}>
      <Stack direction="row" gap={3} align="center" justify="between">
        <Stack gap={1}>
          <h1>{t.subject}</h1>
          <small>
            Chamado <span className="mono">{t.id.slice(0, 8)}</span>
            {t.order_id !== null && (
              <>
                {" · pedido "}
                <Link to={`/pedidos/${t.order_id}`} className="mono">
                  {t.order_id.slice(0, 8)}
                </Link>
              </>
            )}
          </small>
        </Stack>
        <span data-testid="ticket-status">
          <StatusTag tone={v.tone}>{v.label}</StatusTag>
        </span>
      </Stack>

      <Conversation label="Mensagens do chamado">
        {(t.messages ?? []).map((msg) => (
          <Conversation.Message key={msg.id} side={msg.author_kind === "customer" ? "self" : "other"} author={msg.author_kind === "customer" ? "Você" : "Suporte"} at={when.format(new Date(msg.created_at))}>
            {msg.body}
          </Conversation.Message>
        ))}
      </Conversation>

      {closed ? (
        <Banner tone="info" action={<Link to="/suporte/novo">Abrir novo chamado</Link>}>
          Este chamado foi fechado.
        </Banner>
      ) : (
        <Panel>
          <Panel.Body>
            <Field label="Responder" hint={`${String(r.ok ? r.length : reply.length)}/${String(maxMessage)}`} error={tried && !r.ok ? r.reason : undefined}>
              {(c) => <TextArea {...c} rows={4} value={reply} onChange={(e) => { setReply(e.target.value); }} />}
            </Field>
          </Panel.Body>
          <Panel.Footer>
            <Button
              variant="primary"
              disabled={send.isPending}
              onClick={() => {
                setTried(true);
                if (r.ok) {
                  send.mutate(r.text);
                }
              }}
            >
              Enviar
            </Button>
          </Panel.Footer>
        </Panel>
      )}
    </Stack>
  );
}
