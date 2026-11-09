// The panel through the browser: a staff login with the password and the
// TOTP in its own context, and the page problems the tests check for.
import type { Browser, Page } from "@playwright/test";

import { fillLogin } from "./login";

export const panelBase = "https://localhost:9445";

/** problems collects CSP and Trusted Types violations and script errors,
 * leaving out the 4xx answers these tests provoke on purpose. */
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

/** staffPage logs a staff account in to the panel in its own browser
 * context, with the password and the TOTP. */
export async function staffPage(browser: Browser, username: string, path: string): Promise<{ page: Page; errors: string[] }> {
  const ctx = await browser.newContext({ ignoreHTTPSErrors: true, locale: "pt-BR" });
  const page = await ctx.newPage();
  const errors = problems(page);
  await page.goto(`${panelBase}${path}`);
  await fillLogin(page, username, true);
  await page.waitForURL(`${panelBase}${path}`);
  return { page, errors };
}
