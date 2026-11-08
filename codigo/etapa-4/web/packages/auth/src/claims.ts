// What the apps read from the tokens, checked before use: a token that does
// not have this shape is treated as no session.
import { jwtDecode } from "jwt-decode";

export type Role = "customer" | "technician" | "support" | "admin";

const roles: readonly Role[] = ["customer", "technician", "support", "admin"];

export interface Claims {
  sub: string;
  username: string;
  email: string | undefined;
  /** "2" when the login had the second factor. */
  acr: string;
  /** When the user last typed the credentials. */
  authTime: Date;
  expiresAt: Date;
  roles: Role[];
}

function isRecord(v: unknown): v is Record<string, unknown> {
  return typeof v === "object" && v !== null && !Array.isArray(v);
}

function isRole(v: unknown): v is Role {
  return typeof v === "string" && (roles as readonly string[]).includes(v);
}

/** parseClaims reads the access token, the one the APIs check, or returns
 * undefined when it does not have the expected shape. The ID token may lack
 * auth_time; the access token always has it in this realm. */
export function parseClaims(accessToken: string): Claims | undefined {
  let access: unknown;
  try {
    access = jwtDecode(accessToken);
  } catch {
    return undefined;
  }
  if (!isRecord(access)) {
    return undefined;
  }
  const { sub, preferred_username: username, email, acr, auth_time: authTime, exp, realm_access: realm } = access;
  if (typeof sub !== "string" || typeof username !== "string" || typeof authTime !== "number" || typeof exp !== "number") {
    return undefined;
  }
  const granted = isRecord(realm) && Array.isArray(realm.roles) ? realm.roles.filter(isRole) : [];
  return {
    sub,
    username,
    email: typeof email === "string" ? email : undefined,
    acr: typeof acr === "string" ? acr : "",
    authTime: new Date(authTime * 1000),
    expiresAt: new Date(exp * 1000),
    roles: granted,
  };
}
