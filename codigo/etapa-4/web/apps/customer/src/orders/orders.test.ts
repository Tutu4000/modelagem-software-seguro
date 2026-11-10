import { ApiError } from "@nugget/api";
import { digitsOnly } from "@nugget/ui";
import { describe, expect, it } from "vitest";

import type { Order } from "../quote/types";
import { confirmMessage } from "./AddressChangePanel";
import { reasonTooLong } from "./CancelDialog";
import { actions, pollInterval } from "./status";

const now = new Date("2026-11-09T12:00:00Z");

function order(status: Order["status"], extra: Partial<Order> = {}): Order {
  return {
    id: "0b7e3a52-0000-4000-8000-000000000001", file_id: "f1", material: "pla", color: "black", quality: "standard", finishing: "none",
    quantity: 1, price_table_version: 1, unit_cents: 100, discount_cents: 0, shipping_cents: 1500, total_cents: 2600,
    currency: "BRL", status, quote_expires_at: "2026-11-10T00:00:00Z", created_at: "2026-11-09T10:00:00Z", dfm_flags: [],
    ...extra,
  };
}

describe("status", () => {
  it("offers only what the status allows", () => {
    expect(actions(order("quoted"), now)).toMatchObject({ submit: true, cancel: true, pay: false });
    expect(actions(order("quoted", { quote_expires_at: "2026-11-09T11:00:00Z" }), now)).toMatchObject({ submit: false, requote: true });
    expect(actions(order("awaiting_payment"), now)).toMatchObject({ pay: true, cancel: true });
    const pending = { charge_id: "ch", amount_cents: 2600, status: "pending" as const, created_at: "2026-11-09T11:00:00Z" };
    expect(actions(order("awaiting_payment", { payment: pending }), now)).toMatchObject({ pay: false, cancel: false });
    expect(actions(order("in_production"), now)).toMatchObject({ cancel: false, pay: false });
    const refund = { ...pending, status: "refund_pending" as const };
    expect(actions(order("cancelled", { payment: refund }), now).finishRefund).toBe(true);
  });

  it("polls only while something will change by itself", () => {
    const pending = { charge_id: "ch", amount_cents: 2600, status: "pending" as const, created_at: "2026-11-09T11:00:00Z" };
    expect(pollInterval(order("awaiting_payment", { payment: pending }))).toBe(2000);
    expect(pollInterval(order("shipped"))).toBe(5000);
    expect(pollInterval(order("in_production"))).toBe(5000);
    expect(pollInterval(order("paid"))).toBe(5000);
    expect(pollInterval(order("awaiting_review"))).toBe(30_000);
    expect(pollInterval(order("awaiting_payment"))).toBe(false);
    expect(pollInterval(order("delivered"))).toBe(false);
    expect(pollInterval(undefined)).toBe(false);
  });
});

describe("inputs", () => {
  it("keeps only the digits of a code", () => {
    expect(digitsOnly("12a34-5678 9", 8)).toBe("12345678");
  });

  it("limits the reason to 500 code points", () => {
    expect(reasonTooLong("x".repeat(500))).toBe(false);
    expect(reasonTooLong("x".repeat(501))).toBe(true);
    expect(reasonTooLong("👍".repeat(500))).toBe(false);
  });

  it("explains a refused code", () => {
    expect(confirmMessage(new ApiError(400, "invalid_code"))).toMatch(/incorreto/);
    expect(confirmMessage(new ApiError(409, "conflict"))).toMatch(/não está mais aberta/);
    expect(confirmMessage(new Error("x"))).toBeUndefined();
  });
});
