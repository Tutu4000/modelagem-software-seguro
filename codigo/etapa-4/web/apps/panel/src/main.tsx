import "@nugget/ui/styles.css";

import { createQueryClient } from "@nugget/api";
import { AuthProvider } from "@nugget/auth";
import { QueryClientProvider } from "@tanstack/react-query";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { RouterProvider } from "react-router/dom";

import { manager } from "./auth";
import { router } from "./router";

const root = document.getElementById("root");
if (root === null) {
  throw new Error("missing #root");
}

createRoot(root).render(
  <StrictMode>
    <AuthProvider manager={manager}>
      <QueryClientProvider client={createQueryClient()}>
        <RouterProvider router={router} />
      </QueryClientProvider>
    </AuthProvider>
  </StrictMode>,
);
