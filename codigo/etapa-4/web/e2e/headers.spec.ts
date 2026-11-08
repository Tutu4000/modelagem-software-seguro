import { expect, request, test } from "@playwright/test";

const appCSP =
  "default-src 'none'; script-src 'self'; style-src 'self'; font-src 'self'; img-src 'self' data: blob:; connect-src 'self' https://localhost:8444 https://localhost:8443; worker-src 'self' blob:; manifest-src 'self'; form-action 'self' https://localhost:8444; base-uri 'none'; frame-ancestors 'none'; object-src 'none'; require-trusted-types-for 'script'; trusted-types 'none'";
const jsonCSP = "default-src 'none'; frame-ancestors 'none'";

const common: Record<string, string> = {
  "strict-transport-security": "max-age=31536000",
  "x-content-type-options": "nosniff",
  "referrer-policy": "no-referrer",
  "permissions-policy": "camera=(), microphone=(), geolocation=(), payment=(), usb=(), interest-cohort=()",
};

interface Case {
  url: string;
  extra: Record<string, string>;
}

const cases: Case[] = [
  { url: "https://localhost/", extra: { "content-security-policy": appCSP, "cross-origin-opener-policy": "same-origin", "cross-origin-resource-policy": "same-origin" } },
  { url: "https://localhost/api/v1/catalog", extra: { "content-security-policy": jsonCSP, "cross-origin-resource-policy": "same-origin" } },
  { url: "https://localhost:8443/files", extra: { "content-security-policy": jsonCSP, "cross-origin-resource-policy": "same-site", "access-control-allow-origin": "https://localhost" } },
  { url: "https://localhost:8444/realms/nugget/", extra: {} },
  { url: "https://localhost:9445/", extra: { "content-security-policy": appCSP, "cross-origin-opener-policy": "same-origin" } },
  { url: "https://localhost:9445/panel/v1/me", extra: { "content-security-policy": jsonCSP } },
  { url: "https://localhost:9446/login", extra: {} },
];

test.describe("security headers (C09)", () => {
  for (const c of cases) {
    test(c.url, async () => {
      const ctx = await request.newContext({ ignoreHTTPSErrors: true });
      const res = await ctx.get(c.url, { maxRedirects: 0 });
      const headers = res.headersArray();
      for (const [name, value] of Object.entries({ ...common, ...c.extra })) {
        const found = headers.filter((h) => h.name.toLowerCase() === name);
        expect(found, `${name} once`).toHaveLength(1);
        expect(found[0]?.value).toBe(value);
      }
      await ctx.dispose();
    });
  }
});
