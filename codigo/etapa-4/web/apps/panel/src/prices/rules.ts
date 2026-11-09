// The price table editor: the admin changes values of the active table, never
// the catalog's keys. The checks mirror pricing.Rules.Validate, only to help:
// the API decides.
import type { PanelComponents } from "@nugget/api";
import { finishingLabel, qualityLabel } from "@nugget/domain";
import { money } from "@nugget/ui";

export type Rules = PanelComponents["schemas"]["PriceRules"];
export type PriceTable = PanelComponents["schemas"]["PriceTable"];

/** centsFromInput reads reais as typed ("12,34", "12.34", "12") into cents,
 * without floating point. Anything else, a third decimal or a negative is
 * undefined. */
export function centsFromInput(input: string): number | undefined {
  const m = /^(\d{1,7})(?:[,.](\d{1,2}))?$/.exec(input.trim());
  if (m === null) {
    return undefined;
  }
  const [, whole = "0", frac = ""] = m;
  return Number(whole) * 100 + Number(frac.padEnd(2, "0"));
}

/** inputFromCents writes cents as the editor shows them: "12,34". */
export function inputFromCents(cents: number): string {
  return `${String(Math.trunc(cents / 100))},${String(cents % 100).padStart(2, "0")}`;
}

/** rulesError says what pricing.Rules.Validate would refuse. */
export function rulesError(r: Rules): string | undefined {
  for (const [id, m] of Object.entries(r.materials)) {
    if (m.cents_per_cm3 <= 0) {
      return `O preço de ${m.name || id} deve ser maior que zero.`;
    }
  }
  for (const [id, q] of Object.entries(r.qualities)) {
    if (!Number.isInteger(q) || q < 100 || q > 10_000) {
      return `O multiplicador da qualidade ${qualityLabel(id)} deve ficar entre 100 e 10000 ‰.`;
    }
  }
  for (const [id, f] of Object.entries(r.finishings)) {
    if (f < 0) {
      return `O acabamento ${finishingLabel(id)} não pode ter preço negativo.`;
    }
  }
  if (r.setup_cents < 0) {
    return "A taxa de preparo não pode ser negativa.";
  }
  let lastQty = 0;
  let lastPct = 0;
  for (const t of r.discounts) {
    if (!Number.isInteger(t.min_quantity) || !Number.isInteger(t.percent_off) || t.min_quantity <= lastQty || t.percent_off <= lastPct || t.percent_off >= 100) {
      return "As faixas de desconto devem crescer em quantidade e em porcentagem, abaixo de 100%.";
    }
    lastQty = t.min_quantity;
    lastPct = t.percent_off;
  }
  return undefined;
}

export interface Change {
  label: string;
  before: string;
  after: string;
}

const discountText = (r: Rules) =>
  r.discounts.length === 0 ? "nenhum" : r.discounts.map((t) => `${String(t.min_quantity)} un.: ${String(t.percent_off)}%`).join(" · ");

/** priceDiff lists what a draft changes against the active table, so the
 * second admin approves knowing what moves. */
export function priceDiff(active: Rules, draft: Rules): Change[] {
  const changes: Change[] = [];
  const push = (label: string, before: string, after: string) => {
    if (before !== after) {
      changes.push({ label, before, after });
    }
  };
  for (const id of new Set([...Object.keys(active.materials), ...Object.keys(draft.materials)])) {
    const a = active.materials[id];
    const d = draft.materials[id];
    push(`${d?.name ?? a?.name ?? id} por cm³`, a === undefined ? "—" : money(a.cents_per_cm3), d === undefined ? "—" : money(d.cents_per_cm3));
  }
  for (const id of new Set([...Object.keys(active.qualities), ...Object.keys(draft.qualities)])) {
    const a = active.qualities[id];
    const d = draft.qualities[id];
    push(`Qualidade ${qualityLabel(id)}`, a === undefined ? "—" : `${String(a)} ‰`, d === undefined ? "—" : `${String(d)} ‰`);
  }
  for (const id of new Set([...Object.keys(active.finishings), ...Object.keys(draft.finishings)])) {
    const a = active.finishings[id];
    const d = draft.finishings[id];
    push(`Acabamento ${finishingLabel(id)}`, a === undefined ? "—" : money(a), d === undefined ? "—" : money(d));
  }
  push("Taxa de preparo", money(active.setup_cents), money(draft.setup_cents));
  push("Descontos", discountText(active), discountText(draft));
  return changes;
}

export const tableStatus: Record<PriceTable["status"], { label: string; tone: "success" | "warning" | "neutral" }> = {
  active: { label: "Ativa", tone: "success" },
  draft: { label: "Rascunho", tone: "warning" },
  retired: { label: "Retirada", tone: "neutral" },
};

/** Draft is the editor's state: the values as typed. */
export interface Draft {
  materials: Record<string, string>;
  qualities: Record<string, string>;
  finishings: Record<string, string>;
  setup: string;
  discounts: { min: string; pct: string }[];
}

const mapValues = <A, B>(o: Record<string, A>, f: (a: A) => B): Record<string, B> =>
  Object.fromEntries(Object.entries(o).map(([k, v]) => [k, f(v)]));

export function draftFrom(r: Rules): Draft {
  return {
    materials: mapValues(r.materials, (m) => inputFromCents(m.cents_per_cm3)),
    qualities: mapValues(r.qualities, String),
    finishings: mapValues(r.finishings, inputFromCents),
    setup: inputFromCents(r.setup_cents),
    discounts: r.discounts.map((t) => ({ min: String(t.min_quantity), pct: String(t.percent_off) })),
  };
}

const wholeNumber = (s: string): number | undefined => (/^\d{1,6}$/.test(s.trim()) ? Number(s.trim()) : undefined);

/** rulesFrom turns the editor's values into a table on the keys of the base
 * one (names and colors stay), or says what is wrong. */
export function rulesFrom(base: Rules, d: Draft): { ok: true; rules: Rules } | { ok: false; error: string } {
  const materials: Rules["materials"] = {};
  for (const [id, m] of Object.entries(base.materials)) {
    const cents = centsFromInput(d.materials[id] ?? "");
    if (cents === undefined) {
      return { ok: false, error: `Preço de ${m.name} inválido: use reais, como 0,12.` };
    }
    materials[id] = { ...m, cents_per_cm3: cents };
  }
  const qualities: Rules["qualities"] = {};
  for (const id of Object.keys(base.qualities)) {
    const n = wholeNumber(d.qualities[id] ?? "");
    if (n === undefined) {
      return { ok: false, error: `Multiplicador da qualidade ${qualityLabel(id)} inválido: use um inteiro em ‰.` };
    }
    qualities[id] = n;
  }
  const finishings: Rules["finishings"] = {};
  for (const id of Object.keys(base.finishings)) {
    const cents = centsFromInput(d.finishings[id] ?? "");
    if (cents === undefined) {
      return { ok: false, error: `Preço do acabamento ${finishingLabel(id)} inválido.` };
    }
    finishings[id] = cents;
  }
  const setup = centsFromInput(d.setup);
  if (setup === undefined) {
    return { ok: false, error: "Taxa de preparo inválida." };
  }
  const discounts: Rules["discounts"] = [];
  for (const t of d.discounts) {
    const min = wholeNumber(t.min);
    const pct = wholeNumber(t.pct);
    if (min === undefined || pct === undefined) {
      return { ok: false, error: "Cada faixa de desconto precisa de uma quantidade e uma porcentagem inteiras." };
    }
    discounts.push({ min_quantity: min, percent_off: pct });
  }
  const rules: Rules = { materials, qualities, finishings, setup_cents: setup, discounts };
  const error = rulesError(rules);
  return error === undefined ? { ok: true, rules } : { ok: false, error };
}
