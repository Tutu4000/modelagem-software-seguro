import { defineConfig, devices } from "@playwright/test";

// Records the walkthrough for the presentation (just demo). Not part of the
// test suite: it lives outside the testDir of playwright.config.ts.
export default defineConfig({
  testDir: "demo",
  outputDir: "demo/results",
  reporter: [["list"]],
  fullyParallel: false,
  workers: 1,
  timeout: 15 * 60 * 1000,
  use: { ignoreHTTPSErrors: true, locale: "pt-BR", launchOptions: { slowMo: 300 } },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
});
