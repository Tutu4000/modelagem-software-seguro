// The order lifecycle as both apps show it: one table of labels and tones,
// and the timeline. Each app decides which actions it offers; the API decides
// what is allowed.
import type { CustomerComponents } from "@nugget/api";
import type { TimelineItem, Tone } from "@nugget/ui";

type Order = CustomerComponents["schemas"]["Order"];

export type Status = Order["status"];

/** Lifecycle is the part of an order these functions read: the customer's
 * order and the panel's staff order both have it. */
export type Lifecycle = Pick<Order, "status" | "quote_expires_at" | "history">;

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

export function statusLabel(s: Status): string {
  return view[s].label;
}

export function expired(o: Lifecycle, now: Date): boolean {
  return o.status === "quoted" && new Date(o.quote_expires_at).getTime() <= now.getTime();
}

export function statusView(o: Lifecycle, now: Date): { label: string; tone: Tone } {
  return expired(o, now) ? { label: "Orçamento vencido", tone: "neutral" } : view[o.status];
}

/** The happy path, in order: the timeline shows what is done and what is
 * still ahead. */
const path: readonly Status[] = ["quoted", "awaiting_review", "awaiting_payment", "paid", "in_production", "shipped", "delivered"];

const when = new Intl.DateTimeFormat("pt-BR", { dateStyle: "short", timeStyle: "short" });

/** timeline joins the real history (with the API's dates) and the steps
 * still ahead. A rejected or cancelled order ends where it stopped. */
export function timeline(o: Lifecycle): TimelineItem[] {
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

export type Filter = "all" | "open" | "done" | "cancelled";

export function matches(o: Lifecycle, f: Filter): boolean {
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
