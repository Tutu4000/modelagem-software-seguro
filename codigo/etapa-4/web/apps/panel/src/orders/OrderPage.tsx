import { unwrap } from "@nugget/api";
import { colorLabel, finishingLabel, materialLabel, qualityLabel, statusView, timeline } from "@nugget/domain";
import { Banner, Button, Grid, KeyValue, money, Panel, Skeleton, Stack, StatusTag, Timeline } from "@nugget/ui";
import { dimensions } from "@nugget/viewer";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useParams, useSearchParams } from "react-router";

import { useApi, useMe } from "../api";
import type { StaffRole } from "../roles";
import { AddressChangePanel } from "./AddressChangePanel";
import { AssignPanel } from "./AssignPanel";
import { DesignPanel } from "./DesignPanel";
import { ReviewForm } from "./ReviewForm";
import { orderProblem, work, type StaffOrder } from "./work";

const when = new Intl.DateTimeFormat("pt-BR", { dateStyle: "short", timeStyle: "short" });

/** OrderPage is one order for the staff (RF19): the timeline, the part with
 * its DFM data, the review and what this role can do now. */
export function OrderPage() {
  const { id = "" } = useParams();
  const api = useApi();
  const me = useMe();
  const [search] = useSearchParams();
  const ticketId = search.get("chamado") ?? undefined;
  const order = useQuery({
    queryKey: ["order", id],
    queryFn: () => unwrap(api.GET("/panel/v1/orders/{id}", { params: { path: { id } } })),
  });

  if (order.isPending || me.isPending) {
    return <Skeleton lines={10} />;
  }
  if (order.isError || me.isError) {
    return (
      <Panel>
        <Panel.Body>
          <Stack gap={3}>
            <h1>{orderProblem(order.error ?? me.error)}</h1>
            <Link to="/pedidos">Voltar aos pedidos</Link>
          </Stack>
        </Panel.Body>
      </Panel>
    );
  }

  const o = order.data;
  const can = work(me.data.role, me.data.id, o);
  const v = statusView(o, new Date());

  return (
    <Stack gap={4}>
      <Stack direction="row" gap={3} align="center" justify="between">
        <Stack gap={1}>
          <h1>
            Pedido <span className="mono">{o.id.slice(0, 8)}</span>
          </h1>
          <small>Criado em {when.format(new Date(o.created_at))}</small>
        </Stack>
        <span data-testid="order-status">
          <StatusTag tone={v.tone}>{v.label}</StatusTag>
        </span>
      </Stack>
      <Grid columns="sidebar">
        <Stack gap={4}>
          <Panel>
            <Panel.Header>
              <h2>Andamento</h2>
            </Panel.Header>
            <Panel.Body>
              <Timeline label="Andamento do pedido" items={timeline(o)} />
            </Panel.Body>
          </Panel>
          {o.dfm_flags.includes("exceeds_build_volume") && (
            <Banner tone="warning">A peça é maior que a área de impressão (250 mm por eixo).</Banner>
          )}
          <Panel>
            <Panel.Header>
              <h2>Peça</h2>
            </Panel.Header>
            <Panel.Body>
              <KeyValue
                pairs={[
                  { label: "Medidas", value: dimensions([o.bbox_mm[0] ?? 0, o.bbox_mm[1] ?? 0, o.bbox_mm[2] ?? 0]), mono: true, testId: "order-dimensions" },
                  { label: "Volume", value: `${(o.volume_mm3 / 1000).toLocaleString("pt-BR", { maximumFractionDigits: 2 })} cm³`, mono: true },
                  { label: "Material e cor", value: `${materialLabel(o.material)} · ${colorLabel(o.color)}` },
                  { label: "Qualidade e acabamento", value: `${qualityLabel(o.quality)} · ${finishingLabel(o.finishing)}` },
                  { label: "Quantidade", value: String(o.quantity), mono: true },
                ]}
              />
            </Panel.Body>
          </Panel>
          {can.openFile && <DesignPanel orderId={o.id} />}
          {o.review !== undefined && o.review !== null ? (
            <Panel>
              <Panel.Header>
                <h2>Laudo da revisão técnica</h2>
                <StatusTag tone={o.review.decision === "approved" ? "success" : "danger"}>{o.review.decision === "approved" ? "Aprovado" : "Reprovado"}</StatusTag>
              </Panel.Header>
              <Panel.Body>
                <Stack gap={2}>
                  <p data-testid="review-report">{o.review.report}</p>
                  <small>Registrado em {when.format(new Date(o.review.reviewed_at))}</small>
                </Stack>
              </Panel.Body>
            </Panel>
          ) : (
            can.review && <ReviewForm orderId={o.id} />
          )}
        </Stack>

        <Stack gap={4}>
          <Panel>
            <Panel.Header>
              <h2>Cliente e entrega</h2>
            </Panel.Header>
            <Panel.Body>
              <KeyValue
                pairs={[
                  { label: "Cliente", value: o.owner_id, mono: true },
                  { label: "Técnico", value: o.assigned_technician === null ? "Nenhum" : o.assigned_technician === me.data.id ? "Você" : o.assigned_technician, mono: o.assigned_technician !== null && o.assigned_technician !== me.data.id },
                  { label: "Endereço", testId: "shipping-address", value: address(o, me.data.role) },
                  ...(o.tracking_code === undefined ? [] : [{ label: "Rastreio", value: o.tracking_code, mono: true, testId: "tracking" }]),
                  { label: "Total", value: money(o.total_cents), mono: true },
                ]}
              />
            </Panel.Body>
          </Panel>
          {can.assign && <AssignPanel order={o} />}
          {(can.startProduction || can.ship) && <ProductionPanel order={o} start={can.startProduction} ship={can.ship} />}
          {can.changeAddress && <AddressChangePanel orderId={o.id} ticketId={ticketId} />}
          {me.data.role === "admin" && <Link to={`/revogar?usuario=${o.owner_id}`}>Revogar acesso do cliente</Link>}
        </Stack>
      </Grid>
    </Stack>
  );
}

/** The technician never gets the address (LGPD: data minimization); the API
 * leaves it out, and the page says why. For the others, a missing address was
 * cleared by the retention. */
function address(o: StaffOrder, role: StaffRole): string {
  const s = o.shipping;
  if (s === undefined) {
    return role === "technician" ? "Endereço visível só para suporte e administração" : "Removido pela política de retenção";
  }
  return `${s.recipient}, ${s.street}, ${s.number}, ${s.city} - ${s.state}, ${s.postal_code}`;
}

interface ProductionPanelProps {
  order: StaffOrder;
  start: boolean;
  ship: boolean;
}

/** ProductionPanel starts the printing and books the pickup. The carrier
 * gets the address from the panel API; the technician never sees it. */
function ProductionPanel({ order, start, ship }: ProductionPanelProps) {
  const api = useApi();
  const client = useQueryClient();
  const path = { params: { path: { id: order.id } } };
  const refresh = async () => {
    await client.invalidateQueries({ queryKey: ["order", order.id] });
    await client.invalidateQueries({ queryKey: ["orders"] });
  };
  const production = useMutation({ mutationFn: () => unwrap(api.POST("/panel/v1/orders/{id}/production", path)), onSuccess: refresh });
  const pickup = useMutation({ mutationFn: () => unwrap(api.POST("/panel/v1/orders/{id}/ship", path)), onSuccess: refresh });
  const failed = production.error ?? pickup.error;
  return (
    <Panel accent>
      <Panel.Header>
        <h2>Produção</h2>
      </Panel.Header>
      <Panel.Body>
        <Stack gap={3}>
          <p>{start ? "O pagamento foi confirmado. Inicie a impressão quando a peça entrar na fila da impressora." : "Com a peça pronta, agende a coleta. A transportadora recebe o endereço direto do painel."}</p>
          {failed !== null && <Banner tone="danger">{orderProblem(failed)}</Banner>}
        </Stack>
      </Panel.Body>
      <Panel.Footer>
        {start && (
          <Button variant="primary" disabled={production.isPending} onClick={() => { production.mutate(); }}>
            Iniciar produção
          </Button>
        )}
        {ship && (
          <Button variant="primary" disabled={pickup.isPending} onClick={() => { pickup.mutate(); }}>
            Agendar coleta
          </Button>
        )}
      </Panel.Footer>
    </Panel>
  );
}
