import { expect, test, type Page } from "@playwright/test";

/** watch fails the test on any console error or uncaught exception: a CSP
 * or Trusted Types violation shows up as a console error. */
function watch(page: Page): string[] {
  const problems: string[] = [];
  page.on("console", (m) => {
    if (m.type() === "error") {
      problems.push(m.text());
    }
  });
  page.on("pageerror", (e) => problems.push(e.message));
  return problems;
}

test("the customer app loads with no CSP violation", async ({ page }) => {
  const problems = watch(page);
  await page.goto("https://localhost/");
  await expect(page.getByRole("heading", { level: 1, name: "Envie seu STL e receba o orçamento" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Como funciona" })).toBeVisible();
  const font = await page.evaluate(async () => {
    await document.fonts.ready;
    return document.fonts.check('16px "IBM Plex Sans"');
  });
  expect(font, "the bundled font loads from 'self'").toBe(true);
  expect(problems).toEqual([]);
});

test("a deep link returns the app, and the panel API is not on the public port", async ({ page, request }) => {
  const problems = watch(page);
  const res = await page.goto("https://localhost/caminho/que-nao-existe");
  expect(res?.status()).toBe(200);
  await expect(page.getByRole("heading", { name: "Página não encontrada" })).toBeVisible();
  expect((await request.get("https://localhost/panel/v1/me")).status()).toBe(404);
  expect((await request.get("https://localhost/metrics")).status()).toBe(404);
  expect(problems).toEqual([]);
});

test("the mobile menu opens and closes", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("https://localhost/");
  const open = page.getByRole("button", { name: "Abrir menu" });
  await expect(open).toHaveAttribute("aria-expanded", "false");
  await open.click();
  await expect(page.getByRole("navigation", { name: "Menu" })).toBeVisible();
  await page.getByRole("button", { name: "Fechar menu" }).click();
  await expect(page.getByRole("navigation", { name: "Menu" })).toBeHidden();
});

test("the panel sends to the staff login right away", async ({ page }) => {
  const problems = watch(page);
  await page.goto("https://localhost:9445/");
  await page.waitForURL(/localhost:8444\/realms\/nugget\/.*client_id=panel-web/);
  expect(new URL(page.url()).searchParams.get("acr_values")).toBe("2");
  expect(problems).toEqual([]);
});

// Screenshots for the visual review on phone, tablet and desktop. No pixel
// assertion: they are kept as artifacts.
for (const [name, width, height] of [
  ["phone", 390, 844],
  ["tablet", 820, 1180],
  ["desktop", 1440, 900],
] as const) {
  test(`screenshots on ${name}`, async ({ page }) => {
    await page.setViewportSize({ width, height });
    await page.goto("https://localhost/");
    await page.screenshot({ path: `e2e/results/screens/customer-${name}.png`, fullPage: true });
    await page.goto("https://localhost:9445/");
    await page.screenshot({ path: `e2e/results/screens/panel-${name}.png`, fullPage: true });
  });
}
