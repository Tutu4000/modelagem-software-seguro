// The /_design page of the development build: every primitive, to review
// the look. The production build never contains it.
import { useState } from "react";

import { Button } from "./Button";
import { DataTable } from "./DataTable";
import { Field, Select, TextInput } from "./Field";
import { Grid } from "./Grid";
import { CubeIcon, UploadIcon } from "./icons";
import { Panel } from "./Panel";
import { Stack } from "./Stack";
import { StatusTag, type Tone } from "./StatusTag";
import { Stepper } from "./Stepper";
import { Timeline } from "./Timeline";
import { Toolbar } from "./Toolbar";

interface Sample {
  id: string;
  part: string;
  material: string;
  total: string;
  status: { tone: Tone; label: string };
}

const samples: Sample[] = [
  { id: "4dd563ab", part: "bracket.stl", material: "PLA preto", total: "R$ 48,30", status: { tone: "info", label: "Em revisão" } },
  { id: "9a53ed11", part: "gear.stl", material: "PETG branco", total: "R$ 112,90", status: { tone: "progress", label: "Em produção" } },
  { id: "db1bb6c4", part: "cover.stl", material: "PLA cinza", total: "R$ 76,00", status: { tone: "success", label: "Entregue" } },
  { id: "101c13a5", part: "membrane.stl", material: "PLA preto", total: "R$ 31,10", status: { tone: "danger", label: "Reprovado" } },
];

export function DesignGallery() {
  const [quantity, setQuantity] = useState(2);
  return (
    <Stack gap={6}>
      <Stack gap={2}>
        <h1>Componentes</h1>
        <p>Primitivas do design system, só no build de desenvolvimento.</p>
      </Stack>

      <Panel>
        <Panel.Header>
          <h2>Botões e tags</h2>
        </Panel.Header>
        <Panel.Body>
          <Stack gap={4}>
            <Stack direction="row" gap={2}>
              <Button variant="primary">
                <UploadIcon /> Enviar STL
              </Button>
              <Button>Secundário</Button>
              <Button variant="ghost">Ghost</Button>
              <Button variant="primary" disabled>
                Desabilitado
              </Button>
            </Stack>
            <Stack direction="row" gap={2}>
              {(["neutral", "info", "progress", "success", "warning", "danger"] as const).map((t) => (
                <StatusTag key={t} tone={t}>
                  {t}
                </StatusTag>
              ))}
            </Stack>
          </Stack>
        </Panel.Body>
      </Panel>

      <Grid columns="sidebar">
        <Panel accent>
          <Panel.Header>
            <h2>Configurar peça</h2>
            <CubeIcon />
          </Panel.Header>
          <Panel.Body>
            <Grid columns={2}>
              <Field label="Material" hint="Preço por cm³ na tabela ativa">
                {(c) => (
                  <Select {...c} defaultValue="pla">
                    <option value="pla">PLA</option>
                    <option value="petg">PETG</option>
                  </Select>
                )}
              </Field>
              <Field label="Quantidade">{(c) => <Stepper {...c} value={quantity} min={1} max={1000} onChange={setQuantity} />}</Field>
              <Field label="CEP" error="Use o formato 01310-100">
                {(c) => <TextInput {...c} defaultValue="0131" inputMode="numeric" />}
              </Field>
            </Grid>
          </Panel.Body>
          <Panel.Footer>
            <Button>Cancelar</Button>
            <Button variant="primary">Orçar</Button>
          </Panel.Footer>
        </Panel>
        <Panel>
          <Panel.Header>
            <h2>Andamento</h2>
          </Panel.Header>
          <Panel.Body>
            <Timeline
              label="Andamento do pedido"
              items={[
                { key: "q", label: "Orçado", state: "done", at: "08/11 14:02" },
                { key: "r", label: "Revisão técnica", state: "done", at: "08/11 14:10" },
                { key: "p", label: "Em produção", state: "current", at: "08/11 15:31" },
                { key: "s", label: "Enviado", state: "pending" },
              ]}
            />
          </Panel.Body>
        </Panel>
      </Grid>

      <Stack gap={2}>
        <Toolbar>
          <h2>Pedidos</h2>
          <Toolbar.Spacer />
          <Button>Filtrar</Button>
          <Button variant="primary">Novo pedido</Button>
        </Toolbar>
        <DataTable
          caption="Pedidos de exemplo"
          rows={samples}
          rowKey={(r) => r.id}
          columns={[
            { key: "id", header: "Pedido", render: (r) => <code>{r.id}</code> },
            { key: "part", header: "Peça" },
            { key: "material", header: "Material" },
            { key: "status", header: "Estado", render: (r) => <StatusTag tone={r.status.tone}>{r.status.label}</StatusTag> },
            { key: "total", header: "Total", align: "end", render: (r) => <span className="mono">{r.total}</span> },
          ]}
        />
      </Stack>
    </Stack>
  );
}
