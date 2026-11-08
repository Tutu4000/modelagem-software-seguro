import { Banner, Button, Panel, PriceSummary, Stack, type PriceLine } from "@nugget/ui";

import type { Order } from "./types";

const until = new Intl.DateTimeFormat("pt-BR", { dateStyle: "short", timeStyle: "short" });

/** lines splits the server's total into what the customer pays for. The
 * setup fee is what is left after the parts, the discount and the shipping,
 * so the lines always add up to the server's total. */
export function lines(o: Order): PriceLine[] {
  const parts = o.unit_cents * o.quantity;
  const setup = o.total_cents - parts + o.discount_cents - o.shipping_cents;
  const out: PriceLine[] = [{ label: `Peças (${String(o.quantity)} × unidade)`, cents: parts }];
  if (o.discount_cents > 0) {
    out.push({ label: "Desconto por volume", cents: o.discount_cents, discount: true });
  }
  if (setup > 0) {
    out.push({ label: "Preparação", cents: setup });
  }
  out.push({ label: "Frete", cents: o.shipping_cents });
  return out;
}

export interface QuotePanelProps {
  order: Order | undefined;
  stale: boolean;
  canQuote: boolean;
  busy: boolean;
  problem: string | undefined;
  onQuote: () => void;
  onSubmit: () => void;
}

/** QuotePanel shows the server's quote and the two actions: compute it and
 * send the order to the technical review. */
export function QuotePanel({ order, stale, canQuote, busy, problem, onQuote, onSubmit }: QuotePanelProps) {
  const current = order !== undefined && !stale;
  return (
    <Panel accent>
      <Panel.Header>
        <h2>Orçamento</h2>
      </Panel.Header>
      <Panel.Body>
        <Stack gap={3}>
          {current ? (
            <>
              <PriceSummary lines={lines(order)} totalCents={order.total_cents} />
              <small>Válido até {until.format(new Date(order.quote_expires_at))}. Preço calculado no servidor, pela tabela {String(order.price_table_version)}.</small>
              {order.dfm_flags.includes("exceeds_build_volume") && (
                <Banner tone="warning">A peça é maior que a área de impressão (250 mm por eixo). A revisão técnica vai avaliar se dá para dividir a peça.</Banner>
              )}
            </>
          ) : (
            <p>{stale ? "As opções mudaram. Calcule de novo." : "Envie o arquivo e configure a peça para calcular."}</p>
          )}
          {problem !== undefined && <Banner tone="danger">{problem}</Banner>}
        </Stack>
      </Panel.Body>
      <Panel.Footer>
        {current ? (
          <Button variant="primary" block disabled={busy} onClick={onSubmit}>
            Enviar para revisão técnica
          </Button>
        ) : (
          <Button variant="primary" block disabled={!canQuote || busy} onClick={onQuote}>
            Calcular orçamento
          </Button>
        )}
      </Panel.Footer>
    </Panel>
  );
}
