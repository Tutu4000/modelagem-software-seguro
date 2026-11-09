// Basic accessibility of both apps: axe with the WCAG 2.1 A and AA rules on
// the main screens, and the keyboard paths (skip link, phone menu, dialog,
// forms). axe does not replace a test with a real screen reader.
import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Locator, type Page } from "@playwright/test";

import { apiToken, fillLogin } from "./login";
import { panelBase, staffPage } from "./panel";
import { panel, submittedOrder } from "./setup";

const wcag = ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"];

/** violations runs axe on the page as it is and lists each rule broken,
 * with the elements, so a failure says what to fix. */
async function violations(page: Page): Promise<string[]> {
  const result = await new AxeBuilder({ page }).withTags(wcag).analyze();
  return result.violations.map((v) => `${v.id} (${v.impact ?? "?"}): ${v.nodes.map((n) => n.target.join(" ")).join(", ")}`);
}

/** settled waits for the page to fill in after the API answers: no
 * skeleton left. */
async function settled(page: Page) {
  await expect(page.locator("[class*='skeleton']")).toHaveCount(0);
}

/** tabTo presses Tab until the element has the focus, as a keyboard user
 * would, and fails if it never gets there. */
async function tabTo(page: Page, target: Locator, max = 40) {
  for (let i = 0; i < max; i++) {
    await page.keyboard.press("Tab");
    if (await target.evaluate((el) => el === document.activeElement)) {
      return;
    }
  }
  throw new Error(`Tab never reached ${target.toString()}`);
}

async function ticketOf(request: Parameters<typeof apiToken>[0], customer: string, orderId: string): Promise<string> {
  const res = await request.post("https://localhost/api/v1/tickets", {
    headers: { Authorization: `Bearer ${await apiToken(request, customer)}` },
    data: { order_id: orderId, subject: "e2e: acessibilidade", message: "Uma pergunta sobre o pedido." },
  });
  expect(res.ok()).toBe(true);
  return ((await res.json()) as { id: string }).id;
}

// The staff login waits for a fresh TOTP step.
test.describe.configure({ timeout: 5 * 60 * 1000 });

test("the customer app passes axe on the main screens", async ({ page, request }) => {
  const order = await submittedOrder(request, "customer-a");
  const ticket = await ticketOf(request, "customer-a", order);

  await page.goto("https://localhost/conta");
  await fillLogin(page, "customer-a", false);
  await page.waitForURL("https://localhost/conta");
  const found: Record<string, string[]> = {};
  for (const path of ["/", "/orcar", "/pedidos", `/pedidos/${order}`, "/suporte", `/suporte/${ticket}`, "/suporte/novo", "/conta"]) {
    await page.goto(`https://localhost${path}`);
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
    await settled(page);
    found[path] = await violations(page);
  }
  expect(found).toEqual(Object.fromEntries(Object.keys(found).map((k) => [k, []])));
  await panel(request, "support", `/panel/v1/tickets/${ticket}/close`);
});

test("the panel passes axe on the main screens", async ({ browser, request }) => {
  const order = await submittedOrder(request, "customer-a");
  const ticket = await ticketOf(request, "customer-a", order);

  const { page } = await staffPage(browser, "admin", "/");
  const found: Record<string, string[]> = {};
  for (const path of ["/", "/pedidos", `/pedidos/${order}`, "/chamados", `/chamados/${ticket}`, "/precos", "/precos/nova", "/revogar"]) {
    await page.goto(`${panelBase}${path}`);
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
    await settled(page);
    found[path] = await violations(page);
  }
  expect(found).toEqual(Object.fromEntries(Object.keys(found).map((k) => [k, []])));
  await panel(request, "support", `/panel/v1/tickets/${ticket}/close`);
});

test("the keyboard skips the bar, works the phone menu and answers a ticket", async ({ page, request }) => {
  const order = await submittedOrder(request, "customer-a");
  const ticket = await ticketOf(request, "customer-a", order);
  await page.goto(`https://localhost/suporte/${ticket}`);
  await fillLogin(page, "customer-a", false);
  await page.waitForURL(`https://localhost/suporte/${ticket}`);
  await expect(page.getByRole("heading", { level: 1 })).toHaveText("e2e: acessibilidade");

  // The first Tab is the skip link; it takes the focus to the content.
  await page.keyboard.press("Tab");
  const skip = page.getByRole("link", { name: "Pular para o conteúdo" });
  await expect(skip).toBeFocused();
  await expect(skip).toBeVisible();
  await page.keyboard.press("Enter");
  await expect(page.locator("main")).toBeFocused();

  // The answer, typed and sent with the keyboard only.
  await tabTo(page, page.getByLabel("Responder"));
  await page.keyboard.type("Resposta pelo teclado.");
  await tabTo(page, page.getByRole("button", { name: "Enviar" }));
  await page.keyboard.press("Enter");
  await expect(page.getByRole("list", { name: "Mensagens do chamado" }).getByText("Resposta pelo teclado.")).toBeVisible();

  // The phone menu: opened with Enter, closed with Esc, focus back on its
  // button.
  await page.setViewportSize({ width: 390, height: 844 });
  const menu = page.getByRole("button", { name: "Abrir menu" });
  await menu.focus();
  await page.keyboard.press("Enter");
  await expect(page.getByRole("navigation", { name: "Menu" })).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("navigation", { name: "Menu" })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Abrir menu" })).toBeFocused();
  await panel(request, "support", `/panel/v1/tickets/${ticket}/close`);
});

test("the critical dialog keeps the focus and closes with Esc", async ({ browser }) => {
  const { page } = await staffPage(browser, "admin", "/revogar?usuario=0b7e3a52-5f1d-4c3e-9a51-7f6a1c2d0d04");
  await page.getByRole("button", { name: "Revogar", exact: true }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByLabel("Justificativa")).toBeFocused();
  // The page behind is inert: Tab cycles through the dialog and, past its
  // end, the browser's own bar (the page then sees the body), never a link
  // or a field of the page.
  for (let i = 0; i < 8; i++) {
    await page.keyboard.press("Tab");
    const where = await page.evaluate(() => {
      const el = document.activeElement;
      return el === null || el === document.body ? "browser" : el.closest("dialog") !== null ? "dialog" : el.outerHTML.slice(0, 80);
    });
    expect(["dialog", "browser"]).toContain(where);
  }
  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
  // Nothing was sent: the account is untouched.
  await expect(page.getByTestId("revoke-result")).toHaveCount(0);
});
