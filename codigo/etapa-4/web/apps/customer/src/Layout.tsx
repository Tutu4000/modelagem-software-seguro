import { ErrorBanner, SessionRecovery } from "@nugget/api";
import { useSession } from "@nugget/auth";
import { AccountMenu, AppShell, Button, Stack } from "@nugget/ui";
import { Link, NavLink, Outlet, useLocation } from "react-router";

// The sections still to come: orders (FE3) and support (FE4). Until then they
// show, disabled, to whoever is logged in.
const upcoming = ["Pedidos", "Suporte"];

function Account() {
  const session = useSession();
  const location = useLocation();
  const { state } = session;
  if (state.status === "authenticated") {
    return (
      <AccountMenu username={state.claims.username} email={state.claims.email} role="cliente">
        <Link to="/conta">Minha conta</Link>
        <button type="button" onClick={() => void session.signOut()}>
          Sair
        </button>
      </AccountMenu>
    );
  }
  return (
    <Button variant="primary" disabled={state.status === "loading"} onClick={() => void session.signIn(location.pathname)}>
      Entrar
    </Button>
  );
}

export function Layout() {
  const session = useSession();
  const signedIn = session.state.status === "authenticated";
  return (
    <AppShell
      brand="nugget-machine"
      nav={
        <>
          <NavLink to="/" end>
            Início
          </NavLink>
          {signedIn && <NavLink to="/orcar">Orçar</NavLink>}
          {signedIn &&
            upcoming.map((label) => (
              <a key={label} aria-disabled="true">
                {label}
              </a>
            ))}
        </>
      }
      actions={<Account />}
      footer={<span>nugget-machine · impressão 3D sob demanda · ambiente de demonstração</span>}
    >
      <SessionRecovery refresh={session.refresh} />
      <Stack gap={4}>
        <ErrorBanner />
        <Outlet />
      </Stack>
    </AppShell>
  );
}
