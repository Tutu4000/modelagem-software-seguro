import { useQueryClient } from "@tanstack/react-query";
import { useEffect } from "react";

import { isUnauthenticated } from "./unwrap";

/** SessionRecovery reacts to a 401 that is not a step-up: it tries the
 * session again once, silently, and refetches when it is back. When it is
 * gone, refresh marks the session expired and the page asks for a login. */
export function SessionRecovery({ refresh }: { refresh: () => Promise<boolean> }) {
  const client = useQueryClient();
  useEffect(() => {
    let busy = false;
    return client.getQueryCache().subscribe((e) => {
      if (busy || e.type !== "updated" || e.action.type !== "error" || !isUnauthenticated(e.action.error)) {
        return;
      }
      busy = true;
      void refresh().then((ok) => {
        busy = false;
        if (ok) {
          void client.invalidateQueries();
        }
      });
    });
  }, [client, refresh]);
  return null;
}
