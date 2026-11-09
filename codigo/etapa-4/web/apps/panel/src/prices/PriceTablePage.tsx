import { unwrap } from "@nugget/api";
import { Banner, Button, DataTable, Panel, Skeleton, Stack, StatusTag } from "@nugget/ui";
import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { Link, useParams } from "react-router";

import { useApi, useMe } from "../api";
import { CriticalActionDialog } from "../CriticalActionDialog";
import { RulesView, usePriceTables } from "./PricesPage";
import { priceDiff, tableStatus, type Change } from "./rules";

/** PriceTablePage shows one version. A draft shows what it changes against
 * the active table, and another admin approves it as a critical operation:
 * whoever proposed it cannot (D02). */
export function PriceTablePage() {
  const { version = "" } = useParams();
  const api = useApi();
  const me = useMe();
  const client = useQueryClient();
  const tables = usePriceTables();
  const [asking, setAsking] = useState(false);

  if (tables.isPending || me.isPending) {
    return <Skeleton lines={8} />;
  }
  const table = tables.data?.price_tables.find((t) => String(t.version) === version);
  if (table === undefined || me.data === undefined) {
    return (
      <Panel>
        <Panel.Body>
          <Stack gap={3}>
            <h1>Tabela não encontrada</h1>
            <Link to="/precos">Voltar às tabelas</Link>
          </Stack>
        </Panel.Body>
      </Panel>
    );
  }
  const active = tables.data?.price_tables.find((t) => t.status === "active");
  const changes = table.status === "draft" && active !== undefined ? priceDiff(active.rules, table.rules) : [];
  const own = table.created_by === me.data.id;
  const status = tableStatus[table.status];

  const approve = async (justification: string) => {
    try {
      await unwrap(api.POST("/panel/v1/price-tables/{version}/approve", { params: { path: { version: table.version } }, body: { justification } }));
    } finally {
      await client.invalidateQueries({ queryKey: ["price-tables"] });
    }
  };

  return (
    <Stack gap={4}>
      <Stack direction="row" gap={3} align="center" justify="between">
        <h1>Tabela v{String(table.version)}</h1>
        <span data-testid="table-status">
          <StatusTag tone={status.tone}>{status.label}</StatusTag>
        </span>
      </Stack>
      {table.status === "draft" && (
        <Panel accent>
          <Panel.Header>
            <h2>O que muda em relação à v{String(active?.version ?? 0)}</h2>
          </Panel.Header>
          <Panel.Body>
            {changes.length === 0 ? (
              <p>Nenhum valor muda.</p>
            ) : (
              <DataTable<Change>
                caption="Mudanças"
                rows={changes}
                rowKey={(c) => c.label}
                columns={[
                  { key: "label", header: "Item" },
                  { key: "before", header: "Antes", render: (c) => <span className="mono">{c.before}</span> },
                  { key: "after", header: "Depois", render: (c) => <strong className="mono">{c.after}</strong> },
                ]}
              />
            )}
          </Panel.Body>
          <Panel.Footer>
            {own ? (
              <span data-testid="own-draft">Aguardando outro admin: quem propõe não aprova.</span>
            ) : (
              <Button variant="primary" onClick={() => { setAsking(true); }}>
                Aprovar
              </Button>
            )}
          </Panel.Footer>
        </Panel>
      )}
      <Panel>
        <Panel.Header>
          <h2>Valores</h2>
        </Panel.Header>
        <Panel.Body>
          <RulesView rules={table.rules} />
        </Panel.Body>
      </Panel>
      {table.status === "draft" && active === undefined && <Banner tone="warning">Não há tabela ativa para comparar.</Banner>}
      <Link to="/precos">Voltar às tabelas</Link>
      <CriticalActionDialog
        open={asking}
        onClose={() => { setAsking(false); }}
        title={`Aprovar a tabela v${String(table.version)}`}
        confirmLabel="Aprovar e ativar"
        onConfirm={approve}
      >
        <p>A tabela passa a valer para os orçamentos novos na hora. Os pedidos existentes mantêm o preço com que foram orçados.</p>
      </CriticalActionDialog>
    </Stack>
  );
}

