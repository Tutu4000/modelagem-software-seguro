import type { CustomerComponents } from "@nugget/api";
import type { Tone } from "@nugget/ui";

export type Ticket = CustomerComponents["schemas"]["Ticket"];

export const maxSubject = 200;
export const maxMessage = 2000;

/** codePoints counts like the Go API (utf8.RuneCountInString), so the limits
 * here and there agree. */
export function codePoints(s: string): number {
  let n = 0;
  for (let i = 0; i < s.length; i++) {
    const unit = s.charCodeAt(i);
    if (unit >= 0xd800 && unit <= 0xdbff && i + 1 < s.length) {
      i++;
    }
    n++;
  }
  return n;
}

export type CleanResult = { ok: true; text: string; length: number } | { ok: false; reason: string };

// A control character other than the line break: what support.CleanText
// refuses, since it could hide content or break a log line.
// eslint-disable-next-line no-control-regex -- matching control characters is the point
const control = /[\u0000-\u0009\u000b-\u001f\u007f-\u009f]/;

/** cleanText mirrors the API's support.CleanText, only to help: the API
 * decides. */
export function cleanText(input: string, max: number): CleanResult {
  const text = input.replace(/\r\n/g, "\n").trim();
  const length = codePoints(text);
  if (length < 1) {
    return { ok: false, reason: "Escreva alguma coisa." };
  }
  if (length > max) {
    return { ok: false, reason: `Use no máximo ${String(max)} caracteres.` };
  }
  if (control.test(text)) {
    return { ok: false, reason: "O texto tem um caractere de controle. Apague-o e tente de novo." };
  }
  return { ok: true, text, length };
}

const tone: Record<Ticket["status"], Tone> = { open: "info", answered: "warning", closed: "neutral" };

/** The same status, told from each side: the customer waits for the support;
 * the support sees an open ticket. */
const label: Record<"customer" | "staff", Record<Ticket["status"], string>> = {
  customer: { open: "Aguardando o suporte", answered: "Respondido", closed: "Fechado" },
  staff: { open: "Aberto", answered: "Respondido", closed: "Fechado" },
};

export function ticketStatus(status: Ticket["status"], side: "customer" | "staff" = "customer"): { label: string; tone: Tone } {
  return { label: label[side][status], tone: tone[status] };
}
