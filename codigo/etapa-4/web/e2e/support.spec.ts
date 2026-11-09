import { expect, test, type Page } from "@playwright/test";

import { fillLogin } from "./login";
import { problems } from "./problems";
import { panel, submittedOrder } from "./setup";

async function open(page: Page, path: string, customer: string) {
  await page.goto(`https://localhost${path}`);
  await fillLogin(page, customer, false);
  await page.waitForURL(`https://localhost${path}`);
}

async function newTicket(page: Page, subject: string, message: string): Promise<string> {
  await page.getByLabel("Assunto").fill(subject);
  await page.getByLabel("Mensagem").fill(message);
  await page.getByRole("button", { name: "Enviar" }).click();
  await page.waitForURL(/\/suporte\/[0-9a-f-]{36}$/);
  return page.url().split("/").at(-1) ?? "";
}

test.describe.configure({ timeout: 4 * 60 * 1000 });

test("the customer opens a ticket about an order and talks to the support", async ({ page, request }) => {
  const errors = problems(page);
  const orderId = await submittedOrder(request, "customer-e");
  await open(page, `/pedidos/${orderId}`, "customer-e");
  await page.getByRole("link", { name: "Abrir chamado sobre este pedido" }).click();
  await page.waitForURL(/\/suporte\/novo\?pedido=/);
  await expect(page.getByLabel("Pedido", { exact: true })).toHaveValue(orderId);
  const id = await newTicket(page, "Prazo de entrega", "Quando a peça fica pronta?\nPreciso até sexta.");
  await expect(page.getByTestId("ticket-status")).toHaveText("Aguardando o suporte");
  await expect(page.getByRole("link", { name: orderId.slice(0, 8) })).toBeVisible();

  await panel(request, "support", `/panel/v1/tickets/${id}/messages`, { message: "Sai amanhã pela transportadora." });
  // The page asks every 20 seconds while the ticket is open.
  await expect(page.getByText("Sai amanhã pela transportadora.")).toBeVisible({ timeout: 30_000 });
  await expect(page.getByTestId("ticket-status")).toHaveText("Respondido");

  await page.getByLabel("Responder").fill("Obrigado!");
  await page.getByRole("button", { name: "Enviar" }).click();
  await expect(page.getByText("Obrigado!")).toBeVisible();
  await expect(page.getByTestId("ticket-status")).toHaveText("Aguardando o suporte");

  await panel(request, "support", `/panel/v1/tickets/${id}/close`);
  await page.reload();
  await expect(page.getByTestId("ticket-status")).toHaveText("Fechado");
  await expect(page.getByLabel("Responder")).toHaveCount(0);
  expect(errors).toEqual([]);
});

test("markup in a message stays text (stored XSS)", async ({ page }) => {
  const errors = problems(page);
  await open(page, "/suporte/novo", "customer-f");
  const payload = `<img src=x onerror="document.title='xss'"><script>document.title='xss'</script>`;
  await newTicket(page, "<b>assunto</b>", payload);
  const thread = page.getByRole("list", { name: "Mensagens do chamado" });
  await expect(thread.getByText(payload)).toBeVisible();
  await expect(thread.locator("img, script, b")).toHaveCount(0);
  await expect(page.getByRole("heading", { level: 1 })).toHaveText("<b>assunto</b>");
  expect(await page.title()).not.toBe("xss");
  expect(errors).toEqual([]);
});

test("a control character is refused before the API", async ({ page }) => {
  await open(page, "/suporte/novo", "customer-f");
  let calls = 0;
  page.on("request", (r) => {
    if (r.method() === "POST" && r.url().endsWith("/api/v1/tickets")) {
      calls++;
    }
  });
  await page.getByLabel("Assunto").fill("Teste");
  await page.getByLabel("Mensagem").fill("sino \u0007 aqui");
  await page.getByRole("button", { name: "Enviar" }).click();
  await expect(page.getByText(/caractere de controle/)).toBeVisible();
  expect(calls).toBe(0);
});

test("another customer's ticket is not found (D01)", async ({ page, browser }) => {
  await open(page, "/suporte/novo", "customer-e");
  const id = await newTicket(page, "Particular", "Só meu.");
  const other = await browser.newPage();
  await open(other, `/suporte/${id}`, "customer-f");
  await expect(other.getByRole("heading", { name: "Chamado não encontrado" })).toBeVisible();
  await other.close();
});

test("support on a phone", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await open(page, "/suporte/novo", "customer-f");
  await newTicket(page, "Endereço com URL longa", `Veja https://example.com/${"caminho-muito-longo/".repeat(8)} e me diga.`);
  await expect(page.getByTestId("ticket-status")).toBeVisible();
  await page.screenshot({ path: "e2e/results/screens/ticket-phone.png", fullPage: true });
  await page.goto("https://localhost/suporte");
  await page.screenshot({ path: "e2e/results/screens/tickets-phone.png", fullPage: true });
});
