import type { Role } from "@nugget/auth";

export type StaffRole = Exclude<Role, "customer">;

export const roleLabel: Record<StaffRole, string> = {
  technician: "técnico",
  support: "suporte",
  admin: "admin",
};

export interface Section {
  label: string;
  /** Where it is; a section without a path arrives in a later phase. */
  to?: string;
}

/** The sections each role sees. The panel API decides for real (D02). */
export const sections: Record<StaffRole, readonly Section[]> = {
  technician: [{ label: "Meus pedidos", to: "/pedidos" }],
  support: [{ label: "Pedidos", to: "/pedidos" }, { label: "Chamados" }],
  admin: [{ label: "Pedidos", to: "/pedidos" }, { label: "Chamados" }, { label: "Tabelas de preço" }],
};

export function staffRole(roles: readonly Role[]): StaffRole | undefined {
  return roles.find((r): r is StaffRole => r !== "customer");
}
