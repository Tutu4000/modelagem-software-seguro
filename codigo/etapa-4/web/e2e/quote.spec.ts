import type { CustomerComponents } from "@nugget/api";
import { expect, test, type Page } from "@playwright/test";

import { apiToken, fillLogin } from "./login";
import { box } from "./stl";

/** problems collects console errors of our pages: CSP and Trusted Types
 * violations included, which the worker and three.js must not cause. */
function problems(page: Page): string[] {
  const found: string[] = [];
  page.on("console", (m) => {
    if (m.type() === "error" && !m.location().url.startsWith("https://localhost:8444")) {
      found.push(`${m.location().url}: ${m.text()}`);
    }
  });
  page.on("pageerror", (e) => found.push(e.message));
  return found;
}

async function openQuote(page: Page, username: string) {
  await page.goto("https://localhost/orcar");
  await fillLogin(page, username, false);
  await page.waitForURL("https://localhost/orcar");
  await expect(page.getByRole("heading", { name: "Orçar uma peça" })).toBeVisible();
}

async function upload(page: Page, name: string, data: Buffer) {
  await page.locator('input[type="file"]').setInputFiles({ name, mimeType: "model/stl", buffer: data });
}

async function fillAddress(page: Page) {
  await page.getByLabel("Destinatário").fill("Customer E");
  await page.getByLabel("CEP").fill("01310100");
  await page.getByLabel("Logradouro").fill("Avenida Paulista");
  await page.getByLabel("Número").fill("1000");
  await page.getByLabel("Cidade").fill("São Paulo");
}

test("the customer uploads, sees the part in 3D, quotes and sends to review", async ({ page, request }) => {
  const errors = problems(page);
  await openQuote(page, "customer-e");
  await upload(page, "bracket.stl", box(10, 20, 30));
  await expect(page.getByTestId("dimensions")).toHaveText("10 × 20 × 30 mm");
  await expect(page.getByRole("img", { name: /bracket\.stl/ })).toBeVisible();
  await expect(page.getByText("Pronto", { exact: true })).toBeVisible({ timeout: 30_000 });
  await expect(page.getByTestId("server-dimensions")).toHaveText("10 × 20 × 30 mm");

  await page.getByRole("button", { name: "Aumentar" }).click();
  await fillAddress(page);
  await page.getByRole("button", { name: "Calcular orçamento" }).click();
  const total = page.getByTestId("total");
  await expect(total).toBeVisible();
  const shown = (await total.textContent()) ?? "";

  // The total on screen is the server's, for the order the app created.
  const token = await apiToken(request, "customer-e");
  const res = await request.get("https://localhost/api/v1/orders", { headers: { Authorization: `Bearer ${token}` } });
  // The API's own contract types the answer.
  const list = (await res.json()) as { orders: CustomerComponents["schemas"]["Order"][] };
  const order = list.orders[0];
  expect(order?.status).toBe("quoted");
  expect(order?.quantity).toBe(2);
  const cents = order?.total_cents ?? 0;
  expect(shown.replace(/\s/g, " ")).toBe(new Intl.NumberFormat("pt-BR", { style: "currency", currency: "BRL" }).format(cents / 100).replace(/\s/g, " "));

  await page.getByRole("button", { name: "Enviar para revisão técnica" }).click();
  await expect(page.getByRole("heading", { name: "Pedido enviado para a revisão técnica" })).toBeVisible();
  const after = await request.get(`https://localhost/api/v1/orders/${order?.id ?? ""}`, { headers: { Authorization: `Bearer ${token}` } });
  expect(((await after.json()) as CustomerComponents["schemas"]["Order"]).status).toBe("awaiting_review");
  expect(errors).toEqual([]);
});

test("a part bigger than the printer shows the DFM warning", async ({ page }) => {
  await openQuote(page, "customer-f");
  await upload(page, "beam.stl", box(320, 20, 20));
  await expect(page.getByText("Pronto", { exact: true })).toBeVisible({ timeout: 30_000 });
  await fillAddress(page);
  await page.getByRole("button", { name: "Calcular orçamento" }).click();
  await expect(page.getByText(/maior que a área de impressão/)).toBeVisible();
});

test("an ASCII STL is refused in the browser, with no upload", async ({ page }) => {
  await openQuote(page, "customer-f");
  let uploads = 0;
  page.on("request", (r) => {
    if (r.method() === "POST" && r.url().endsWith("/api/v1/files")) {
      uploads++;
    }
  });
  await upload(page, "part.stl", Buffer.from("solid part\n facet normal 0 0 1\n  outer loop\n   vertex 0 0 0\n".repeat(20)));
  await expect(page.getByText(/não é um STL binário/)).toBeVisible();
  expect(uploads).toBe(0);
});

test("a reload resumes from the uploaded file, shown through its signed URL", async ({ page }) => {
  const errors = problems(page);
  await openQuote(page, "customer-e");
  await upload(page, "plate.stl", box(40, 40, 5));
  await expect(page.getByText("Pronto", { exact: true })).toBeVisible({ timeout: 30_000 });
  expect(new URL(page.url()).searchParams.get("arquivo")).toMatch(/^[0-9a-f-]{36}$/);
  await page.reload();
  await expect(page.getByText("Pronto", { exact: true })).toBeVisible();
  await expect(page.getByTestId("dimensions")).toHaveText("40 × 40 × 5 mm", { timeout: 15_000 });
  expect(errors).toEqual([]);
});

test("the quote works on a phone", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await openQuote(page, "customer-f");
  await upload(page, "clip.stl", box(15, 10, 5));
  await expect(page.getByText("Pronto", { exact: true })).toBeVisible({ timeout: 30_000 });
  await fillAddress(page);
  await page.getByRole("button", { name: "Calcular orçamento" }).click();
  await expect(page.getByTestId("total")).toBeVisible();
  await page.screenshot({ path: "e2e/results/screens/quote-phone.png", fullPage: true });
});
