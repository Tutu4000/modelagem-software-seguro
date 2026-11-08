import { lazy, Suspense } from "react";
import { createBrowserRouter, type RouteObject } from "react-router";

import { Layout } from "./Layout";
import { Home } from "./pages/Home";
import { NotFound } from "./pages/NotFound";

const routes: RouteObject[] = [{ index: true, element: <Home /> }];

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
