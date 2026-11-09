import { expect, test, type Browser, type Page } from "@playwright/test";

import { apiToken, fillLogin } from "./login";
import { approvedOrder, submittedOrder } from "./setup";

const panelBase = "https://localhost:9445";

/** problems collects CSP and Trusted Types violations and script errors,
 * leaving out the 4xx answers these tests provoke on purpose. */
function problems(page: Page): string[] {
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
async function staffPage(browser: Browser, username: string, path: string): Promise<{ page: Page; errors: string[] }> {
  const ctx = await browser.newContext({ ignoreHTTPSErrors: true, locale: "pt-BR" });
  const page = await ctx.newPage();
  const errors = problems(page);
  await page.goto(`${panelBase}${path}`);
  await fillLogin(page, username, true);
  await page.waitForURL(`${panelBase}${path}`);
  return { page, errors };
}

async function openOrder(page: Page, id: string) {
  const row = page.getByRole("row").filter({ hasText: id.slice(0, 8) });
  await row.getByRole("link", { name: id.slice(0, 8) }).click();
  await page.waitForURL(`${panelBase}/pedidos/${id}`);
}

// Every staff login waits for a fresh TOTP step: these tests take minutes.
test.describe.configure({ timeout: 6 * 60 * 1000 });

test("the admin assigns, and the technician opens the design, reviews, prints and ships", async ({ browser, request }) => {
  const id = await submittedOrder(request, "customer-e");

  const admin = await staffPage(browser, "admin", "/pedidos");
  await expect(admin.page.getByRole("heading", { name: "Pedidos" })).toBeVisible();
  await admin.page.getByRole("radio", { name: "Em revisão", exact: true }).check();
  await openOrder(admin.page, id);
  await expect(admin.page.getByTestId("order-status")).toHaveText("Em revisão técnica");
  await admin.page.getByLabel("Técnico").selectOption({ label: "technician" });
  await admin.page.getByRole("button", { name: "Atribuir" }).click();
  await expect(admin.page.getByTestId("assigned-technician")).toHaveText("Atribuído a technician.");
  expect(admin.errors).toEqual([]);

  const tech = await staffPage(browser, "technician", "/pedidos");
  const { page } = tech;
  await expect(page.getByRole("heading", { name: "Meus pedidos" })).toBeVisible();
  await openOrder(page, id);
  // The technician never sees the address (LGPD).
  await expect(page.getByTestId("shipping-address")).toHaveText("Endereço visível só para suporte e administração");
  await expect(page.getByTestId("order-dimensions")).toHaveText("10 × 20 × 30 mm");

  // Opening the design is critical: a justification, then the 3D view, with
  // the STL parsed in the panel's worker (Trusted Types).
  await page.getByRole("button", { name: "Abrir projeto" }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel("Justificativa").fill("e2e: conferir a espessura das paredes");
  const file = page.waitForResponse((r) => r.url().endsWith(`/panel/v1/orders/${id}/file`));
  await dialog.getByRole("button", { name: "Abrir projeto" }).click();
  expect((await file).status()).toBe(200);
  await expect(page.getByTestId("dimensions")).toHaveText("10 × 20 × 30 mm");
  await expect(page.getByRole("link", { name: /baixar|download/i })).toHaveCount(0);

  await page.getByRole("radio", { name: "Aprovar" }).check();
  await page.getByRole("button", { name: "Registrar revisão" }).click();
  await expect(page.getByText("Escreva pelo menos 10 caracteres.")).toBeVisible();
  await page.getByLabel("Laudo").fill("e2e: paredes de 2 mm, cabe na impressora, aprovado.");
  await page.getByRole("button", { name: "Registrar revisão" }).click();
  await expect(page.getByTestId("review-report")).toHaveText("e2e: paredes de 2 mm, cabe na impressora, aprovado.");
  await expect(page.getByTestId("order-status")).toHaveText("Aguardando pagamento");
  await expect(page.getByRole("button", { name: "Registrar revisão" })).toHaveCount(0);

  // The customer pays; the gateway's signed notification confirms it.
  const customer = { Authorization: `Bearer ${await apiToken(request, "customer-e")}` };
  expect((await request.post(`https://localhost/api/v1/orders/${id}/payment`, { headers: customer })).ok()).toBe(true);
  await expect
    .poll(async () => {
      await page.reload();
      return page.getByTestId("order-status").textContent();
    }, { timeout: 30_000 })
    .toBe("Pago");

  await page.getByRole("button", { name: "Iniciar produção" }).click();
  await expect(page.getByTestId("order-status")).toHaveText("Em produção");
  await page.getByRole("button", { name: "Agendar coleta" }).click();
  await expect(page.getByTestId("tracking")).toHaveText(/^NG[0-9A-F]+BR$/);
  await expect(page.getByRole("button", { name: "Agendar coleta" })).toHaveCount(0);
  expect(tech.errors).toEqual([]);
});

test("another technician cannot see the order, and the support sees the address", async ({ browser, request }) => {
  const id = await approvedOrder(request, "customer-f");

  const other = await staffPage(browser, "technician-2", `/pedidos/${id}`);
  await expect(other.page.getByRole("heading", { name: "Pedido não encontrado ou não atribuído a você." })).toBeVisible();
  await other.page.goto(`${panelBase}/pedidos`);
  await expect(other.page.getByRole("heading", { name: "Meus pedidos" })).toBeVisible();
  await expect(other.page.getByRole("link", { name: id.slice(0, 8) })).toHaveCount(0);

  const support = await staffPage(browser, "support", `/pedidos/${id}`);
  const { page } = support;
  await expect(page.getByTestId("shipping-address")).toContainText("Avenida Paulista, 1000");
  // Only the assigned technician works on the order.
  await expect(page.getByRole("button", { name: "Abrir projeto" })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Atribuir" })).toHaveCount(0);

  await page.setViewportSize({ width: 390, height: 844 });
  await page.screenshot({ path: "e2e/results/screens/panel-order-phone.png", fullPage: true });
  await page.goto(`${panelBase}/pedidos`);
  await page.getByRole("radio", { name: "Aguardando pagamento" }).check();
  await expect(page.getByRole("link", { name: id.slice(0, 8) })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
  await page.screenshot({ path: "e2e/results/screens/panel-orders-phone.png", fullPage: true });
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.screenshot({ path: "e2e/results/screens/panel-orders-desktop.png", fullPage: true });
  expect(other.errors).toEqual([]);
  expect(support.errors).toEqual([]);
});
