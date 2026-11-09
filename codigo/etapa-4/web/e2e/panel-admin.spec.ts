import { expect, test, type APIRequestContext } from "@playwright/test";

import { apiToken } from "./login";
import { panelBase, staffPage } from "./panel";
import { approvedOrder, mailCode } from "./setup";

async function customerApi(request: APIRequestContext, customer: string) {
  const headers = { Authorization: `Bearer ${await apiToken(request, customer)}` };
  return {
    get: (path: string) => request.get(`https://localhost${path}`, { headers }),
    post: (path: string, data: object) => request.post(`https://localhost${path}`, { headers, data }),
  };
}

// Every staff login waits for a fresh TOTP step: these tests take minutes.
test.describe.configure({ timeout: 6 * 60 * 1000 });

test("the support answers a ticket and proposes the address change the customer confirms", async ({ browser, request }) => {
  const id = await approvedOrder(request, "customer-e");
  const customer = await customerApi(request, "customer-e");
  const opened = await customer.post("/api/v1/tickets", { order_id: id, subject: "e2e: mudei de endereço", message: "Preciso entregar na Rua Augusta, 500." });
  expect(opened.ok()).toBe(true);
  const ticket = ((await opened.json()) as { id: string }).id;

  const { page, errors } = await staffPage(browser, "support", "/chamados");
  await expect(page.getByRole("heading", { name: "Chamados" })).toBeVisible();
  await page.getByRole("link", { name: ticket.slice(0, 8) }).click();
  await page.waitForURL(`${panelBase}/chamados/${ticket}`);
  await expect(page.getByTestId("ticket-status")).toHaveText("Aberto");
  await expect(page.getByText("Preciso entregar na Rua Augusta, 500.")).toBeVisible();
  await page.getByLabel("Resposta").fill("Vamos propor a troca. Confirme com o código do e-mail.");
  await page.getByRole("button", { name: "Responder" }).click();
  await expect(page.getByTestId("ticket-status")).toHaveText("Respondido");
  const seen = (await (await customer.get(`/api/v1/tickets/${ticket}`)).json()) as { messages: { author_kind: string; body: string }[] };
  expect(seen.messages.at(-1)).toMatchObject({ author_kind: "staff", body: "Vamos propor a troca. Confirme com o código do e-mail." });

  await page.setViewportSize({ width: 390, height: 844 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
  await page.screenshot({ path: "e2e/results/screens/panel-ticket-phone.png", fullPage: true });
  await page.setViewportSize({ width: 1280, height: 900 });

  // From the ticket to its order: the change is tied to the ticket.
  await page.getByRole("link", { name: id.slice(0, 8) }).click();
  await page.waitForURL(`${panelBase}/pedidos/${id}?chamado=${ticket}`);
  await page.getByLabel("Destinatário").fill("Customer E");
  await page.getByLabel("CEP").fill("01304000");
  await page.getByLabel("Logradouro").fill("Rua Augusta");
  await page.getByLabel("Número").fill("500");
  await page.getByLabel("Cidade").fill("São Paulo");
  await page.getByRole("button", { name: "Propor troca" }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel("Justificativa").fill("e2e: o cliente pediu a troca no chamado");
  await dialog.getByRole("button", { name: "Enviar código ao cliente" }).click();
  await expect(page.getByTestId("address-change-sent")).toContainText("Código enviado ao cliente");

  const confirmed = await customer.post(`/api/v1/orders/${id}/address-change/confirm`, { code: await mailCode(request, "customer-e", id) });
  expect(confirmed.ok()).toBe(true);
  await page.reload();
  await expect(page.getByTestId("shipping-address")).toContainText("Rua Augusta, 500");

  await page.goto(`${panelBase}/chamados/${ticket}`);
  await page.getByRole("button", { name: "Fechar chamado" }).click();
  await expect(page.getByTestId("ticket-status")).toHaveText("Fechado");
  await expect(page.getByRole("button", { name: "Responder" })).toHaveCount(0);
  expect(errors).toEqual([]);
});

test("a price table needs a second admin, who sees what changes", async ({ browser, request }) => {
  const customer = await customerApi(request, "customer-a");
  const before = (await (await customer.get("/api/v1/catalog")).json()) as { version: number; rules: { materials: Record<string, { cents_per_cm3: number }> } };
  const pla = (before.rules.materials.pla?.cents_per_cm3 ?? 0) + 1;
  const plaInput = `${String(Math.trunc(pla / 100))},${String(pla % 100).padStart(2, "0")}`;

  const admin = await staffPage(browser, "admin", "/precos");
  await admin.page.getByRole("link", { name: "Propor nova tabela" }).click();
  await admin.page.getByLabel("PLA", { exact: true }).fill(plaInput);
  await admin.page.setViewportSize({ width: 390, height: 844 });
  expect(await admin.page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
  await admin.page.screenshot({ path: "e2e/results/screens/panel-price-form-phone.png", fullPage: true });
  await admin.page.setViewportSize({ width: 1280, height: 900 });
  await admin.page.getByRole("button", { name: "Propor tabela" }).click();
  await admin.page.waitForURL(/\/precos\/\d+$/);
  const version = admin.page.url().split("/").at(-1) ?? "";
  await expect(admin.page.getByTestId("table-status")).toHaveText("Rascunho");
  await expect(admin.page.getByTestId("own-draft")).toBeVisible();
  await expect(admin.page.getByRole("button", { name: "Aprovar" })).toHaveCount(0);

  const second = await staffPage(browser, "admin-2", `/precos/${version}`);
  const change = second.page.getByRole("row").filter({ hasText: "PLA por cm³" });
  await expect(change).toContainText(plaInput);
  await second.page.getByRole("button", { name: "Aprovar" }).click();
  const dialog = second.page.getByRole("dialog");
  await dialog.getByLabel("Justificativa").fill("e2e: reajuste do PLA conferido");
  await dialog.getByRole("button", { name: "Aprovar e ativar" }).click();
  await expect(second.page.getByTestId("table-status")).toHaveText("Ativa");

  const after = (await (await customer.get("/api/v1/catalog")).json()) as typeof before;
  expect(after.version).toBe(Number(version));
  expect(after.rules.materials.pla?.cents_per_cm3).toBe(pla);
  expect(admin.errors).toEqual([]);
  expect(second.errors).toEqual([]);
});

test("the admin revokes a customer: the token stops working and the login fails", async ({ browser, request }) => {
  const customerG = "0b7e3a52-5f1d-4c3e-9a51-7f6a1c2d1007";
  // The revocation sticks: customer-g comes back only with `just reset`.
  const token = await apiToken(request, "customer-g").catch(() => undefined);
  test.skip(token === undefined, "customer-g já foi revogado neste ambiente; rode just reset para testar de novo");
  const headers = { Authorization: `Bearer ${token ?? ""}` };
  expect((await request.get("https://localhost/api/v1/catalog", { headers })).status()).toBe(200);

  const { page, errors } = await staffPage(browser, "admin", `/revogar?usuario=${customerG}`);
  await page.getByRole("button", { name: "Revogar", exact: true }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel("Justificativa").fill("e2e: conta vista num vazamento de senhas");
  await dialog.getByRole("button", { name: "Revogar agora" }).click();
  await expect(page.getByTestId("revoke-result")).toBeVisible();

  expect((await request.get("https://localhost/api/v1/catalog", { headers })).status()).toBe(401);
  await expect(apiToken(request, "customer-g")).rejects.toThrow(/no token for customer-g/);
  expect(errors).toEqual([]);
});
