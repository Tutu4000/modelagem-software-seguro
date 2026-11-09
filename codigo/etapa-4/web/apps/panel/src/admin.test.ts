import { ticketStatus } from "@nugget/domain";
import { describe, expect, it } from "vitest";

import { isUuid } from "./RevokePage";
import { stillOpen } from "./tickets/work";

describe("revoke", () => {
  it("takes only an account ID", () => {
    expect(isUuid("0b7e3a52-5f1d-4c3e-9a51-7f6a1c2d0d04")).toBe(true);
    expect(isUuid(" 0B7E3A52-5F1D-4C3E-9A51-7F6A1C2D0D04 ")).toBe(true);
    expect(isUuid("customer-d")).toBe(false);
    expect(isUuid("0b7e3a52-5f1d-4c3e-9a51-7f6a1c2d0d04/../x")).toBe(false);
  });
});

describe("tickets", () => {
  it("takes answers until it is closed", () => {
    expect(stillOpen({ status: "open" })).toBe(true);
    expect(stillOpen({ status: "answered" })).toBe(true);
    expect(stillOpen({ status: "closed" })).toBe(false);
  });

  it("tells the status from the staff's side", () => {
    expect(ticketStatus("open", "staff").label).toBe("Aberto");
    expect(ticketStatus("open").label).toBe("Aguardando o suporte");
  });
});
