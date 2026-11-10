import { describe, expect, it } from "vitest";

import { parseClaims } from "./claims";
import { safeReturnTo } from "./returnTo";

function jwt(payload: object): string {
  const enc = (o: object) => btoa(JSON.stringify(o)).replace(/=+$/, "").replace(/\+/g, "-").replace(/\//g, "_");
  return `${enc({ alg: "EdDSA" })}.${enc(payload)}.sig`;
}

const base = { sub: "u1", preferred_username: "customer-a", email: "a@nugget.local", acr: "1", auth_time: 1_800_000_000, exp: 1_800_000_300 };

describe("parseClaims", () => {
  it("reads the access token, roles included", () => {
    const c = parseClaims(jwt({ ...base, realm_access: { roles: ["customer", "offline_access"] } }));
    expect(c).toMatchObject({ sub: "u1", username: "customer-a", email: "a@nugget.local", acr: "1", roles: ["customer"] });
    expect(c?.authTime.getTime()).toBe(1_800_000_000_000);
  });

  it("refuses what does not have the shape", () => {
    expect(parseClaims(jwt({ ...base, sub: 7 }))).toBeUndefined();
    expect(parseClaims(jwt({ ...base, auth_time: "yesterday" }))).toBeUndefined();
    expect(parseClaims(jwt({ ...base, exp: undefined }))).toBeUndefined();
    expect(parseClaims("not.a.jwt")).toBeUndefined();
    expect(parseClaims("")).toBeUndefined();
  });

  it("drops unknown roles", () => {
    expect(parseClaims(jwt({ ...base, realm_access: { roles: ["root", 3, "admin"] } }))?.roles).toEqual(["admin"]);
  });
});

describe("safeReturnTo", () => {
  it("keeps a path of the app", () => {
    expect(safeReturnTo("/conta?x=1#y")).toBe("/conta?x=1#y");
    expect(safeReturnTo("/")).toBe("/");
  });

  it("refuses other origins and schemes (open redirect)", () => {
    for (const bad of ["//evil.example", "https://evil.example/x", "javascript:alert(1)", "/\\evil.example", "conta", "", 42, undefined]) {
      expect(safeReturnTo(bad)).toBe("/");
    }
  });
});
