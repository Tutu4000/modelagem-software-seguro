import type { UserManager } from "oidc-client-ts";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { useLocation, useNavigate } from "react-router";

import type { Role } from "./claims";
import { finishSignIn, useSession } from "./session";

export interface RequireSessionProps {
  children: ReactNode;
  /** Shown while the session is being restored. */
  loading: ReactNode;
  /** Shown instead of redirecting when the session expired. */
  expired: (signIn: () => void) => ReactNode;
  /** Roles that see this page. The API decides for real; this only spares a
   * screen that would answer 403. */
  roles?: readonly Role[];
  forbidden?: ReactNode;
}

/** RequireSession sends an anonymous visitor to the login, and back here
 * afterwards. */
export function RequireSession({ children, loading, expired, roles, forbidden = null }: RequireSessionProps) {
  const session = useSession();
  const location = useLocation();
  const here = location.pathname + location.search;
  const { status } = session.state;
  useEffect(() => {
    if (status === "anonymous") {
      void session.signIn(here);
    }
  }, [status, session, here]);

  switch (session.state.status) {
    case "loading":
    case "anonymous":
      return loading;
    case "expired":
      return expired(() => void session.signIn(here));
    case "authenticated":
      if (roles !== undefined && !session.state.claims.roles.some((r) => roles.includes(r))) {
        return forbidden;
      }
      return children;
  }
}

/** AuthCallback is the route Keycloak returns to. */
export function AuthCallback({ manager, loading, failed }: { manager: UserManager; loading: ReactNode; failed: ReactNode }) {
  const navigate = useNavigate();
  const [error, setError] = useState(false);
  const done = useRef(false);
  useEffect(() => {
    if (done.current) {
      return;
    }
    done.current = true;
    finishSignIn(manager).then(
      (to) => { void navigate(to, { replace: true }); },
      () => { setError(true); },
    );
  }, [manager, navigate]);
  return error ? failed : loading;
}
