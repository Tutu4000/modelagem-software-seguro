import { expect, test, type Page } from "@playwright/test";

import { fillLogin, secret, storedTokens } from "./login";

const jwt = /eyJ[\w-]+\.eyJ[\w-]+\./;

/** cspProblems collects the console errors of our own pages: a CSP or a
 * Trusted Types violation shows up there. Keycloak's pages are its own. */
function cspProblems(page: Page): string[] {
  const problems: string[] = [];
  page.on("console", (m) => {
    if (m.type() === "error" && !m.location().url.startsWith("https://localhost:8444")) {
      problems.push(`${m.location().url}: ${m.text()}`);
    }
  });
  page.on("pageerror", (e) => problems.push(e.message));
  return problems;
}

test.describe("customer session", () => {
  test("logs in, survives a reload with no token in storage, and logs out", async ({ page }) => {
    const problems = cspProblems(page);
    await page.goto("https://localhost/");
    await page.getByRole("button", { name: "Entrar" }).first().click();
    await fillLogin(page, "customer-a", false);
    await page.waitForURL("https://localhost/");
    await expect(page.getByText("customer-a").first()).toBeVisible();

    await page.goto("https://localhost/conta");
    await expect(page.getByRole("heading", { name: "Minha conta" })).toBeVisible();
    await expect(page.getByTestId("catalog-version")).toHaveText(/versão \d+/);
    expect(await storedTokens(page)).not.toMatch(jwt);

    // The memory is gone; the hidden frame brings the session back.
    await page.reload();
    await expect(page.getByRole("heading", { name: "Minha conta" })).toBeVisible();
    await expect(page.getByTestId("catalog-version")).toHaveText(/versão \d+/);
    expect(new URL(page.url()).host).toBe("localhost");
    expect(await storedTokens(page)).not.toMatch(jwt);

    await page.getByText("customer-a").first().click();
    await page.getByRole("button", { name: "Sair" }).click();
    await page.waitForURL("https://localhost/");
    await expect(page.getByRole("button", { name: "Entrar" }).first()).toBeVisible();

    await page.goto("https://localhost/conta");
    await page.waitForURL(/localhost:8444\/realms\/nugget\//);
    await expect(page.locator("#username")).toBeVisible();
    expect(problems).toEqual([]);
  });

  test("a protected page sends to the login and back to itself", async ({ page }) => {
    await page.goto("https://localhost/conta");
    await fillLogin(page, "customer-b", false);
    await page.waitForURL("https://localhost/conta");
    await expect(page.getByRole("heading", { name: "Minha conta" })).toBeVisible();
  });
});

test.describe("panel session", () => {
  // A staff login waits for a fresh TOTP step, up to 30 seconds.
  test.describe.configure({ timeout: 2 * 60 * 1000 });

  test("the staff logs in with TOTP, sees the role and confirms the identity in a popup", async ({ page }) => {
    const problems = cspProblems(page);
    await page.goto("https://localhost:9445/");
    await fillLogin(page, "admin", true);
    await page.waitForURL("https://localhost:9445/");
    await expect(page.getByTestId("role")).toHaveText("admin");
    const before = await page.getByTestId("step-up-until").textContent();
    expect(await storedTokens(page)).not.toMatch(jwt);

    const popupPromise = page.waitForEvent("popup");
    await page.getByRole("button", { name: "Confirmar identidade agora" }).click();
    const popup = await popupPromise;
    await fillLogin(popup, "admin", true);
    await popup.waitForEvent("close");
    await expect(page.getByTestId("step-up-until")).not.toHaveText(before ?? "");
    expect(problems).toEqual([]);
  });

  test("a customer cannot get into the panel", async ({ page }) => {
    await page.goto("https://localhost:9445/");
    await page.locator("#username").fill("customer-a");
    await page.locator("#password").fill(secret("keycloak_demo_password"));
    await page.locator("#kc-login").click();
    // Without a TOTP credential the level-2 flow cannot finish: Keycloak stops
    // there, and the panel never gets a token.
    await page.waitForTimeout(1000);
    expect(new URL(page.url()).host).toBe("localhost:8444");
    await expect(page.getByTestId("role")).toHaveCount(0);
  });
});
