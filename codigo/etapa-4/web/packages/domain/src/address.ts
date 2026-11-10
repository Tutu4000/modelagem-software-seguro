// The delivery address as the API validates it (partners.Address.Validate):
// the customer's quote and the support's address change use the same rules.
import type { CustomerComponents } from "@nugget/api";

export type Address = CustomerComponents["schemas"]["Address"];

export const states = [
  "AC", "AL", "AP", "AM", "BA", "CE", "DF", "ES", "GO", "MA", "MT", "MS", "MG", "PA",
  "PB", "PR", "PE", "PI", "RJ", "RN", "RS", "RO", "RR", "SC", "SP", "SE", "TO",
] as const;

export const emptyAddress: Address = { recipient: "", street: "", number: "", city: "", state: "SP", postal_code: "" };

/** maskPostalCode writes the digits typed as 00000-000. */
export function maskPostalCode(input: string): string {
  const digits = input.replace(/\D/g, "").slice(0, 8);
  return digits.length > 5 ? `${digits.slice(0, 5)}-${digits.slice(5)}` : digits;
}

export type AddressErrors = Partial<Record<keyof Address, string>>;

/** validateAddress mirrors the API's rules (partners.Address.Validate), only
 * to help: the API decides. */
export function validateAddress(a: Address): AddressErrors {
  const errors: AddressErrors = {};
  for (const field of ["recipient", "street", "number", "city"] as const) {
    const n = a[field].trim().length;
    if (n < 1) {
      errors[field] = "Obrigatório.";
    } else if (n > 120) {
      errors[field] = "Use no máximo 120 caracteres.";
    }
  }
  if (!(states as readonly string[]).includes(a.state)) {
    errors.state = "Escolha a UF.";
  }
  if (!/^\d{5}-?\d{3}$/.test(a.postal_code)) {
    errors.postal_code = "Use o formato 00000-000.";
  }
  return errors;
}
