// The panel through the browser: a staff login with the password and the
// TOTP in its own context, with the page problems collected.
import type { Browser, Page } from "@playwright/test";

import { fillLogin } from "./login";
import { problems } from "./problems";

export const panelBase = "https://localhost:9445";

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
