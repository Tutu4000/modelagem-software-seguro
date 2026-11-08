import { unwrap } from "@nugget/api";
import { Banner, ChipGroup, CubeIcon, DataTable, EmptyState, money, Skeleton, Stack, StatusTag, Toolbar } from "@nugget/ui";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { Link } from "react-router";

import { useApi } from "../api";
import { colorLabel, materialLabel } from "../quote/labels";
import type { Order } from "../quote/types";
import { matches, statusView, type Filter } from "./status";

const date = new Intl.DateTimeFormat("pt-BR", { dateStyle: "short" });

const filters: { value: Filter; label: string }[] = [
  { value: "all", label: "Todos" },
  { value: "open", label: "Em andamento" },
  { value: "done", label: "Concluídos" },
  { value: "cancelled", label: "Cancelados" },
];

/** OrdersPage is the customer's order history (RF09). */
export function OrdersPage() {
  const api = useApi();
  const [filter, setFilter] = useState<Filter>("all");
  const orders = useQuery({ queryKey: ["orders"], queryFn: () => unwrap(api.GET("/api/v1/orders")) });
  const now = new Date();
  const rows = (orders.data?.orders ?? []).filter((o) => matches(o, filter));

  return (
    <Stack gap={4}>
      <h1>Meus pedidos</h1>
      <Toolbar>
        <ChipGroup legend="Mostrar" value={filter} options={filters} onChange={setFilter} />
        <Toolbar.Spacer />
        <Link to="/orcar">Novo orçamento</Link>
      </Toolbar>
      {orders.isPending ? (
        <Skeleton lines={6} />
      ) : orders.isError ? (
        <Banner tone="danger">Não foi possível carregar os pedidos.</Banner>
      ) : rows.length === 0 && filter === "all" ? (
        <EmptyState icon={<CubeIcon />} action={<Link to="/orcar">Orçar uma peça</Link>}>
          Você ainda não tem pedidos.
        </EmptyState>
      ) : (
        <DataTable<Order>
          caption="Meus pedidos"
          rows={rows}
          rowKey={(o) => o.id}
          empty="Nenhum pedido neste filtro."
          columns={[
            { key: "id", header: "Pedido", render: (o) => <Link to={`/pedidos/${o.id}`} className="mono">{o.id.slice(0, 8)}</Link> },
            { key: "material", header: "Peça", render: (o) => `${materialLabel(o.material)} ${colorLabel(o.color).toLowerCase()} · ${String(o.quantity)} un.` },
            {
              key: "status",
              header: "Estado",
              render: (o) => {
                const v = statusView(o, now);
                return <StatusTag tone={v.tone}>{v.label}</StatusTag>;
              },
            },
            { key: "total_cents", header: "Total", align: "end", render: (o) => <span className="mono">{money(o.total_cents)}</span> },
            { key: "created_at", header: "Data", render: (o) => date.format(new Date(o.created_at)) },
          ]}
        />
      )}
    </Stack>
  );
}
