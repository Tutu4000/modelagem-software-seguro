// What each staff role may try on an order. The panel API decides for real
// (403, 409); this only offers the buttons that make sense.
import { ApiError, type PanelComponents } from "@nugget/api";
import type { Status } from "@nugget/domain";

import { codePoints } from "../CriticalActionDialog";
import type { StaffRole } from "../roles";

export type StaffOrder = PanelComponents["schemas"]["StaffOrder"];
export type Technician = PanelComponents["schemas"]["Technician"];

export interface Work {
  assign: boolean;
  openFile: boolean;
  review: boolean;
  startProduction: boolean;
  ship: boolean;
}

/** work: the admin assigns an order awaiting review; only the technician it
 * was assigned to opens the design, reviews it, prints it and books the
 * pickup. */
export function work(role: StaffRole, me: string, o: StaffOrder): Work {
  const mine = role === "technician" && o.assigned_technician === me;
  return {
    assign: role === "admin" && o.status === "awaiting_review",
    openFile: mine && o.status === "awaiting_review",
    review: mine && o.status === "awaiting_review" && (o.review === undefined || o.review === null),
    startProduction: mine && o.status === "paid",
    ship: mine && o.status === "in_production" && o.tracking_code === undefined,
  };
}

export const minReport = 10;
export const maxReport = 5000;

/** reportError counts like the API (utf8.RuneCountInString). */
export function reportError(text: string): string | undefined {
  const n = codePoints(text.trim());
  if (n < minReport) {
    return `Escreva pelo menos ${String(minReport)} caracteres.`;
  }
  if (n > maxReport) {
    return `Use no máximo ${String(maxReport)} caracteres.`;
  }
  return undefined;
}

/** technicianOptions sorts the technicians by name for the select. */
export function technicianOptions(list: readonly Technician[]): { value: string; label: string }[] {
  return [...list].sort((a, b) => a.username.localeCompare(b.username, "pt-BR")).map((t) => ({ value: t.id, label: t.username }));
}

/** The statuses the admin and the support filter the queue by; the empty
 * value is every order. */
export type QueueFilter = "" | Extract<Status, "awaiting_review" | "awaiting_payment" | "paid" | "in_production" | "shipped">;

export const queueFilters: { value: QueueFilter; label: string }[] = [
  { value: "", label: "Todos" },
  { value: "awaiting_review", label: "Em revisão" },
  { value: "awaiting_payment", label: "Aguardando pagamento" },
  { value: "paid", label: "Pagos" },
  { value: "in_production", label: "Em produção" },
  { value: "shipped", label: "Enviados" },
];

/** orderProblem explains a refused operation. A 403 is both "someone else's
 * order" and "no such order": the API does not tell them apart. */
export function orderProblem(e: unknown): string {
  if (e instanceof ApiError) {
    switch (e.status) {
      case 403:
        return "Pedido não encontrado ou não atribuído a você.";
      case 409:
        return "O pedido mudou de estado enquanto você trabalhava. Recarregue para ver o estado atual.";
      case 410:
        return "O projeto já foi apagado pela política de retenção.";
      case 502:
        return "Um parceiro não respondeu. Nada mudou; tente de novo em instantes.";
    }
  }
  return "A operação não foi concluída. Tente de novo.";
}
