import { AuthCallback, RequireSession } from "@nugget/auth";
import { Banner, Button, Panel, Spinner, Stack } from "@nugget/ui";
import { lazy, Suspense, type ReactNode } from "react";
import { createBrowserRouter, type RouteObject } from "react-router";

import { manager } from "./auth";
import { Layout } from "./Layout";
import { Account } from "./pages/Account";
import { Home } from "./pages/Home";
import { NotFound } from "./pages/NotFound";
import { OrderPage } from "./orders/OrderPage";
import { OrdersPage } from "./orders/OrdersPage";
import { QuotePage } from "./quote/QuotePage";
import { NewTicketPage } from "./support/NewTicketPage";
import { TicketPage } from "./support/TicketPage";
import { TicketsPage } from "./support/TicketsPage";

function Protected({ children }: { children: ReactNode }) {
  return (
    <RequireSession
      roles={["customer"]}
      loading={<Spinner label="Conferindo a sessão…" />}
      expired={(signIn) => (
        <Banner tone="warning" action={<Button variant="primary" onClick={signIn}>Entrar de novo</Button>}>
          Sua sessão expirou.
        </Banner>
      )}
      forbidden={
        <Panel>
          <Panel.Body>
            <Stack gap={2}>
              <h1>Área do cliente</h1>
              <p>Esta conta é da equipe. Use o painel interno.</p>
            </Stack>
          </Panel.Body>
        </Panel>
      }
    >
      {children}
    </RequireSession>
  );
}

const routes: RouteObject[] = [
  { index: true, element: <Home /> },
  {
    path: "auth/callback",
    element: (
      <AuthCallback
        manager={manager}
        loading={<Spinner label="Entrando…" />}
        failed={<Banner tone="danger">Não foi possível concluir o login. Tente de novo.</Banner>}
      />
    ),
  },
  { path: "conta", element: <Protected><Account /></Protected> },
  { path: "orcar", element: <Protected><QuotePage /></Protected> },
  { path: "pedidos", element: <Protected><OrdersPage /></Protected> },
  { path: "pedidos/:id", element: <Protected><OrderPage /></Protected> },
  { path: "suporte", element: <Protected><TicketsPage /></Protected> },
  { path: "suporte/novo", element: <Protected><NewTicketPage /></Protected> },
  { path: "suporte/:id", element: <Protected><TicketPage /></Protected> },
];

// The component gallery exists only in development: the production build
// drops this branch, and the gallery with it.
if (import.meta.env.DEV) {
  const DesignGallery = lazy(() => import("@nugget/ui/src/DesignGallery").then((m) => ({ default: m.DesignGallery })));
  routes.push({
    path: "_design",
    element: (
      <Suspense>
        <DesignGallery />
      </Suspense>
    ),
  });
}

routes.push({ path: "*", element: <NotFound /> });

export const router = createBrowserRouter([{ path: "/", element: <Layout />, children: routes }]);
