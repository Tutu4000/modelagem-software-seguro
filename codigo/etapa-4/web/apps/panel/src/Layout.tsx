import { AppShell } from "@nugget/ui";
import { Outlet } from "react-router";

// The sections arrive with FE5 and FE6; the login with FE1.
const upcoming = ["Pedidos", "Chamados", "Tabelas de preço"];

export function Layout() {
  return (
    <AppShell
      brand="nugget-machine · painel"
      nav={upcoming.map((label) => (
        <a key={label} aria-disabled="true">
          {label}
        </a>
      ))}
      footer={<span>Painel interno · acesso só pela VPN, com MFA · cada acesso fica na auditoria</span>}
    >
      <Outlet />
    </AppShell>
  );
}
