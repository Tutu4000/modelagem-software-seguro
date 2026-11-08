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
    rolldownOptions: {
      // The app, and the small page that answers the OIDC frame and popup.
      input: {
        main: new URL("index.html", import.meta.url).pathname,
        callback: new URL("oidc-callback.html", import.meta.url).pathname,
      },
    },
  },
  // The dev server proxies the API of the compose, trusting its local CA.
  // The build never reads the secrets. The login needs the compose's origin,
  // so the dev server runs without it.
  server:
    command === "serve"
      ? {
          port: 5173,
          proxy: {
            "/api": {
              target: "https://localhost",
              changeOrigin: false,
              agent: new Agent({ ca: readFileSync(new URL("../../../secrets/tls_ca.pem", import.meta.url)) }),
            },
          },
        }
      : {},
}));
