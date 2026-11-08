import type { User, UserManager } from "oidc-client-ts";
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";

import { parseClaims, type Claims } from "./claims";
import { safeReturnTo } from "./returnTo";

export type SessionState =
  | { status: "loading" }
  | { status: "anonymous" }
  | { status: "expired" }
  | { status: "authenticated"; claims: Claims };

export interface Session {
  state: SessionState;
  /** Goes to Keycloak and comes back to returnTo, a path of this app. */
  signIn: (returnTo?: string) => Promise<void>;
  signOut: () => Promise<void>;
  /** A fresh login with the second factor, in a popup, keeping the page as it
   * is (critical operations, E6.4). */
  stepUp: () => Promise<void>;
  /** Tries the session again without a page change; false when it is gone. */
  refresh: () => Promise<boolean>;
  /** The access token, or undefined without a session. Read at request time. */
  token: () => string | undefined;
}

const SessionContext = createContext<Session | undefined>(undefined);

function stateOf(user: User | null): SessionState {
  if (user === null || user.expired === true) {
    return { status: "anonymous" };
  }
  const claims = parseClaims(user.access_token);
  return claims === undefined ? { status: "anonymous" } : { status: "authenticated", claims };
}

export interface AuthProviderProps {
  manager: UserManager;
  children: ReactNode;
  /** The path of the redirect callback, where no silent attempt runs. */
  callbackPath?: string;
}

/** AuthProvider holds the session in memory. When the app opens, it tries
 * the Keycloak session in a hidden frame, so a reload does not log out. */
export function AuthProvider({ manager, children, callbackPath = "/auth/callback" }: AuthProviderProps) {
  const [state, setState] = useState<SessionState>({ status: "loading" });
  const user = useRef<User | null>(null);
  const started = useRef(false);

  const apply = useCallback((u: User | null) => {
    user.current = u;
    setState(stateOf(u));
  }, []);

  useEffect(() => {
    const loaded = (u: User) => { apply(u); };
    const expired = () => {
      user.current = null;
      setState({ status: "expired" });
    };
    manager.events.addUserLoaded(loaded);
    manager.events.addSilentRenewError(expired);
    manager.events.addAccessTokenExpired(expired);
    if (!started.current && window.location.pathname !== callbackPath) {
      started.current = true;
      manager.signinSilent().then(apply, () => { apply(null); });
    }
    return () => {
      manager.events.removeUserLoaded(loaded);
      manager.events.removeSilentRenewError(expired);
      manager.events.removeAccessTokenExpired(expired);
    };
  }, [manager, apply, callbackPath]);

  const session = useMemo<Session>(
    () => ({
      state,
      signIn: (returnTo) => manager.signinRedirect({ state: { returnTo: safeReturnTo(returnTo ?? "/") } }),
      signOut: async () => {
        const idToken = user.current?.id_token;
        user.current = null;
        await manager.removeUser();
        await manager.signoutRedirect(idToken === undefined ? {} : { id_token_hint: idToken });
      },
      stepUp: async () => {
        // prompt=login always asks for the password and the TOTP again;
        // max_age=0 would accept a login made in the same second.
        apply(await manager.signinPopup({ prompt: "login", acr_values: "2" }));
      },
      refresh: async () => {
        try {
          apply(await manager.signinSilent());
          return true;
        } catch {
          user.current = null;
          setState({ status: "expired" });
          return false;
        }
      },
      token: () => user.current?.access_token,
    }),
    [manager, state, apply],
  );

  return <SessionContext.Provider value={session}>{children}</SessionContext.Provider>;
}

export function useSession(): Session {
  const s = useContext(SessionContext);
  if (s === undefined) {
    throw new Error("useSession outside AuthProvider");
  }
  return s;
}

/** finishSignIn completes the redirect and says where to go back to. */
export async function finishSignIn(manager: UserManager): Promise<string> {
  const user = await manager.signinRedirectCallback();
  const state: unknown = user.state;
  const returnTo = typeof state === "object" && state !== null && "returnTo" in state ? state.returnTo : "/";
  return safeReturnTo(returnTo);
}
