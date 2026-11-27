// One order, from the STL upload to the delivery, recorded in six clips for
// the presentation (just demo, then just demo-gifs). The logins that would
// wait for a new TOTP step happen in a context that is not recorded, and its
// cookies are reused in the recorded one. It runs against the compose, in
// the order of the clips: each stage needs the state the previous one left.
import { writeFile } from "node:fs/promises";

import type { Browser, BrowserContext, Page } from "@playwright/test";
import { expect, test } from "@playwright/test";

import { apiToken, fillLogin } from "../e2e/login";
import { panelBase } from "../e2e/panel";
import { box } from "../e2e/stl";
import { showCursor } from "./cursor";

const out = "demo/out";
const viewport = { width: 1920, height: 1080 };
const customer = "customer-c";
const base = "https://localhost";

type State = Awaited<ReturnType<BrowserContext["storageState"]>>;

const context = (browser: Browser, extra: object = {}) =>
  browser.newContext({ ignoreHTTPSErrors: true, locale: "pt-BR", viewport, ...extra });

/** loginState logs in outside the recording, so the clip starts on the app. */
async function loginState(browser: Browser, url: string, user: string, staff: boolean): Promise<State> {
  const ctx = await context(browser);
  const page = await ctx.newPage();
  await page.goto(url);
  await fillLogin(page, user, staff);
  await page.waitForURL(url);
  const state = await ctx.storageState();
  await ctx.close();
  return state;
}

/** record opens a recorded context. mark names the chapter that starts now,
 * and finish writes the clip as <name>.webm and its chapters (seconds from
 * the start of the video, a tab, the title) as <name>.chapters. */
async function record(browser: Browser, name: string, state?: State) {
  const ctx = await context(browser, { recordVideo: { dir: `${out}/raw`, size: viewport }, ...(state === undefined ? {} : { storageState: state }) });
  await showCursor(ctx);
  const page = await ctx.newPage();
  const start = Date.now();
  const chapters: string[] = [];
  return {
    page,
    mark: (title: string) => {
      chapters.push(`${((Date.now() - start) / 1000).toFixed(2)}\t${title}`);
    },
    finish: async () => {
      const video = page.video();
      await ctx.close();
      await video?.saveAs(`${out}/${name}.webm`);
      await writeFile(`${out}/${name}.chapters`, `${chapters.join("\n")}\n`);
    },
  };
}

// A beat so each screen is readable in the GIF.
const beat = (page: Page, ms = 1500) => page.waitForTimeout(ms);

async function openOrder(page: Page, id: string, base: string) {
  const row = page.getByRole("row").filter({ hasText: id.slice(0, 8) });
  await row.getByRole("link", { name: id.slice(0, 8) }).click();
  await page.waitForURL(`${base}/pedidos/${id}`);
}

test.describe.configure({ mode: "serial" });

let orderId = "";

test("1. the customer uploads the part, quotes and sends it to review", async ({ browser, request }) => {
  const { page, mark, finish } = await record(browser, "01-cliente-pedido");
  mark("Cliente entra na conta");
  await page.goto(`${base}/orcar`);
  await fillLogin(page, customer, false);
  await page.waitForURL(`${base}/orcar`);
  await expect(page.getByRole("heading", { name: "Orçar uma peça" })).toBeVisible();
  await beat(page);

  mark("Upload do STL e visão 3D");
  await page.locator('input[type="file"]').setInputFiles({ name: "suporte.stl", mimeType: "model/stl", buffer: box(40, 25, 15) });
  await expect(page.getByTestId("dimensions")).toHaveText("40 × 25 × 15 mm");
  await expect(page.getByText("Pronto", { exact: true })).toBeVisible({ timeout: 30_000 });
  await beat(page, 2500);

  mark("Preenche o pedido");
  await page.getByRole("button", { name: "Aumentar" }).click();
  await page.getByLabel("Destinatário").fill("Customer C");
  await page.getByLabel("CEP").fill("01310100");
  await page.getByLabel("Logradouro").fill("Avenida Paulista");
  await page.getByLabel("Número").fill("1000");
  await page.getByLabel("Cidade").fill("São Paulo");
  await beat(page);
  mark("Orçamento calculado pelo servidor");
  await page.getByRole("button", { name: "Calcular orçamento" }).click();
  await expect(page.getByTestId("total")).toBeVisible();
  await beat(page, 2500);

  mark("Envia para a revisão técnica");
  await page.getByRole("button", { name: "Enviar para revisão técnica" }).click();
  await expect(page.getByRole("heading", { name: "Pedido enviado para a revisão técnica" })).toBeVisible();
  await beat(page, 2500);
  await finish();

  const token = await apiToken(request, customer);
  const res = await request.get(`${base}/api/v1/orders`, { headers: { Authorization: `Bearer ${token}` } });
  const list = (await res.json()) as { orders: { id: string; status: string }[] };
  const order = list.orders[0];
  expect(order?.status).toBe("awaiting_review");
  orderId = order?.id ?? "";
});

test("2. the admin assigns the order to a technician", async ({ browser }) => {
  const state = await loginState(browser, `${panelBase}/pedidos`, "admin", true);
  const { page, mark, finish } = await record(browser, "02-admin-atribui", state);
  mark("Administrador vê os pedidos em revisão");
  await page.goto(`${panelBase}/pedidos`);
  await expect(page.getByRole("heading", { name: "Pedidos" })).toBeVisible();
  await beat(page);
  await page.getByRole("radio", { name: "Em revisão", exact: true }).check();
  await beat(page);
  await openOrder(page, orderId, panelBase);
  await expect(page.getByTestId("order-status")).toHaveText("Em revisão técnica");
  await beat(page, 2000);
  mark("Atribui o pedido ao técnico");
  await page.getByLabel("Técnico").selectOption({ label: "technician" });
  await page.getByRole("button", { name: "Atribuir" }).click();
  await expect(page.getByTestId("assigned-technician")).toHaveText("Atribuído a technician.");
  await beat(page, 2500);
  await finish();
});

test("3. the technician opens the design, reviews and approves", async ({ browser }) => {
  const state = await loginState(browser, `${panelBase}/pedidos`, "technician", true);
  const { page, mark, finish } = await record(browser, "03-tecnico-revisao", state);
  mark("Técnico recebe o pedido");
  await page.goto(`${panelBase}/pedidos`);
  await expect(page.getByRole("heading", { name: "Meus pedidos" })).toBeVisible();
  await beat(page);
  await openOrder(page, orderId, panelBase);
  await expect(page.getByTestId("shipping-address")).toHaveText("Endereço visível só para suporte e administração");
  await expect(page.getByTestId("order-dimensions")).toHaveText("40 × 25 × 15 mm");
  await beat(page, 2500);

  mark("Abre o projeto com justificativa");
  await page.getByRole("button", { name: "Abrir projeto" }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel("Justificativa").fill("Conferir a espessura das paredes");
  await beat(page);
  await dialog.getByRole("button", { name: "Abrir projeto" }).click();
  await expect(page.getByTestId("dimensions")).toHaveText("40 × 25 × 15 mm");
  await beat(page, 3000);

  mark("Faz a revisão e aprova");
  await page.getByRole("radio", { name: "Aprovar" }).check();
  await page.getByLabel("Laudo").fill("Paredes de 2 mm, cabe na impressora. Aprovado.");
  await beat(page);
  await page.getByRole("button", { name: "Registrar revisão" }).click();
  await expect(page.getByTestId("order-status")).toHaveText("Aguardando pagamento");
  await beat(page, 2500);
  await finish();
});

test("4. the customer pays", async ({ browser }) => {
  const state = await loginState(browser, `${base}/pedidos`, customer, false);
  const { page, mark, finish } = await record(browser, "04-cliente-paga", state);
  mark("Cliente paga o pedido aprovado");
  await page.goto(`${base}/pedidos`);
  const row = page.getByRole("row").filter({ hasText: orderId.slice(0, 8) });
  await expect(row.getByText("Aguardando pagamento")).toBeVisible();
  await beat(page);
  await openOrder(page, orderId, base);
  await beat(page, 2000);
  await page.getByRole("button", { name: /^Pagar R\$/ }).click();
  await expect(page.getByTestId("order-status")).toHaveText("Pago", { timeout: 20_000 });
  await beat(page, 2500);
  await finish();
});

test("5. the technician starts the production and schedules the pickup", async ({ browser }) => {
  const state = await loginState(browser, `${panelBase}/pedidos`, "technician", true);
  const { page, mark, finish } = await record(browser, "05-tecnico-producao", state);
  mark("Técnico inicia a produção");
  await page.goto(`${panelBase}/pedidos/${orderId}`);
  await expect(page.getByTestId("order-status")).toHaveText("Pago");
  await beat(page, 2000);
  await page.getByRole("button", { name: "Iniciar produção" }).click();
  await expect(page.getByTestId("order-status")).toHaveText("Em produção");
  await beat(page, 2000);
  mark("Agenda a coleta");
  await page.getByRole("button", { name: "Agendar coleta" }).click();
  await expect(page.getByTestId("tracking")).toHaveText(/^NG[0-9A-F]+BR$/);
  await beat(page, 2500);
  await finish();
});

test("6. the customer follows the order until the delivery", async ({ browser }) => {
  const state = await loginState(browser, `${base}/pedidos`, customer, false);
  const { page, mark, finish } = await record(browser, "06-cliente-entrega", state);
  mark("Cliente acompanha até a entrega");
  await page.goto(`${base}/pedidos/${orderId}`);
  await expect(page.getByTestId("order-status")).toBeVisible();
  await beat(page, 2000);
  // The carrier's webhooks move the order; the page follows by itself.
  await expect(page.getByTestId("order-status")).toHaveText("Entregue", { timeout: 60_000 });
  await expect(page.getByRole("list", { name: "Andamento do pedido" }).getByRole("listitem")).toHaveCount(7);
  await beat(page, 3500);
  await finish();
});
