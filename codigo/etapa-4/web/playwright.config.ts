import { defineConfig, devices } from "@playwright/test";

// Runs against the compose (just up), from the official Playwright image on
// the host network. The local CA is not in the image's trust store, so TLS
// errors are ignored here; the chain itself is checked by the Go tests.
export default defineConfig({
  testDir: "e2e",
  outputDir: "e2e/results",
  reporter: [["list"], ["html", { outputFolder: "e2e/report", open: "never" }]],
  fullyParallel: false,
  workers: 1,
  use: { ignoreHTTPSErrors: true, locale: "pt-BR" },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
});
