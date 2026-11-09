import { ApiError, unwrap } from "@nugget/api";
import { colorLabel, finishingLabel, materialLabel, qualityLabel, statusView, timeline } from "@nugget/domain";
import { Banner, Button, Grid, KeyValue, money, Panel, PriceSummary, Skeleton, Spinner, Stack, StatusTag, Timeline } from "@nugget/ui";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { Link, useParams } from "react-router";

import { useApi } from "../api";
import { lines } from "../quote/QuotePanel";
import { AddressChangePanel } from "./AddressChangePanel";
import { CancelDialog } from "./CancelDialog";
import { actions, pollInterval } from "./status";

const when = new Intl.DateTimeFormat("pt-BR", { dateStyle: "short", timeStyle: "short" });

const paymentLabel = {
  pending: "Aguardando a confirmação do gateway",
  succeeded: "Pago",
  refund_pending: "Estorno em andamento",
  refunded: "Estornado",
} as const;

/** OrderPage follows one order (RF12, RF20): the timeline, the review report,
 * the payment, the tracking and what the customer can do now. */
export function OrderPage() {
  const { id = "" } = useParams();
  const api = useApi();
  const client = useQueryClient();
  const [cancelling, setCancelling] = useState(false);
  const order = useQuery({
    queryKey: ["order", id],
    queryFn: () => unwrap(api.GET("/api/v1/orders/{id}", { params: { path: { id } } })),
    refetchInterval: (q) => pollInterval(q.state.data),
  });
  const refresh = async () => {
    await client.invalidateQueries({ queryKey: ["order", id] });
    await client.invalidateQueries({ queryKey: ["orders"] });
  };
  const pay = useMutation({ mutationFn: () => unwrap(api.POST("/api/v1/orders/{id}/payment", { params: { path: { id } } })), onSuccess: refresh });
  const submit = useMutation({ mutationFn: () => unwrap(api.POST("/api/v1/orders/{id}/submit", { params: { path: { id } } })), onSuccess: refresh });
  const refund = useMutation({
    mutationFn: () => unwrap(api.POST("/api/v1/orders/{id}/cancel", { params: { path: { id } }, body: {} })),
    onSuccess: refresh,
  });

  if (order.isPending) {
    return <Skeleton lines={10} />;
  }
  if (order.isError) {
    const missing = order.error instanceof ApiError && (order.error.status === 403 || order.error.status === 404);
    return (
      <Panel>
        <Panel.Body>
          <Stack gap={3}>
            <h1>{missing ? "Pedido não encontrado" : "Não foi possível carregar o pedido"}</h1>
            <Link to="/pedidos">Voltar aos pedidos</Link>
          </Stack>
        </Panel.Body>
      </Panel>
    );
  }

  const o = order.data;
  const now = new Date();
  const v = statusView(o, now);
  const can = actions(o, now);
  const shipping = o.shipping;

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
          {o.review !== undefined && o.review !== null && (
            <Panel>
              <Panel.Header>
                <h2>Laudo da revisão técnica</h2>
                <StatusTag tone={o.review.decision === "approved" ? "success" : "danger"}>{o.review.decision === "approved" ? "Aprovado" : "Reprovado"}</StatusTag>
              </Panel.Header>
              <Panel.Body>
                <p data-testid="review-report">{o.review.report}</p>
              </Panel.Body>
            </Panel>
          )}
          {o.dfm_flags.includes("exceeds_build_volume") && (
            <Banner tone="warning">A peça é maior que a área de impressão (250 mm por eixo).</Banner>
          )}
          <Grid columns={2}>
            <Panel>
              <Panel.Header>
                <h2>Peça</h2>
              </Panel.Header>
              <Panel.Body>
                <KeyValue
                  pairs={[
                    { label: "Material e cor", value: `${materialLabel(o.material)} · ${colorLabel(o.color)}` },
                    { label: "Qualidade e acabamento", value: `${qualityLabel(o.quality)} · ${finishingLabel(o.finishing)}` },
                    { label: "Quantidade", value: String(o.quantity), mono: true },
                  ]}
                />
              </Panel.Body>
            </Panel>
            <Panel>
              <Panel.Header>
                <h2>Entrega</h2>
              </Panel.Header>
              <Panel.Body>
                <KeyValue
                  pairs={[
                    {
                      label: "Endereço",
                      testId: "shipping-address",
                      value:
                        shipping === undefined ? "Removido pela política de retenção" : `${shipping.recipient}, ${shipping.street}, ${shipping.number}, ${shipping.city} - ${shipping.state}, ${shipping.postal_code}`,
                    },
                    ...(o.tracking_code === undefined ? [] : [{ label: "Rastreio", value: o.tracking_code, mono: true, testId: "tracking" }]),
                  ]}
                />
              </Panel.Body>
            </Panel>
          </Grid>
        </Stack>

        <Stack gap={4}>
          <Panel accent>
            <Panel.Header>
              <h2>Valores</h2>
            </Panel.Header>
            <Panel.Body>
              <Stack gap={3}>
                <PriceSummary lines={lines(o)} totalCents={o.total_cents} />
                {o.payment !== undefined && (
                  <KeyValue pairs={[{ label: "Pagamento", value: paymentLabel[o.payment.status], testId: "payment-status" }]} />
                )}
                {o.payment?.status === "pending" && <Spinner label="Aguardando a confirmação do gateway…" />}
              </Stack>
            </Panel.Body>
            {(can.pay || can.submit || can.requote || can.cancel || can.finishRefund) && (
              <Panel.Footer>
                {can.cancel && (
                  <Button variant="ghost" onClick={() => { setCancelling(true); }}>
                    Cancelar pedido
                  </Button>
                )}
                {can.submit && (
                  <Button variant="primary" disabled={submit.isPending} onClick={() => { submit.mutate(); }}>
                    Enviar para revisão
                  </Button>
                )}
                {can.requote && <Link to={`/orcar?arquivo=${o.file_id}`}>Orçar de novo</Link>}
                {can.pay && (
                  <Button variant="primary" disabled={pay.isPending} onClick={() => { pay.mutate(); }}>
                    Pagar {money(o.total_cents)}
                  </Button>
                )}
                {can.finishRefund && (
                  <Button variant="primary" disabled={refund.isPending} onClick={() => { refund.mutate(); }}>
                    Concluir estorno
                  </Button>
                )}
              </Panel.Footer>
            )}
          </Panel>
          <AddressChangePanel orderId={o.id} />
          <Link to={`/suporte/novo?pedido=${o.id}`}>Abrir chamado sobre este pedido</Link>
        </Stack>
      </Grid>
      <CancelDialog orderId={o.id} open={cancelling} onClose={() => { setCancelling(false); }} paid={o.status === "paid"} />
    </Stack>
  );
}
