import type { Role } from "@nugget/auth";

export type StaffRole = Exclude<Role, "customer">;

export const roleLabel: Record<StaffRole, string> = {
  technician: "técnico",
  support: "suporte",
  admin: "admin",
};

/** The sections each role sees. The panel API decides for real (D02). */
export const sections: Record<StaffRole, readonly string[]> = {
  technician: ["Pedidos atribuídos"],
  support: ["Pedidos", "Chamados"],
  admin: ["Pedidos", "Chamados", "Tabelas de preço"],
};

export function staffRole(roles: readonly Role[]): StaffRole | undefined {
  return roles.find((r): r is StaffRole => r !== "customer");
}
