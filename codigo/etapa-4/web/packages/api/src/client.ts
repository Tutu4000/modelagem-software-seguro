// Typed clients for the two APIs, on the same origin as the app that uses
// them: the customer app talks to /api/v1 and the panel to /panel/v1. The
// token comes from the session (FE1) through the middleware, never from web
// storage.
import createClient, { type Middleware } from "openapi-fetch";

import type { paths as CustomerPaths } from "./customer.gen";
import type { paths as PanelPaths } from "./panel.gen";

/** TokenSource returns the current access token, or undefined when there is
 * no session. */
export type TokenSource = () => string | undefined;

export function bearer(token: TokenSource): Middleware {
  return {
    onRequest({ request }) {
      const t = token();
      if (t !== undefined) {
        request.headers.set("Authorization", `Bearer ${t}`);
      }
      return request;
    },
  };
}

export interface ClientOptions {
  token?: TokenSource;
  baseUrl?: string;
  /** For the tests; the browser's fetch otherwise. */
  fetch?: typeof globalThis.fetch;
}

function options(o: ClientOptions) {
  return o.fetch === undefined ? { baseUrl: o.baseUrl ?? "" } : { baseUrl: o.baseUrl ?? "", fetch: o.fetch };
}

export function createCustomerClient(o: ClientOptions = {}) {
  const client = createClient<CustomerPaths>(options(o));
  client.use(bearer(o.token ?? (() => undefined)));
  return client;
}

export function createPanelClient(o: ClientOptions = {}) {
  const client = createClient<PanelPaths>(options(o));
  client.use(bearer(o.token ?? (() => undefined)));
  return client;
}

export type CustomerClient = ReturnType<typeof createCustomerClient>;
export type PanelClient = ReturnType<typeof createPanelClient>;
