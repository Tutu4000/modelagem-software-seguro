/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** The Keycloak realm; the compose's when unset. */
  readonly VITE_OIDC_AUTHORITY?: string;
}
