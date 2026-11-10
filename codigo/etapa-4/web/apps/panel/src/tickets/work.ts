// What the support may do with a ticket. The panel API decides (409 on a
// closed ticket); this only offers what makes sense.
import type { PanelComponents } from "@nugget/api";

export type StaffTicket = PanelComponents["schemas"]["StaffTicket"];
export type TicketFilter = "" | StaffTicket["status"];

export const ticketFilters: { value: TicketFilter; label: string }[] = [
  { value: "open", label: "Abertos" },
  { value: "answered", label: "Respondidos" },
  { value: "closed", label: "Fechados" },
  { value: "", label: "Todos" },
];

/** stillOpen: an open or answered ticket takes answers and can be closed; a
 * closed one is only read. */
export function stillOpen(t: Pick<StaffTicket, "status">): boolean {
  return t.status !== "closed";
}
