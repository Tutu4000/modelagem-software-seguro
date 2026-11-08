// The OIDC client of each app. Authorization code with PKCE S256, against the
// Keycloak realm on the same site. The tokens live in memory only; the PKCE
// state and verifier, which must survive the trip to Keycloak and back, stay
// in sessionStorage until the callback consumes them (oidc-client-ts does it,
// never our code).
import { InMemoryWebStorage, UserManager, WebStorageStateStore } from "oidc-client-ts";

export interface AuthConfig {
  /** Keycloak client: "web" or "panel-web". */
  clientId: string;
  /** The app's origin, such as https://localhost. */
  origin: string;
  /** The realm URL, https://localhost:8444/realms/nugget. */
  authority: string;
  /** "2" makes every login ask for the second factor (panel, D02). */
  acr?: "2";
}

/** The page that answers the hidden frame (session restore) and the popup
 * (step-up). It is not the SPA: a few kilobytes, same CSP. */
export const callbackPage = "/oidc-callback.html";

export function createUserManager(c: AuthConfig): UserManager {
  return new UserManager({
    authority: c.authority,
    client_id: c.clientId,
    redirect_uri: `${c.origin}/auth/callback`,
    silent_redirect_uri: `${c.origin}${callbackPage}`,
    popup_redirect_uri: `${c.origin}${callbackPage}`,
    post_logout_redirect_uri: `${c.origin}/`,
    response_type: "code",
    scope: "openid",
    userStore: new WebStorageStateStore({ store: new InMemoryWebStorage() }),
    automaticSilentRenew: true,
    // The session iframe of Keycloak would need third-party cookies; the
    // back-channel logout already ends the session at the APIs.
    monitorSession: false,
    loadUserInfo: false,
    ...(c.acr === undefined ? {} : { acr_values: c.acr }),
  });
}
