import { createPanelClient, unwrap, type PanelClient } from "@nugget/api";
import { useSession } from "@nugget/auth";
import { useQuery } from "@tanstack/react-query";
import { useMemo } from "react";

/** useApi is the panel API with the session's token, read at each request. */
export function useApi(): PanelClient {
  const { token } = useSession();
  return useMemo(() => createPanelClient({ token }), [token]);
}

/** useMe is who the panel API says is logged in: the role and the ID the
 * screens use come from the API, not only from the token. */
export function useMe() {
  const api = useApi();
  return useQuery({ queryKey: ["me"], queryFn: () => unwrap(api.GET("/panel/v1/me")) });
}
