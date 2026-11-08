// The order lifecycle as the customer sees it: one table of labels and
// tones, the timeline and the actions each status allows. The API decides
// what is allowed; this only offers what makes sense.
import type { TimelineItem, Tone } from "@nugget/ui";

import type { Order } from "../quote/types";

export type Status = Order["status"];

const view: Record<Status, { label: string; tone: Tone }> = {
  quoted: { label: "Orçado", tone: "neutral" },
  awaiting_review: { label: "Em revisão técnica", tone: "info" },
  awaiting_payment: { label: "Aguardando pagamento", tone: "warning" },
  rejected: { label: "Reprovado", tone: "danger" },
  paid: { label: "Pago", tone: "progress" },
  in_production: { label: "Em produção", tone: "progress" },
  shipped: { label: "Enviado", tone: "info" },
  delivered: { label: "Entregue", tone: "success" },
  cancelled: { label: "Cancelado", tone: "neutral" },
};

export function expired(o: Order, now: Date): boolean {
  return o.status === "quoted" && new Date(o.quote_expires_at).getTime() <= now.getTime();
}

export function statusView(o: Order, now: Date): { label: string; tone: Tone } {
  return expired(o, now) ? { label: "Orçamento vencido", tone: "neutral" } : view[o.status];
}

/** The happy path, in order: the timeline shows what is done and what is
 * still ahead. */
const path: readonly Status[] = ["quoted", "awaiting_review", "awaiting_payment", "paid", "in_production", "shipped", "delivered"];

const when = new Intl.DateTimeFormat("pt-BR", { dateStyle: "short", timeStyle: "short" });

/** timeline joins the real history (with the API's dates) and the steps
 * still ahead. A rejected or cancelled order ends where it stopped. */
export function timeline(o: Order): TimelineItem[] {
  const history = o.history ?? [];
  const at = new Map(history.map((h) => [h.to, when.format(new Date(h.at))]));
  const ended = o.status === "rejected" || o.status === "cancelled";
  const items: TimelineItem[] = [];
  for (const s of path) {
    const done = at.get(s);
    if (done !== undefined) {
      items.push({ key: s, label: view[s].label, state: s === o.status ? "current" : "done", at: done });
    } else if (!ended) {
      items.push({ key: s, label: view[s].label, state: "pending" });
    }
  }
  if (ended) {
    const endedAt = at.get(o.status);
    items.push(endedAt === undefined ? { key: o.status, label: view[o.status].label, state: "current" } : { key: o.status, label: view[o.status].label, state: "current", at: endedAt });
  }
  return items;
}

export interface Actions {
  submit: boolean;
  requote: boolean;
  pay: boolean;
  cancel: boolean;
  finishRefund: boolean;
}

/** actions says what the customer may try from this status (RF08, RF10). */
export function actions(o: Order, now: Date): Actions {
  const payment = o.payment?.status;
  const pending = payment === "pending";
  return {
    submit: o.status === "quoted" && !expired(o, now),
    requote: expired(o, now),
    pay: o.status === "awaiting_payment" && !pending,
    cancel: (["quoted", "awaiting_review", "awaiting_payment", "paid"] as const).some((s) => s === o.status) && !pending,
    finishRefund: o.status === "cancelled" && payment === "refund_pending",
  };
}

/** pollInterval: only while the order moves without the customer. Fast for
 * the gateway's confirmation, every 5 seconds while the staff and the carrier
 * take it to the delivery, slowly while it waits for the technical review,
 * and never once it stopped or waits for the customer. */
export function pollInterval(o: Order | undefined): number | false {
  if (o === undefined) {
    return false;
  }
  if (o.payment?.status === "pending") {
    return 2000;
  }
  switch (o.status) {
    case "paid":
    case "in_production":
    case "shipped":
      return 5000;
    case "awaiting_review":
      return 30_000;
    default:
      return false;
  }
}

export type Filter = "all" | "open" | "done" | "cancelled";

export function matches(o: Order, f: Filter): boolean {
  switch (f) {
    case "all":
      return true;
    case "done":
      return o.status === "delivered";
    case "cancelled":
      return o.status === "cancelled" || o.status === "rejected";
    case "open":
      return o.status !== "delivered" && o.status !== "cancelled" && o.status !== "rejected";
  }
}
