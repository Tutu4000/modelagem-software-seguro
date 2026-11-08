import { expect, test, type Page } from "@playwright/test";

import { fillLogin } from "./login";
import { approvedOrder, mailCode, panel, submittedOrder } from "./setup";

/** problems collects CSP and Trusted Types violations and script errors.
 * The browser also logs every 4xx answer as a console error; those are the
 * expected refusals these tests provoke, so they are left out. */
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

async function open(page: Page, path: string, customer: string) {
  await page.goto(`https://localhost${path}`);
  await fillLogin(page, customer, false);
  await page.waitForURL(`https://localhost${path}`);
}

// The staff logins wait for fresh TOTP steps: these tests take longer.
test.describe.configure({ timeout: 4 * 60 * 1000 });

test("the customer pays and follows the order until the delivery", async ({ page, request }) => {
  const errors = problems(page);
  const id = await approvedOrder(request, "customer-e");

  await open(page, "/pedidos", "customer-e");
  const row = page.getByRole("row").filter({ hasText: id.slice(0, 8) });
  await expect(row.getByText("Aguardando pagamento")).toBeVisible();
  await row.getByRole("link", { name: id.slice(0, 8) }).click();
  await page.waitForURL(`https://localhost/pedidos/${id}`);

  await page.getByRole("button", { name: /^Pagar R\$/ }).click();
  await expect(page.getByTestId("order-status")).toHaveText("Pago", { timeout: 20_000 });
  await expect(page.getByTestId("payment-status")).toHaveText("Pago");

  await panel(request, "technician", `/panel/v1/orders/${id}/production`);
  await panel(request, "technician", `/panel/v1/orders/${id}/ship`);
  // The carrier's webhooks move the order; the page follows by itself.
  await expect(page.getByTestId("order-status")).toHaveText("Entregue", { timeout: 45_000 });
  await expect(page.getByTestId("tracking")).toHaveText(/^NG[0-9A-F]+BR$/);
  await expect(page.getByRole("list", { name: "Andamento do pedido" }).getByRole("listitem")).toHaveCount(7);
  expect(errors).toEqual([]);
});

test("the customer cancels an order in review", async ({ page, request }) => {
  const id = await submittedOrder(request, "customer-f");
  await open(page, `/pedidos/${id}`, "customer-f");
  await expect(page.getByTestId("order-status")).toHaveText("Em revisão técnica");
  await page.getByRole("button", { name: "Cancelar pedido" }).click();
  await page.getByLabel("Motivo (opcional)").fill("e2e: comprei errado");
  await page.getByRole("dialog").getByRole("button", { name: "Cancelar pedido" }).click();
  await expect(page.getByTestId("order-status")).toHaveText("Cancelado");
  await expect(page.getByRole("button", { name: "Cancelar pedido" })).toHaveCount(0);
});

test("the address changes only with the code from the e-mail (C21)", async ({ page, request }) => {
  const errors = problems(page);
  const id = await approvedOrder(request, "customer-e");
  await panel(request, "support", `/panel/v1/orders/${id}/address-change`, {
    address: { recipient: "Customer E", street: "Rua Augusta", number: "500", city: "São Paulo", state: "SP", postal_code: "01304-000" },
    justification: "e2e: o cliente pediu a troca no chamado",
  });

  await open(page, `/pedidos/${id}`, "customer-e");
  await expect(page.getByTestId("proposed-address")).toContainText("Rua Augusta, 500");
  await expect(page.getByTestId("shipping-address")).toContainText("Avenida Paulista");

  const code = page.getByLabel("Código enviado ao seu e-mail");
  await code.fill("00000000");
  await page.getByRole("button", { name: "Confirmar troca" }).click();
  await expect(page.getByText(/Código incorreto/)).toBeVisible();

  await code.fill(await mailCode(request, "customer-e", id));
  await page.getByRole("button", { name: "Confirmar troca" }).click();
  await expect(page.getByText("Endereço de entrega alterado.", { exact: false })).toBeVisible();
  await expect(page.getByTestId("shipping-address")).toContainText("Rua Augusta");
  expect(errors).toEqual([]);
});

test("another customer's order is not found (D01)", async ({ page, request }) => {
  const id = await submittedOrder(request, "customer-e");
  await open(page, `/pedidos/${id}`, "customer-f");
  await expect(page.getByRole("heading", { name: "Pedido não encontrado" })).toBeVisible();
});

test("the orders on a phone", async ({ page, request }) => {
  const id = await submittedOrder(request, "customer-f");
  await page.setViewportSize({ width: 390, height: 844 });
  await open(page, "/pedidos", "customer-f");
  await expect(page.getByRole("link", { name: id.slice(0, 8) })).toBeVisible();
  await page.screenshot({ path: "e2e/results/screens/orders-phone.png", fullPage: true });
  await page.getByRole("link", { name: id.slice(0, 8) }).click();
  await expect(page.getByTestId("order-status")).toBeVisible();
  await page.screenshot({ path: "e2e/results/screens/order-phone.png", fullPage: true });
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.screenshot({ path: "e2e/results/screens/order-desktop.png", fullPage: true });
});
