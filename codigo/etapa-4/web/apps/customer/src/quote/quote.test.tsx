import { money } from "@nugget/ui";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it } from "vitest";

import { emptyAddress, maskPostalCode, validateAddress } from "./address";
import { ConfigForm } from "./ConfigForm";
import { lines } from "./QuotePanel";
import type { Address, Choice, Order, Rules } from "./types";

const rules: Rules = {
  materials: {
    pla: { name: "PLA", cents_per_cm3: 15, colors: ["black", "white"] },
    petg: { name: "PETG", cents_per_cm3: 20, colors: ["clear", "blue"] },
  },
  qualities: { standard: 1000, fine: 1500 },
  finishings: { none: 0, painted: 1500 },
  setup_cents: 1000,
  discounts: [{ min_quantity: 10, percent_off: 5 }],
};

describe("address", () => {
  it("masks and validates the postal code like the API", () => {
    expect(maskPostalCode("01310100")).toBe("01310-100");
    expect(maskPostalCode("01a3")).toBe("013");
    const ok: Address = { recipient: "A", street: "Rua", number: "1", city: "SP", state: "SP", postal_code: "01310-100" };
    expect(validateAddress(ok)).toEqual({});
    expect(Object.keys(validateAddress(emptyAddress)).sort()).toEqual(["city", "number", "postal_code", "recipient", "street"]);
    const bad = validateAddress({ ...ok, state: "XX", city: "x".repeat(121) });
    expect(Object.keys(bad).sort()).toEqual(["city", "state"]);
  });
});

describe("quote lines", () => {
  it("add up to the server's total", () => {
    const o: Order = {
      id: "o1", file_id: "f1", material: "pla", color: "black", quality: "standard", finishing: "none", quantity: 10,
      price_table_version: 1, unit_cents: 100, discount_cents: 50, shipping_cents: 1700, total_cents: 3650,
      currency: "BRL", status: "quoted", quote_expires_at: "2026-11-10T00:00:00Z", created_at: "2026-11-09T00:00:00Z", dfm_flags: [],
    };
    const l = lines(o);
    expect(l.map((x) => x.label)).toEqual(["Peças (10 × unidade)", "Desconto por volume", "Preparação", "Frete"]);
    const sum = l.reduce((acc, x) => acc + (x.discount === true ? -x.cents : x.cents), 0);
    expect(sum).toBe(o.total_cents);
    expect(money(3650)).toBe("R$ 36,50");
  });
});

function Form() {
  const [choice, setChoice] = useState<Choice>({ material: "pla", color: "white", quality: "standard", finishing: "none", quantity: 1 });
  const [address, setAddress] = useState<Address>(emptyAddress);
  return <ConfigForm rules={rules} choice={choice} onChoice={setChoice} address={address} onAddress={setAddress} errors={{}} disabled={false} />;
}

describe("ConfigForm", () => {
  it("offers only the colors of the chosen material", async () => {
    const user = userEvent.setup();
    render(<Form />);
    expect(screen.getByRole("radio", { name: "Branco" })).toBeChecked();
    await user.click(screen.getByRole("radio", { name: /PETG/ }));
    expect(screen.queryByRole("radio", { name: "Branco" })).toBeNull();
    expect(screen.getByRole("radio", { name: "Transparente" })).toBeChecked();
  });

  it("masks the postal code as it is typed", async () => {
    const user = userEvent.setup();
    render(<Form />);
    await user.type(screen.getByLabelText("CEP"), "01310100");
    expect(screen.getByLabelText("CEP")).toHaveValue("01310-100");
  });
});
