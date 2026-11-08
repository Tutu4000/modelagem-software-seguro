// Login helpers for the end-to-end tests: the Keycloak forms as a person
// fills them, and the TOTP of the staff computed like internal/devlogin does.
import { createHmac } from "node:crypto";
import { readFileSync } from "node:fs";

import type { Page } from "@playwright/test";

const secrets = process.env.SECRETS_DIR ?? "../secrets";

export function secret(name: string): string {
  return readFileSync(`${secrets}/${name}`, "utf8").trim();
}

/** totpAt is RFC 6238 with a 30-second step and the secret's bytes as the
 * HMAC key, the realm's policy. */
export function totpAt(key: string, unixSeconds: number): string {
  const counter = Buffer.alloc(8);
  counter.writeBigUInt64BE(BigInt(Math.floor(unixSeconds / 30)));
  const sum = createHmac("sha1", Buffer.from(key)).update(counter).digest();
  const off = (sum.at(-1) ?? 0) & 0x0f;
  return String((sum.readUInt32BE(off) & 0x7fffffff) % 1_000_000).padStart(6, "0");
}

let lastStep = 0;

/** totp never reuses a step: Keycloak refuses a code used twice. */
export async function totp(): Promise<string> {
  let now = Date.now() / 1000;
  if (Math.floor(now / 30) <= lastStep) {
    await new Promise((r) => setTimeout(r, ((lastStep + 1) * 30 - now) * 1000 + 200));
    now = Date.now() / 1000;
  }
  lastStep = Math.floor(now / 30);
  return totpAt(secret("keycloak_totp_secret"), now);
}

/** fillLogin answers the Keycloak pages on the given page: the password and,
 * for the staff, the TOTP. */
export async function fillLogin(page: Page, username: string, staff: boolean): Promise<void> {
  // A re-authentication (step-up) already knows the user: only the password.
  await page.locator("#password").waitFor();
  if (await page.locator("#username").isVisible()) {
    await page.locator("#username").fill(username);
  }
  await page.locator("#password").fill(secret("keycloak_demo_password"));
  await page.locator("#kc-login").click();
  if (staff) {
    await page.locator("#otp").waitFor();
    await page.locator("#otp").fill(await totp());
    await page.locator("#kc-login").click();
  }
}

/** noTokensInStorage: nothing that looks like a JWT in web storage. */
export async function storedTokens(page: Page): Promise<string> {
  return page.evaluate(() => {
    const dump = (s: Storage) => Object.keys(s).map((k) => `${k}=${s.getItem(k) ?? ""}`).join("\n");
    return [dump(window.localStorage), dump(window.sessionStorage)].join("\n");
  });
}
