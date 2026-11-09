// Abuse cases in the browser that the other specs do not cover. Already
// covered elsewhere: markup in the customer's own ticket (support.spec),
// another customer's order and ticket (orders.spec, support.spec, D01), a
// technician on someone else's order (panel-orders.spec), a customer in the
// panel (auth.spec) and the security headers and CORS (headers.spec).
import { expect, test, type APIRequestContext } from "@playwright/test";

import { apiToken, secret } from "./login";
import { panelBase, staffPage } from "./panel";
import { panel, staffToken } from "./setup";

// Every staff login waits for a fresh TOTP step.
test.describe.configure({ timeout: 5 * 60 * 1000 });

const payloads = [
  `<img src=x onerror="document.title='xss'">`,
  `<script>document.title='xss'</script>`,
  `[clique](javascript:document.title='xss')`,
  `<svg onload="document.title='xss'"></svg>`,
];

async function customerTicket(request: APIRequestContext, customer: string, subject: string, message: string): Promise<string> {
  const res = await request.post("https://localhost/api/v1/tickets", {
    headers: { Authorization: `Bearer ${await apiToken(request, customer)}` },
    data: { subject, message },
  });
  expect(res.ok(), await res.text()).toBe(true);
  return ((await res.json()) as { id: string }).id;
}

test("stored XSS aimed at the staff stays text in the panel (E8.6)", async ({ browser, request }) => {
  const subject = `<b onmouseover="document.title='xss'">urgente</b>`;
  const ticket = await customerTicket(request, "customer-b", subject, payloads.join("\n"));

  const { page, errors } = await staffPage(browser, "support", "/chamados");
  const dialogs: string[] = [];
  page.on("dialog", (d) => { dialogs.push(d.message()); void d.dismiss(); });
  // The subject in the queue is text, not a bold element.
  await expect(page.getByRole("cell", { name: subject })).toBeVisible();
  await page.getByRole("link", { name: ticket.slice(0, 8) }).click();
  await page.waitForURL(`${panelBase}/chamados/${ticket}`);

  await expect(page.getByRole("heading", { level: 1 })).toHaveText(subject);
  const thread = page.getByRole("list", { name: "Mensagens do chamado" });
  for (const p of payloads) {
    await expect(thread).toContainText(p);
  }
  await expect(page.locator("main").locator("img, script, svg[onload], b, a[href^='javascript']")).toHaveCount(0);
  await page.getByRole("heading", { level: 1 }).hover();
  expect(await page.title()).not.toBe("xss");
  expect(dialogs).toEqual([]);
  expect(errors).toEqual([]);
  await panel(request, "support", `/panel/v1/tickets/${ticket}/close`);
});

// The realm never issues a staff token without the TOTP: the password grant
// is refused for the staff (Go e2e TestStaffNeedsTOTPAndCannotUseCustomerAPI),
// and trying it here would count toward the account's brute-force lockout.
// A customer token is for the public API's audience: the panel's verifier
// refuses it as unauthenticated (401) before it looks at the role.
test("the panel API refuses a customer token and no token, from the panel's origin (D02)", async ({ page, request }) => {
  const customer = await apiToken(request, "customer-b");
  await page.goto(`${panelBase}/auth/callback`);
  const call = (token: string | undefined) =>
    page.evaluate(async (t) => {
      const res = await fetch("/panel/v1/me", { headers: t === undefined ? {} : { Authorization: `Bearer ${t}` } });
      return res.status;
    }, token);
  expect(await call(customer)).toBe(401);
  expect(await call(undefined)).toBe(401);
});

test("the password alone stops at the TOTP screen, with no token", async ({ page }) => {
  await page.goto(`${panelBase}/`);
  await page.locator("#username").fill("support");
  await page.locator("#password").fill(secret("keycloak_demo_password"));
  await page.locator("#kc-login").click();
  await expect(page.locator("#otp")).toBeVisible();
  expect(page.url().startsWith("https://localhost:8444/")).toBe(true);
  // Going back to the panel without the code still asks for a login.
  await page.goto(`${panelBase}/pedidos`);
  await page.waitForURL(/^https:\/\/localhost:8444\//);
});

test("a made-up or malformed ID in the panel's URL shows not found", async ({ browser }) => {
  const { page, errors } = await staffPage(browser, "admin", "/");
  for (const path of ["/pedidos/0b7e3a52-0000-4000-8000-00000000dead", "/pedidos/not-a-uuid", "/chamados/0b7e3a52-0000-4000-8000-00000000dead", "/chamados/1%20OR%201=1"]) {
    await page.goto(`${panelBase}${path}`);
    await expect(page.getByRole("heading", { level: 1 })).toHaveText(/não encontrado/);
  }
  // A path that climbs above the root does not even reach the app: nginx
  // refuses it while normalizing the URL.
  const climb = await page.goto(`${panelBase}/pedidos/..%2F..%2F..%2Fpanel%2Fv1%2Fme`);
  expect(climb?.status()).toBe(400);
  expect(errors).toEqual([]);
});

test("a technician gets nothing from the support's queue", async ({ browser }) => {
  const { page } = await staffPage(browser, "technician-2", "/chamados");
  await expect(page.getByText("Não foi possível carregar os chamados.")).toBeVisible();
  await expect(page.getByRole("table")).toHaveCount(0);
});

test("a critical operation without a justification is refused by the API too", async ({ request }) => {
  const res = await request.post(`${panelBase}/panel/v1/users/0b7e3a52-5f1d-4c3e-9a51-7f6a1c2d0d04/revoke`, {
    headers: { Authorization: `Bearer ${await staffToken("admin")}` },
    data: {},
  });
  expect(res.status()).toBe(422);
});
