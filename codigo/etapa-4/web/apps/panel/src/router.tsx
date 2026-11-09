import { AuthCallback, RequireSession } from "@nugget/auth";
import { Banner, Button, Panel, Spinner, Stack } from "@nugget/ui";
import { lazy, Suspense } from "react";
import { createBrowserRouter, Outlet, type RouteObject } from "react-router";

import { manager } from "./auth";
import { Layout } from "./Layout";
import { OrderPage } from "./orders/OrderPage";
import { OrdersPage } from "./orders/OrdersPage";
import { Home } from "./pages/Home";
import { NotFound } from "./pages/NotFound";
import { NewPricePage } from "./prices/NewPricePage";
import { PricesPage } from "./prices/PricesPage";
import { PriceTablePage } from "./prices/PriceTablePage";
import { RevokePage } from "./RevokePage";
import { TicketPage } from "./tickets/TicketPage";
import { TicketsPage } from "./tickets/TicketsPage";

/** Everything in the panel needs a staff session with the second factor. */
function Protected() {
  return (
    <RequireSession
      roles={["technician", "support", "admin"]}
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
              <h1>Acesso negado</h1>
              <p>O painel é só da equipe.</p>
            </Stack>
          </Panel.Body>
        </Panel>
      }
    >
      <Outlet />
    </RequireSession>
  );
}

const routes: RouteObject[] = [
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
  {
    element: <Protected />,
    children: [
      { index: true, element: <Home /> },
      { path: "pedidos", element: <OrdersPage /> },
      { path: "pedidos/:id", element: <OrderPage /> },
      { path: "chamados", element: <TicketsPage /> },
      { path: "chamados/:id", element: <TicketPage /> },
      { path: "precos", element: <PricesPage /> },
      { path: "precos/nova", element: <NewPricePage /> },
      { path: "precos/:version", element: <PriceTablePage /> },
      { path: "revogar", element: <RevokePage /> },
    ],
  },
];

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
