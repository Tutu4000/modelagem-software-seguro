import { createCustomerClient, type CustomerClient } from "@nugget/api";
import { useSession } from "@nugget/auth";
import { useMemo } from "react";

/** useApi is the customer API with the session's token, read at each request. */
export function useApi(): CustomerClient {
  const { token } = useSession();
  return useMemo(() => createCustomerClient({ token }), [token]);
}
