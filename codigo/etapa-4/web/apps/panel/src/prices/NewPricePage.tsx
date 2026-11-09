import { unwrap } from "@nugget/api";
import { finishingLabel, qualityLabel } from "@nugget/domain";
import { Banner, Button, Field, Grid, Panel, Skeleton, Stack, TextInput } from "@nugget/ui";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { Link, useNavigate } from "react-router";

import { useApi } from "../api";
import { usePriceTables } from "./PricesPage";
import { draftFrom, rulesFrom, type Draft, type Rules } from "./rules";

/** NewPricePage proposes a table from the active one: only the values
 * change. Saving creates a draft; prices change only when another admin
 * approves it. */
export function NewPricePage() {
  const tables = usePriceTables();
  const active = tables.data?.price_tables.find((t) => t.status === "active");
  if (tables.isPending) {
    return <Skeleton lines={10} />;
  }
  if (active === undefined) {
    return <Banner tone="danger">Não há tabela ativa para partir.</Banner>;
  }
  return <Editor base={active.rules} version={active.version} />;
}

function Editor({ base, version }: { base: Rules; version: number }) {
  const api = useApi();
  const client = useQueryClient();
  const navigate = useNavigate();
  const [draft, setDraft] = useState<Draft>(() => draftFrom(base));
  const [error, setError] = useState<string | undefined>();
  const propose = useMutation({
    mutationFn: (rules: Rules) => unwrap(api.POST("/panel/v1/price-tables", { body: { rules } })),
    onSuccess: async (res) => {
      await client.invalidateQueries({ queryKey: ["price-tables"] });
      await navigate(`/precos/${String(res.version)}`);
    },
  });
  const set = (patch: Partial<Draft>) => { setDraft({ ...draft, ...patch }); };

  const submit = () => {
    const r = rulesFrom(base, draft);
    if (!r.ok) {
      setError(r.error);
      return;
    }
    setError(undefined);
    propose.mutate(r.rules);
  };

  return (
    <Stack gap={4}>
      <h1>Nova tabela de preço</h1>
      <p>
        Parte da tabela ativa (v{String(version)}). Valores em reais, multiplicadores em ‰ (1000 mantém o preço do material). Criar materiais, cores ou qualidades fica fora do painel.
      </p>
      <Grid columns={2}>
        <Panel>
          <Panel.Header>
            <h2>Materiais (por cm³)</h2>
          </Panel.Header>
          <Panel.Body>
            <Stack gap={3}>
              {Object.entries(base.materials).map(([id, m]) => (
                <Field key={id} label={m.name}>
                  {(c) => <TextInput {...c} inputMode="decimal" value={draft.materials[id] ?? ""} onChange={(e) => { set({ materials: { ...draft.materials, [id]: e.target.value } }); }} />}
                </Field>
              ))}
            </Stack>
          </Panel.Body>
        </Panel>
        <Panel>
          <Panel.Header>
            <h2>Qualidades (‰)</h2>
          </Panel.Header>
          <Panel.Body>
            <Stack gap={3}>
              {Object.keys(base.qualities).map((id) => (
                <Field key={id} label={qualityLabel(id)}>
                  {(c) => <TextInput {...c} inputMode="numeric" value={draft.qualities[id] ?? ""} onChange={(e) => { set({ qualities: { ...draft.qualities, [id]: e.target.value } }); }} />}
                </Field>
              ))}
            </Stack>
          </Panel.Body>
        </Panel>
        <Panel>
          <Panel.Header>
            <h2>Acabamentos (por unidade)</h2>
          </Panel.Header>
          <Panel.Body>
            <Stack gap={3}>
              {Object.keys(base.finishings).map((id) => (
                <Field key={id} label={finishingLabel(id)}>
                  {(c) => <TextInput {...c} inputMode="decimal" value={draft.finishings[id] ?? ""} onChange={(e) => { set({ finishings: { ...draft.finishings, [id]: e.target.value } }); }} />}
                </Field>
              ))}
              <Field label="Taxa de preparo">
                {(c) => <TextInput {...c} inputMode="decimal" value={draft.setup} onChange={(e) => { set({ setup: e.target.value }); }} />}
              </Field>
            </Stack>
          </Panel.Body>
        </Panel>
        <Panel>
          <Panel.Header>
            <h2>Descontos por quantidade</h2>
          </Panel.Header>
          <Panel.Body>
            <Stack gap={3}>
              {draft.discounts.map((t, i) => (
                <Grid key={i} columns={2}>
                  <Field label={`Faixa ${String(i + 1)}: a partir de (un.)`}>
                    {(c) => (
                      <TextInput {...c} inputMode="numeric" value={t.min} onChange={(e) => { set({ discounts: draft.discounts.map((x, j) => (j === i ? { ...x, min: e.target.value } : x)) }); }} />
                    )}
                  </Field>
                  <Field label={`Faixa ${String(i + 1)}: desconto (%)`}>
                    {(c) => (
                      <TextInput {...c} inputMode="numeric" value={t.pct} onChange={(e) => { set({ discounts: draft.discounts.map((x, j) => (j === i ? { ...x, pct: e.target.value } : x)) }); }} />
                    )}
                  </Field>
                </Grid>
              ))}
              <Stack direction="row" gap={2}>
                <Button variant="ghost" onClick={() => { set({ discounts: [...draft.discounts, { min: "", pct: "" }] }); }}>
                  Adicionar faixa
                </Button>
                {draft.discounts.length > 0 && (
                  <Button variant="ghost" onClick={() => { set({ discounts: draft.discounts.slice(0, -1) }); }}>
                    Remover a última
                  </Button>
                )}
              </Stack>
            </Stack>
          </Panel.Body>
        </Panel>
      </Grid>
      {error !== undefined && <Banner tone="danger">{error}</Banner>}
      {propose.isError && <Banner tone="danger">A API recusou a tabela. Confira os valores e tente de novo.</Banner>}
      <Stack direction="row" gap={3} align="center">
        <Button variant="primary" disabled={propose.isPending} onClick={submit}>
          Propor tabela
        </Button>
        <Link to="/precos">Cancelar</Link>
      </Stack>
    </Stack>
  );
}
