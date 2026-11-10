import { ErrorBanner, SessionRecovery } from "@nugget/api";
import { useSession } from "@nugget/auth";
import { AccountMenu, AppShell, Stack } from "@nugget/ui";
import { NavLink, Outlet } from "react-router";

import { roleLabel, sections, staffRole } from "./roles";

export function Layout() {
  const session = useSession();
  const { state } = session;
  const role = state.status === "authenticated" ? staffRole(state.claims.roles) : undefined;
  return (
    <AppShell
      brand="nugget-machine · painel"
      nav={
        role !== undefined && (
          <>
            <NavLink to="/" end>
              Início
            </NavLink>
            {sections[role].map((s) => (
              <NavLink key={s.label} to={s.to}>
                {s.label}
              </NavLink>
            ))}
          </>
        )
      }
      actions={
        state.status === "authenticated" &&
        role !== undefined && (
          <AccountMenu username={state.claims.username} email={state.claims.email} role={roleLabel[role]}>
            <button type="button" onClick={() => void session.signOut()}>
              Sair
            </button>
          </AccountMenu>
        )
      }
      footer={<span>Painel interno · acesso só pela VPN, com MFA · cada acesso fica na auditoria</span>}
    >
      <SessionRecovery refresh={session.refresh} />
      <Stack gap={4}>
        <ErrorBanner />
        <Outlet />
      </Stack>
    </AppShell>
  );
}
