import type { Role } from "@nugget/auth";

export type StaffRole = Exclude<Role, "customer">;

export const roleLabel: Record<StaffRole, string> = {
  technician: "técnico",
  support: "suporte",
  admin: "admin",
};

export interface Section {
  label: string;
  to: string;
}

/** The sections each role sees. The panel API decides for real (D02). */
export const sections: Record<StaffRole, readonly Section[]> = {
  technician: [{ label: "Meus pedidos", to: "/pedidos" }],
  support: [
    { label: "Pedidos", to: "/pedidos" },
    { label: "Chamados", to: "/chamados" },
  ],
  admin: [
    { label: "Pedidos", to: "/pedidos" },
    { label: "Chamados", to: "/chamados" },
    { label: "Tabelas de preço", to: "/precos" },
    { label: "Revogar acesso", to: "/revogar" },
  ],
};

export function staffRole(roles: readonly Role[]): StaffRole | undefined {
  return roles.find((r): r is StaffRole => r !== "customer");
}
