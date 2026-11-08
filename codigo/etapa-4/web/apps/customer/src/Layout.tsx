import { AppShell, Button } from "@nugget/ui";
import { NavLink, Outlet } from "react-router";

// The sections arrive with the next phases: login (FE1), quote (FE2),
// orders (FE3) and support (FE4). Until then they show, disabled.
const upcoming = ["Orçar", "Pedidos", "Suporte"];

export function Layout() {
  return (
    <AppShell
      brand="nugget-machine"
      nav={
        <>
          <NavLink to="/" end>
            Início
          </NavLink>
          {upcoming.map((label) => (
            <a key={label} aria-disabled="true">
              {label}
            </a>
          ))}
        </>
      }
      actions={
        <Button variant="primary" disabled>
          Entrar
        </Button>
      }
      footer={<span>nugget-machine · impressão 3D sob demanda · ambiente de demonstração</span>}
    >
      <Outlet />
    </AppShell>
  );
}
