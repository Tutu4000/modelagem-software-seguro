import { readFileSync } from "node:fs";
import { Agent } from "node:https";

import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

export default defineConfig(({ command }) => ({
  plugins: [react()],
  build: {
    // Fonts and images as files, never data: URIs, and no inline script:
    // the CSP allows only 'self'.
    assetsInlineLimit: 0,
    sourcemap: false,
  },
  // The dev server proxies the panel API of the compose, trusting its local CA.
  // The build never reads the secrets.
  server:
    command === "serve"
      ? {
          port: 5174,
          proxy: {
            "/panel": {
              target: "https://localhost:9445",
              changeOrigin: false,
              agent: new Agent({ ca: readFileSync(new URL("../../../secrets/tls_ca.pem", import.meta.url)) }),
            },
          },
        }
      : {},
}));
