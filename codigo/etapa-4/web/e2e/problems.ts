// What the browser reports as broken on our own pages: CSP and Trusted Types
// violations and script errors. Keycloak's pages are its own. The browser
// also logs every 4xx answer as a console error; those are the refusals the
// tests provoke (and the rate limits they may hit), so they are left out.
import type { Page } from "@playwright/test";

export function problems(page: Page): string[] {
  const found: string[] = [];
  page.on("console", (m) => {
    if (m.type() === "error" && !m.location().url.startsWith("https://localhost:8444") && !m.text().startsWith("Failed to load resource")) {
      found.push(`${m.location().url}: ${m.text()}`);
    }
  });
  page.on("pageerror", (e) => found.push(e.message));
  return found;
}
