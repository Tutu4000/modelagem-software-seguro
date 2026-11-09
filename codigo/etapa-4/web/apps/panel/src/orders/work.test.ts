import { ApiError } from "@nugget/api";
import { describe, expect, it } from "vitest";

import { orderProblem, reportError, technicianOptions, work, type StaffOrder } from "./work";

const me = "0b7e3a52-5f1d-4c3e-9a51-7f6a1c2d1a01";
const other = "0b7e3a52-5f1d-4c3e-9a51-7f6a1c2d1a02";

const shipping = { recipient: "Customer", street: "Rua A", number: "1", city: "São Paulo", state: "SP", postal_code: "01310-100" };

function order(status: StaffOrder["status"], extra: Partial<StaffOrder> = {}): StaffOrder {
  return {
    id: "0b7e3a52-0000-4000-8000-000000000001", file_id: "f1", material: "pla", color: "black", quality: "standard", finishing: "none",
    quantity: 1, price_table_version: 1, unit_cents: 100, discount_cents: 0, shipping_cents: 1500, total_cents: 2600,
    currency: "BRL", status, quote_expires_at: "2026-11-10T00:00:00Z", created_at: "2026-11-09T10:00:00Z", dfm_flags: [],
    owner_id: "c1", volume_mm3: 8000, bbox_mm: [20, 20, 20], assigned_technician: me,
    ...extra,
  };
}

const none = { assign: false, openFile: false, review: false, startProduction: false, ship: false, changeAddress: false };

describe("work", () => {
  it("lets the admin assign only an order awaiting review", () => {
    expect(work("admin", other, order("awaiting_review", { assigned_technician: null }))).toEqual({ ...none, assign: true });
    expect(work("admin", other, order("delivered"))).toEqual(none);
  });

  it("gives the design and the review only to the assigned technician", () => {
    expect(work("technician", me, order("awaiting_review"))).toEqual({ ...none, openFile: true, review: true });
    expect(work("technician", other, order("awaiting_review"))).toEqual(none);
    expect(work("support", me, order("awaiting_review"))).toEqual(none);
  });

  it("follows the order into production and shipping", () => {
    expect(work("technician", me, order("paid"))).toEqual({ ...none, startProduction: true });
    expect(work("technician", me, order("in_production"))).toEqual({ ...none, ship: true });
    expect(work("technician", me, order("in_production", { tracking_code: "BR1" }))).toEqual(none);
    expect(work("technician", me, order("shipped"))).toEqual(none);
  });

  it("offers no second review", () => {
    const review = { decision: "approved" as const, report: "ok ok ok ok", reviewed_at: "2026-11-09T11:00:00Z" };
    expect(work("technician", me, order("awaiting_review", { review })).review).toBe(false);
  });
});

describe("address change", () => {
  it("is for the support and the admin until the pickup is booked", () => {
    expect(work("support", other, order("paid", { shipping })).changeAddress).toBe(true);
    expect(work("admin", other, order("in_production", { shipping })).changeAddress).toBe(true);
    expect(work("support", other, order("in_production", { shipping, tracking_code: "BR1" })).changeAddress).toBe(false);
    expect(work("support", other, order("shipped", { shipping })).changeAddress).toBe(false);
    expect(work("technician", me, order("paid", { shipping })).changeAddress).toBe(false);
  });

  it("needs the address the retention did not clear", () => {
    expect(work("support", other, order("paid")).changeAddress).toBe(false);
  });
});

describe("inputs", () => {
  it("limits the report to 10 to 5000 code points", () => {
    expect(reportError("curto")).toMatch(/pelo menos 10/);
    expect(reportError("          x         ")).toMatch(/pelo menos 10/);
    expect(reportError("Paredes de 2 mm, imprimível.")).toBeUndefined();
    expect(reportError("👍".repeat(5000))).toBeUndefined();
    expect(reportError("x".repeat(5001))).toMatch(/no máximo 5000/);
  });

  it("sorts the technicians by name", () => {
    const list = [
      { id: "2", username: "technician-2" },
      { id: "1", username: "technician" },
    ];
    expect(technicianOptions(list)).toEqual([
      { value: "1", label: "technician" },
      { value: "2", label: "technician-2" },
    ]);
  });

  it("explains a refusal without telling a missing order from someone else's", () => {
    expect(orderProblem(new ApiError(403, "forbidden"))).toMatch(/não encontrado ou não atribuído/);
    expect(orderProblem(new ApiError(409, "conflict"))).toMatch(/mudou de estado/);
    expect(orderProblem(new Error("x"))).toMatch(/não foi concluída/);
  });
});
