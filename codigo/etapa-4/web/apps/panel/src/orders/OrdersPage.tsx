import { unwrap } from "@nugget/api";
import { colorLabel, materialLabel, statusView } from "@nugget/domain";
import { Banner, ChipGroup, CubeIcon, DataTable, EmptyState, money, Skeleton, Stack, StatusTag, Toolbar } from "@nugget/ui";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { Link } from "react-router";

import { useApi, useMe } from "../api";
import { queueFilters, type QueueFilter, type StaffOrder } from "./work";

const date = new Intl.DateTimeFormat("pt-BR", { dateStyle: "short" });

/** OrdersPage is the queue: every order for the admin and the support,
 * filtered by status on the server; only the assigned ones for a
 * technician. */
export function OrdersPage() {
  const me = useMe();
  if (me.data === undefined) {
    return me.isError ? <Banner tone="danger">Não foi possível conferir o seu papel.</Banner> : <Skeleton lines={6} />;
  }
  return me.data.role === "technician" ? <AssignedOrders /> : <Queue />;
}

function Queue() {
  const api = useApi();
  const [filter, setFilter] = useState<QueueFilter>("");
  const orders = useQuery({
    queryKey: ["orders", filter],
    queryFn: () => unwrap(api.GET("/panel/v1/orders", { params: { query: filter === "" ? {} : { status: filter } } })),
  });
  return (
    <Stack gap={4}>
      <h1>Pedidos</h1>
      <Toolbar>
        <ChipGroup legend="Estado" value={filter} options={queueFilters} onChange={setFilter} />
      </Toolbar>
      <OrderTable caption="Pedidos" query={orders} empty="Nenhum pedido neste estado." />
    </Stack>
  );
}

function AssignedOrders() {
  const api = useApi();
  const orders = useQuery({ queryKey: ["orders", "assigned"], queryFn: () => unwrap(api.GET("/panel/v1/orders/assigned")) });
  return (
    <Stack gap={4}>
      <h1>Meus pedidos</h1>
      <OrderTable caption="Pedidos atribuídos a você" query={orders} empty="Nenhum pedido atribuído a você." />
    </Stack>
  );
}

interface OrderTableProps {
  caption: string;
  query: { isPending: boolean; isError: boolean; data?: { orders: StaffOrder[] } | undefined };
  empty: string;
}

function OrderTable({ caption, query, empty }: OrderTableProps) {
  if (query.isPending) {
    return <Skeleton lines={6} />;
  }
  if (query.isError || query.data === undefined) {
    return <Banner tone="danger">Não foi possível carregar os pedidos.</Banner>;
  }
  if (query.data.orders.length === 0) {
    return <EmptyState icon={<CubeIcon />}>{empty}</EmptyState>;
  }
  const now = new Date();
  return (
    <DataTable<StaffOrder>
      caption={caption}
      rows={query.data.orders}
      rowKey={(o) => o.id}
      empty={empty}
      columns={[
        { key: "id", header: "Pedido", render: (o) => <Link to={`/pedidos/${o.id}`} className="mono">{o.id.slice(0, 8)}</Link> },
        { key: "owner_id", header: "Cliente", render: (o) => <span className="mono">{o.owner_id.slice(0, 8)}</span> },
        { key: "material", header: "Peça", render: (o) => `${materialLabel(o.material)} ${colorLabel(o.color).toLowerCase()} · ${String(o.quantity)} un.` },
        {
          key: "status",
          header: "Estado",
          render: (o) => {
            const v = statusView(o, now);
            return (
              <Stack direction="row" gap={1}>
                <StatusTag tone={v.tone}>{v.label}</StatusTag>
                {o.dfm_flags.includes("exceeds_build_volume") && <StatusTag tone="warning">Excede a impressora</StatusTag>}
              </Stack>
            );
          },
        },
        { key: "assigned_technician", header: "Técnico", render: (o) => (o.assigned_technician === null ? "—" : <span className="mono">{o.assigned_technician.slice(0, 8)}</span>) },
        { key: "total_cents", header: "Total", align: "end", render: (o) => <span className="mono">{money(o.total_cents)}</span> },
        { key: "created_at", header: "Data", render: (o) => date.format(new Date(o.created_at)) },
      ]}
    />
  );
}
