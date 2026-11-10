import { unwrap } from "@nugget/api";
import { ticketStatus, type Ticket } from "@nugget/domain";
import { Banner, DataTable, EmptyState, ShieldIcon, Skeleton, Stack, StatusTag, Toolbar } from "@nugget/ui";
import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router";

import { useApi } from "../api";

const when = new Intl.DateTimeFormat("pt-BR", { dateStyle: "short", timeStyle: "short" });

/** TicketsPage lists the customer's support tickets (RF11). */
export function TicketsPage() {
  const api = useApi();
  const tickets = useQuery({ queryKey: ["tickets"], queryFn: () => unwrap(api.GET("/api/v1/tickets")) });
  const rows = tickets.data?.tickets ?? [];
  return (
    <Stack gap={4}>
      <Toolbar>
        <h1>Suporte</h1>
        <Toolbar.Spacer />
        <Link to="/suporte/novo">Abrir chamado</Link>
      </Toolbar>
      {tickets.isPending ? (
        <Skeleton lines={5} />
      ) : tickets.isError ? (
        <Banner tone="danger">Não foi possível carregar os chamados.</Banner>
      ) : rows.length === 0 ? (
        <EmptyState icon={<ShieldIcon />} action={<Link to="/suporte/novo">Abrir chamado</Link>}>
          Nenhum chamado ainda. Fale com o suporte sobre um pedido ou qualquer dúvida.
        </EmptyState>
      ) : (
        <DataTable<Ticket>
          caption="Meus chamados"
          rows={rows}
          rowKey={(t) => t.id}
          columns={[
            { key: "id", header: "Chamado", render: (t) => <Link to={`/suporte/${t.id}`} className="mono">{t.id.slice(0, 8)}</Link> },
            { key: "subject", header: "Assunto" },
            {
              key: "order_id",
              header: "Pedido",
              render: (t) => (t.order_id === null ? "—" : <Link to={`/pedidos/${t.order_id}`} className="mono">{t.order_id.slice(0, 8)}</Link>),
            },
            {
              key: "status",
              header: "Estado",
              render: (t) => {
                const v = ticketStatus(t.status);
                return <StatusTag tone={v.tone}>{v.label}</StatusTag>;
              },
            },
            { key: "updated_at", header: "Atualizado", render: (t) => when.format(new Date(t.updated_at)) },
          ]}
        />
      )}
    </Stack>
  );
}
