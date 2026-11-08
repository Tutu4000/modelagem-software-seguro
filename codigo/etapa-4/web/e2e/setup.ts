// Preparing orders through the APIs, so each test starts where it matters:
// the customer's part with the e2e client token, the staff's with a panel
// token from the browser flow (PKCE, password and TOTP), like
// internal/devlogin.BrowserLogin.
import { createHash, randomBytes } from "node:crypto";

import type { CustomerComponents } from "@nugget/api";
import { request as playwrightRequest, type APIRequestContext } from "@playwright/test";

import { apiToken, secret, totp } from "./login";
import { box } from "./stl";

type Order = CustomerComponents["schemas"]["Order"];

const issuer = "https://localhost:8444/realms/nugget";
const panelRedirect = "https://localhost:9445/auth/callback";
export const technicianId = "0b7e3a52-5f1d-4c3e-9a51-7f6a1c2d1a01";

const b64url = (b: Buffer) => b.toString("base64url");

function formAction(html: string): string {
  const tag = /<form[^>]*>/.exec(html)?.[0] ?? "";
  const action = /action="([^"]+)"/.exec(tag)?.[1];
  if (action === undefined) {
    throw new Error("login form without action");
  }
  return action.replaceAll("&amp;", "&");
}

const staffCache = new Map<string, { token: string; at: number }>();

/** staffToken logs a staff account in to the panel as a browser would, and
 * keeps the token for 4 minutes: every TOTP code costs a 30-second step, and
 * the critical operations accept a login of up to 5 minutes. */
export async function staffToken(username: string): Promise<string> {
  const cached = staffCache.get(username);
  if (cached !== undefined && Date.now() - cached.at < 4 * 60 * 1000) {
    return cached.token;
  }
  const ctx = await playwrightRequest.newContext({ ignoreHTTPSErrors: true });
  try {
    const verifier = b64url(randomBytes(32));
    const challenge = b64url(createHash("sha256").update(verifier).digest());
    const query = new URLSearchParams({
      client_id: "panel-web", response_type: "code", scope: "openid", redirect_uri: panelRedirect,
      state: b64url(randomBytes(12)), nonce: b64url(randomBytes(12)), code_challenge: challenge, code_challenge_method: "S256", acr_values: "2",
    });
    let res = await ctx.get(`${issuer}/protocol/openid-connect/auth?${query.toString()}`, { maxRedirects: 0 });
    for (let hop = 0; hop < 10; hop++) {
      const location = res.headers().location;
      if (location !== undefined) {
        if (location.startsWith(panelRedirect)) {
          const code = new URL(location).searchParams.get("code") ?? "";
          const tokens = await ctx.post(`${issuer}/protocol/openid-connect/token`, {
            form: { grant_type: "authorization_code", client_id: "panel-web", code, redirect_uri: panelRedirect, code_verifier: verifier },
          });
          const body: unknown = await tokens.json();
          if (typeof body !== "object" || body === null || !("access_token" in body) || typeof body.access_token !== "string") {
            throw new Error(`no panel token for ${username}`);
          }
          staffCache.set(username, { token: body.access_token, at: Date.now() });
          return body.access_token;
        }
        res = await ctx.get(location, { maxRedirects: 0 });
        continue;
      }
      const page = await res.text();
      const form = page.includes('name="password"')
        ? { username, password: secret("keycloak_demo_password"), credentialId: "" }
        : page.includes('name="otp"')
          ? { otp: await totp() }
          : undefined;
      if (form === undefined) {
        throw new Error(`login of ${username}: unexpected page ${String(res.status())}`);
      }
      res = await ctx.post(formAction(page), { form, maxRedirects: 0 });
    }
    throw new Error(`login of ${username}: too many steps`);
  } finally {
    await ctx.dispose();
  }
}

const shipping = { recipient: "Customer Test", street: "Avenida Paulista", number: "1000", city: "São Paulo", state: "SP", postal_code: "01310-100" };

async function ok(res: Awaited<ReturnType<APIRequestContext["get"]>>, what: string): Promise<unknown> {
  if (!res.ok()) {
    throw new Error(`${what}: ${String(res.status())} ${await res.text()}`);
  }
  return res.json();
}

/** submittedOrder uploads a box, waits for the processing, quotes and sends
 * it to review, as the customer. */
export async function submittedOrder(request: APIRequestContext, customer: string, size: [number, number, number] = [10, 20, 30]): Promise<string> {
  const auth = { Authorization: `Bearer ${await apiToken(request, customer)}` };
  const file = (await ok(await request.post("https://localhost/api/v1/files", { headers: { ...auth, "Content-Type": "model/stl" }, data: box(...size) }), "upload")) as { id: string };
  for (let i = 0; i < 60; i++) {
    const st = (await ok(await request.get(`https://localhost/api/v1/files/${file.id}`, { headers: auth }), "status")) as { status: string };
    if (st.status === "done") {
      break;
    }
    await new Promise((r) => setTimeout(r, 1000));
  }
  const order = (await ok(
    await request.post("https://localhost/api/v1/orders", {
      headers: auth,
      data: { file_id: file.id, material: "pla", color: "black", quality: "standard", finishing: "none", quantity: 1, shipping },
    }),
    "quote",
  )) as Order;
  await ok(await request.post(`https://localhost/api/v1/orders/${order.id}/submit`, { headers: auth }), "submit");
  return order.id;
}

export async function panel(request: APIRequestContext, staff: string, path: string, data?: object): Promise<unknown> {
  const res = await request.post(`https://localhost:9445${path}`, {
    headers: { Authorization: `Bearer ${await staffToken(staff)}` },
    ...(data === undefined ? {} : { data }),
  });
  return ok(res, `${staff} ${path}`);
}

/** approvedOrder takes a new order to awaiting_payment: the admin assigns
 * it and the technician approves it, through the panel API. */
export async function approvedOrder(request: APIRequestContext, customer: string): Promise<string> {
  const id = await submittedOrder(request, customer);
  await panel(request, "admin", `/panel/v1/orders/${id}/assign`, { technician_id: technicianId });
  await panel(request, "technician", `/panel/v1/orders/${id}/review`, { decision: "approved", report: "e2e: paredes adequadas, cabe na impressora" });
  return id;
}

/** mailCode reads the address-change code from the customer's Mailpit inbox. */
export async function mailCode(request: APIRequestContext, customer: string, orderId: string): Promise<string> {
  const subject = `Confirme a troca de endereço do pedido ${orderId.slice(0, 8)}`;
  for (let i = 0; i < 20; i++) {
    const query = new URLSearchParams({ query: `to:"${customer}@nugget.local" subject:"${subject}"` });
    const found = (await (await request.get(`http://127.0.0.1:8026/api/v1/search?${query.toString()}`)).json()) as { messages: { ID: string }[] };
    const first = found.messages[0];
    if (first !== undefined) {
      const msg = (await (await request.get(`http://127.0.0.1:8026/api/v1/message/${first.ID}`)).json()) as { Text: string };
      const code = /código (\d{8})/.exec(msg.Text)?.[1];
      if (code !== undefined) {
        return code;
      }
    }
    await new Promise((r) => setTimeout(r, 1000));
  }
  throw new Error("no address change code in Mailpit");
}
