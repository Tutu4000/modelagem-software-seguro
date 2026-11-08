import { createUserManager } from "@nugget/auth";

// The "panel-web" client: staff only, and every login with the second factor
// (acr 2, D02).
export const manager = createUserManager({
  clientId: "panel-web",
  origin: window.location.origin,
  authority: import.meta.env.VITE_OIDC_AUTHORITY ?? "https://localhost:8444/realms/nugget",
  acr: "2",
});
