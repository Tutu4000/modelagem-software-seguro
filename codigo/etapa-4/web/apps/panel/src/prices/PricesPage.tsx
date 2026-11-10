import { unwrap } from "@nugget/api";
import { finishingLabel, qualityLabel } from "@nugget/domain";
import { Banner, DataTable, KeyValue, money, Panel, Skeleton, Stack, StatusTag, Toolbar } from "@nugget/ui";
import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router";

import { useApi } from "../api";
import { tableStatus, type PriceTable, type Rules } from "./rules";

const when = new Intl.DateTimeFormat("pt-BR", { dateStyle: "short", timeStyle: "short" });

/** usePriceTables is every version, newest first. */
export function usePriceTables() {
  const api = useApi();
  return useQuery({ queryKey: ["price-tables"], queryFn: () => unwrap(api.GET("/panel/v1/price-tables")) });
}

/** PricesPage lists the versions and shows the active table. A change is a
 * draft until another admin approves it (D02). */
export function PricesPage() {
  const tables = usePriceTables();
  if (tables.isPending) {
    return <Skeleton lines={8} />;
  }
  if (tables.isError) {
    return <Banner tone="danger">Não foi possível carregar as tabelas de preço.</Banner>;
  }
  const list = tables.data.price_tables;
  const active = list.find((t) => t.status === "active");
  return (
    <Stack gap={4}>
      <Toolbar>
        <h1>Tabelas de preço</h1>
        <Toolbar.Spacer />
        {active !== undefined && <Link to="/precos/nova">Propor nova tabela</Link>}
      </Toolbar>
      <DataTable<PriceTable>
        caption="Versões"
        rows={list}
        rowKey={(t) => String(t.version)}
        columns={[
          { key: "version", header: "Versão", render: (t) => <Link to={`/precos/${String(t.version)}`} className="mono">v{String(t.version)}</Link> },
          {
            key: "status",
            header: "Estado",
            render: (t) => <StatusTag tone={tableStatus[t.status].tone}>{tableStatus[t.status].label}</StatusTag>,
          },
          { key: "created_by", header: "Proposta por", render: (t) => <span className="mono">{t.created_by?.slice(0, 8) ?? "sistema"}</span> },
          { key: "approved_by", header: "Aprovada por", render: (t) => <span className="mono">{t.approved_by?.slice(0, 8) ?? "—"}</span> },
          { key: "created_at", header: "Criada", render: (t) => when.format(new Date(t.created_at)) },
        ]}
      />
      {active !== undefined && (
        <Panel>
          <Panel.Header>
            <h2>Tabela ativa · v{String(active.version)}</h2>
          </Panel.Header>
          <Panel.Body>
            <RulesView rules={active.rules} />
          </Panel.Body>
        </Panel>
      )}
    </Stack>
  );
}

export function RulesView({ rules }: { rules: Rules }) {
  return (
    <KeyValue
      pairs={[
        ...Object.values(rules.materials).map((m) => ({ label: `${m.name} por cm³`, value: money(m.cents_per_cm3), mono: true })),
        ...Object.entries(rules.qualities).map(([id, q]) => ({ label: `Qualidade ${qualityLabel(id)}`, value: `${String(q)} ‰`, mono: true })),
        ...Object.entries(rules.finishings).map(([id, f]) => ({ label: `Acabamento ${finishingLabel(id)}`, value: money(f), mono: true })),
        { label: "Taxa de preparo", value: money(rules.setup_cents), mono: true },
        {
          label: "Descontos",
          value: rules.discounts.length === 0 ? "Nenhum" : rules.discounts.map((t) => `${String(t.min_quantity)} un.: ${String(t.percent_off)}%`).join(" · "),
        },
      ]}
    />
  );
}
