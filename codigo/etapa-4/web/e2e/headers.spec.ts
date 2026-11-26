import { expect, request, test } from "@playwright/test";

const appCSP =
  "default-src 'none'; script-src 'self'; style-src 'self'; font-src 'self'; img-src 'self' data: blob:; connect-src 'self' https://localhost:8444 https://localhost:8443; frame-src 'self' https://localhost:8444; worker-src 'self' blob:; manifest-src 'self'; form-action 'self' https://localhost:8444; base-uri 'none'; frame-ancestors 'self'; object-src 'none'; require-trusted-types-for 'script'; trusted-types nugget-worker";
const jsonCSP = "default-src 'none'; frame-ancestors 'none'; form-action 'none'; base-uri 'none'";

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
  { url: "https://localhost:8443/files", extra: { "content-security-policy": jsonCSP, "cross-origin-resource-policy": "same-site" } },
  { url: "https://localhost:8444/realms/nugget/", extra: {} },
  { url: "https://localhost:9445/", extra: { "content-security-policy": appCSP, "cross-origin-opener-policy": "same-origin-allow-popups" } },
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

// Garage's own "*" is hidden: the designs are readable through CORS only by
// the customer app and the panel, the two 3D viewers.
test.describe("storage CORS", () => {
  const origins: [string, string | undefined][] = [
    ["https://localhost", "https://localhost"],
    ["https://localhost:9445", "https://localhost:9445"],
    ["https://evil.example", undefined],
    ["https://localhost:8443", undefined],
  ];
  for (const [origin, allowed] of origins) {
    test(`origin ${origin}`, async () => {
      const ctx = await request.newContext({ ignoreHTTPSErrors: true });
      const res = await ctx.get("https://localhost:8443/files", { headers: { Origin: origin }, maxRedirects: 0 });
      const found = res.headersArray().filter((h) => h.name.toLowerCase() === "access-control-allow-origin");
      expect(found.map((h) => h.value)).toEqual(allowed === undefined ? [] : [allowed]);
      await ctx.dispose();
    });
  }
});
