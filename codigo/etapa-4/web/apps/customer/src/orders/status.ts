// What the customer may do with an order and how often the page asks for
// news. The labels and the timeline are in @nugget/domain, shared with the
// panel. The API decides what is allowed; this only offers what makes sense.
import { expired } from "@nugget/domain";

import type { Order } from "../quote/types";

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
