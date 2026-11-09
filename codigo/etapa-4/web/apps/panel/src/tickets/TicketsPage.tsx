import { unwrap } from "@nugget/api";
import { ticketStatus } from "@nugget/domain";
import { Banner, ChipGroup, DataTable, EmptyState, ShieldIcon, Skeleton, Stack, StatusTag, Toolbar } from "@nugget/ui";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { Link } from "react-router";

import { useApi } from "../api";
import { ticketFilters, type StaffTicket, type TicketFilter } from "./work";

const when = new Intl.DateTimeFormat("pt-BR", { dateStyle: "short", timeStyle: "short" });

/** TicketsPage is the support queue, most recently updated first, filtered
 * by status on the server. */
export function TicketsPage() {
  const api = useApi();
  const [filter, setFilter] = useState<TicketFilter>("open");
  const tickets = useQuery({
    queryKey: ["tickets", filter],
    queryFn: () => unwrap(api.GET("/panel/v1/tickets", { params: { query: filter === "" ? {} : { status: filter } } })),
  });
  const rows = tickets.data?.tickets ?? [];
  return (
    <Stack gap={4}>
      <h1>Chamados</h1>
      <Toolbar>
        <ChipGroup legend="Estado" value={filter} options={ticketFilters} onChange={setFilter} />
      </Toolbar>
      {tickets.isPending ? (
        <Skeleton lines={5} />
      ) : tickets.isError ? (
        <Banner tone="danger">Não foi possível carregar os chamados.</Banner>
      ) : rows.length === 0 ? (
        <EmptyState icon={<ShieldIcon />}>Nenhum chamado neste estado.</EmptyState>
      ) : (
        <DataTable<StaffTicket>
          caption="Chamados"
          rows={rows}
          rowKey={(t) => t.id}
          columns={[
            { key: "id", header: "Chamado", render: (t) => <Link to={`/chamados/${t.id}`} className="mono">{t.id.slice(0, 8)}</Link> },
            { key: "subject", header: "Assunto" },
            { key: "owner_id", header: "Cliente", render: (t) => <span className="mono">{t.owner_id.slice(0, 8)}</span> },
            {
              key: "order_id",
              header: "Pedido",
              render: (t) => (t.order_id === null ? "—" : <Link to={`/pedidos/${t.order_id}`} className="mono">{t.order_id.slice(0, 8)}</Link>),
            },
            {
              key: "status",
              header: "Estado",
              render: (t) => {
                const v = ticketStatus(t.status, "staff");
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
