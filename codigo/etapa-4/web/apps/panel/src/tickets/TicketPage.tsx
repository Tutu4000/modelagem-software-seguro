import { ApiError, unwrap } from "@nugget/api";
import { cleanText, maxMessage, ticketStatus } from "@nugget/domain";
import { Banner, Button, Conversation, Field, Grid, KeyValue, Panel, Skeleton, Stack, StatusTag, TextArea } from "@nugget/ui";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { Link, useParams } from "react-router";

import { useApi, useMe } from "../api";
import { stillOpen } from "./work";

const when = new Intl.DateTimeFormat("pt-BR", { dateStyle: "short", timeStyle: "short" });

/** TicketPage is one conversation, as plain text (E8.6): the support answers
 * and closes. The customer's e-mail only says there is an answer. */
export function TicketPage() {
  const { id = "" } = useParams();
  const api = useApi();
  const me = useMe();
  const client = useQueryClient();
  const [reply, setReply] = useState("");
  const [tried, setTried] = useState(false);
  const ticket = useQuery({ queryKey: ["ticket", id], queryFn: () => unwrap(api.GET("/panel/v1/tickets/{id}", { params: { path: { id } } })) });
  const refresh = async () => {
    await client.invalidateQueries({ queryKey: ["ticket", id] });
    await client.invalidateQueries({ queryKey: ["tickets"] });
  };
  const send = useMutation({
    mutationFn: (message: string) => unwrap(api.POST("/panel/v1/tickets/{id}/messages", { params: { path: { id } }, body: { message } })),
    onSuccess: async () => {
      setReply("");
      setTried(false);
      await refresh();
    },
  });
  const close = useMutation({ mutationFn: () => unwrap(api.POST("/panel/v1/tickets/{id}/close", { params: { path: { id } } })), onSuccess: refresh });

  if (ticket.isPending) {
    return <Skeleton lines={8} />;
  }
  if (ticket.isError) {
    const missing = ticket.error instanceof ApiError && (ticket.error.status === 403 || ticket.error.status === 404);
    return (
      <Panel>
        <Panel.Body>
          <Stack gap={3}>
            <h1>{missing ? "Chamado não encontrado" : "Não foi possível carregar o chamado"}</h1>
            <Link to="/chamados">Voltar aos chamados</Link>
          </Stack>
        </Panel.Body>
      </Panel>
    );
  }

  const t = ticket.data;
  const v = ticketStatus(t.status, "staff");
  const r = cleanText(reply, maxMessage);
  const conflict = [send.error, close.error].some((e) => e instanceof ApiError && e.status === 409);
  const open = stillOpen(t) && !conflict;
  return (
    <Stack gap={4}>
      <Stack direction="row" gap={3} align="center" justify="between">
        <Stack gap={1}>
          <h1>{t.subject}</h1>
          <small>
            Chamado <span className="mono">{t.id.slice(0, 8)}</span> · aberto em {when.format(new Date(t.created_at))}
          </small>
        </Stack>
        <span data-testid="ticket-status">
          <StatusTag tone={v.tone}>{v.label}</StatusTag>
        </span>
      </Stack>
      <Grid columns="sidebar">
        <Stack gap={4}>
          <Conversation label="Mensagens do chamado">
            {(t.messages ?? []).map((msg) => (
              <Conversation.Message
                key={msg.id}
                side={msg.author_kind === "staff" ? "self" : "other"}
                author={msg.author_kind === "staff" ? "Equipe" : "Cliente"}
                at={when.format(new Date(msg.created_at))}
              >
                {msg.body}
              </Conversation.Message>
            ))}
          </Conversation>
          {open ? (
            <Panel>
              <Panel.Body>
                <Field label="Resposta" hint={`${String(r.ok ? r.length : reply.length)}/${String(maxMessage)} · o cliente recebe só um aviso por e-mail`} error={tried && !r.ok ? r.reason : undefined}>
                  {(c) => <TextArea {...c} rows={4} value={reply} onChange={(e) => { setReply(e.target.value); }} />}
                </Field>
              </Panel.Body>
              <Panel.Footer>
                <Button variant="ghost" disabled={close.isPending} onClick={() => { close.mutate(); }}>
                  Fechar chamado
                </Button>
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
                  Responder
                </Button>
              </Panel.Footer>
            </Panel>
          ) : (
            <Banner tone="info">Este chamado está fechado.</Banner>
          )}
          {(send.isError || close.isError) && !conflict && <Banner tone="danger">A operação não foi concluída. Tente de novo.</Banner>}
        </Stack>
        <Panel>
          <Panel.Header>
            <h2>Cliente</h2>
          </Panel.Header>
          <Panel.Body>
            <Stack gap={3}>
              <KeyValue
                pairs={[
                  { label: "Conta", value: t.owner_id, mono: true, testId: "ticket-owner" },
                  {
                    label: "Pedido",
                    value: t.order_id === null ? "Nenhum" : <Link to={`/pedidos/${t.order_id}?chamado=${t.id}`} className="mono">{t.order_id.slice(0, 8)}</Link>,
                  },
                ]}
              />
              {me.data?.role === "admin" && <Link to={`/revogar?usuario=${t.owner_id}`}>Revogar acesso do cliente</Link>}
            </Stack>
          </Panel.Body>
        </Panel>
      </Grid>
    </Stack>
  );
}
