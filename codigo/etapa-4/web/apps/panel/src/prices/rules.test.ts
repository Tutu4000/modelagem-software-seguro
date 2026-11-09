import { describe, expect, it } from "vitest";

import { centsFromInput, draftFrom, inputFromCents, priceDiff, rulesError, rulesFrom, type Rules } from "./rules";

const base: Rules = {
  materials: { pla: { name: "PLA", cents_per_cm3: 12, colors: ["black"] }, petg: { name: "PETG", cents_per_cm3: 15, colors: ["clear"] } },
  qualities: { draft: 800, standard: 1000 },
  finishings: { none: 0, sanded: 500 },
  setup_cents: 1000,
  discounts: [{ min_quantity: 10, percent_off: 5 }],
};

describe("money input", () => {
  it("reads reais into cents without floating point", () => {
    expect(centsFromInput("12,34")).toBe(1234);
    expect(centsFromInput("12.3")).toBe(1230);
    expect(centsFromInput(" 7 ")).toBe(700);
    expect(centsFromInput("0,07")).toBe(7);
  });

  it("refuses a third decimal, a negative or text", () => {
    expect(centsFromInput("12,345")).toBeUndefined();
    expect(centsFromInput("-1")).toBeUndefined();
    expect(centsFromInput("1e3")).toBeUndefined();
    expect(centsFromInput("")).toBeUndefined();
  });

  it("writes cents back as typed", () => {
    expect(inputFromCents(1234)).toBe("12,34");
    expect(inputFromCents(7)).toBe("0,07");
  });
});

describe("rules", () => {
  it("mirrors the API's checks", () => {
    expect(rulesError(base)).toBeUndefined();
    expect(rulesError({ ...base, qualities: { draft: 99 } })).toMatch(/entre 100 e 10000/);
    expect(rulesError({ ...base, materials: { pla: { name: "PLA", cents_per_cm3: 0, colors: ["black"] } } })).toMatch(/maior que zero/);
    expect(rulesError({ ...base, discounts: [{ min_quantity: 10, percent_off: 5 }, { min_quantity: 5, percent_off: 10 }] })).toMatch(/crescer/);
    expect(rulesError({ ...base, discounts: [{ min_quantity: 10, percent_off: 100 }] })).toMatch(/abaixo de 100%/);
  });

  it("keeps the base table's keys, names and colors", () => {
    const d = draftFrom(base);
    const r = rulesFrom(base, { ...d, materials: { ...d.materials, pla: "0,20" } });
    expect(r.ok && r.rules.materials.pla).toEqual({ name: "PLA", cents_per_cm3: 20, colors: ["black"] });
    expect(rulesFrom(base, { ...d, setup: "dez" })).toEqual({ ok: false, error: "Taxa de preparo inválida." });
  });

  it("lists only what changes", () => {
    const draft: Rules = { ...base, materials: { ...base.materials, pla: { name: "PLA", cents_per_cm3: 20, colors: ["black"] } }, discounts: [] };
    const changes = priceDiff(base, draft);
    expect(changes.map((c) => c.label)).toEqual(["PLA por cm³", "Descontos"]);
    expect(changes[1]).toEqual({ label: "Descontos", before: "10 un.: 5%", after: "nenhum" });
    expect(priceDiff(base, base)).toEqual([]);
  });
});
