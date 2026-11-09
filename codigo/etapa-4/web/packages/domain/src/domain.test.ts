import type { CustomerComponents } from "@nugget/api";
import { describe, expect, it } from "vitest";

import { colorLabel, materialLabel } from "./labels";
import { matches, statusView, timeline, type Lifecycle, type Status } from "./status";

const now = new Date("2026-11-09T12:00:00Z");

type Change = NonNullable<CustomerComponents["schemas"]["Order"]["history"]>[number];

function order(status: Status, history: Change[] = [], quoteExpiresAt = "2026-11-10T00:00:00Z"): Lifecycle {
  return { status, quote_expires_at: quoteExpiresAt, history };
}

const step = (to: Status, at: string): Change => ({ from: null, to, at });

describe("status", () => {
  it("names an expired quote", () => {
    expect(statusView(order("quoted"), now).label).toBe("Orçado");
    expect(statusView(order("quoted", [], "2026-11-09T11:00:00Z"), now).label).toBe("Orçamento vencido");
  });

  it("filters the list", () => {
    expect(matches(order("delivered"), "done")).toBe(true);
    expect(matches(order("rejected"), "cancelled")).toBe(true);
    expect(matches(order("paid"), "open")).toBe(true);
    expect(matches(order("paid"), "done")).toBe(false);
  });
});

describe("timeline", () => {
  it("marks the steps done, current and ahead", () => {
    const o = order("paid", [step("quoted", "2026-11-09T10:00:00Z"), step("awaiting_review", "2026-11-09T10:01:00Z"), step("awaiting_payment", "2026-11-09T10:05:00Z"), step("paid", "2026-11-09T10:06:00Z")]);
    const t = timeline(o);
    expect(t.map((i) => i.state)).toEqual(["done", "done", "done", "current", "pending", "pending", "pending"]);
    expect(t[3]?.label).toBe("Pago");
  });

  it("ends where a cancelled order stopped", () => {
    const o = order("cancelled", [step("quoted", "2026-11-09T10:00:00Z"), step("cancelled", "2026-11-09T10:02:00Z")]);
    expect(timeline(o).map((i) => `${i.label}:${i.state}`)).toEqual(["Orçado:done", "Cancelado:current"]);
  });
});

describe("labels", () => {
  it("names the catalog keys and keeps an unknown one", () => {
    expect(materialLabel("nylon")).toBe("Nylon PA12");
    expect(colorLabel("black")).toBe("Preto");
    expect(colorLabel("ultraviolet")).toBe("ultraviolet");
  });
});
