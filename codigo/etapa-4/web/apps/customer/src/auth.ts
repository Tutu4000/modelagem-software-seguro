import { createUserManager } from "@nugget/auth";

// The "web" client of the realm: customers only, PKCE, no second factor.
export const manager = createUserManager({
  clientId: "web",
  origin: window.location.origin,
  authority: import.meta.env.VITE_OIDC_AUTHORITY ?? "https://localhost:8444/realms/nugget",
});
