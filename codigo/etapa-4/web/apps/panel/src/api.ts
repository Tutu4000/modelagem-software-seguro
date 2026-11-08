import { createPanelClient, type PanelClient } from "@nugget/api";
import { useSession } from "@nugget/auth";
import { useMemo } from "react";

/** useApi is the panel API with the session's token, read at each request. */
export function useApi(): PanelClient {
  const { token } = useSession();
  return useMemo(() => createPanelClient({ token }), [token]);
}
