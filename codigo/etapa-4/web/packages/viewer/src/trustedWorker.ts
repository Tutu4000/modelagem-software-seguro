// The only Trusted Types policy of the apps: the CSP names it
// (trusted-types nugget-worker), and it turns a string into a script URL only
// for the app's own bundled workers. Everything else stays blocked.

export const policyName = "nugget-worker";

/** workerUrl accepts only a script of this origin under /assets/, where the
 * build puts the workers. The dev server serves them from elsewhere, still on
 * the same origin. */
export function workerUrl(input: string, origin: string, dev = false): string {
  const url = new URL(input, origin);
  const path = dev || url.pathname.startsWith("/assets/");
  if (url.origin !== origin || !path || url.hash !== "" || (!dev && url.search !== "")) {
    throw new TypeError(`refused worker URL ${input}`);
  }
  return url.href;
}

let policy: TrustedTypePolicy | undefined;

/** createWorker starts a module worker through the policy when the browser
 * enforces Trusted Types, and with the checked URL otherwise. */
export function createWorker(url: string): Worker {
  const origin = window.location.origin;
  const dev = import.meta.env.DEV;
  const tt = window.trustedTypes;
  if (tt === undefined) {
    return new Worker(workerUrl(url, origin, dev), { type: "module" });
  }
  policy ??= tt.createPolicy(policyName, { createScriptURL: (input) => workerUrl(input, origin, dev) });
  // The DOM typings of Worker know only strings and URLs; the browser takes
  // the trusted value in their place.
  return new Worker(policy.createScriptURL(url) as unknown as string, { type: "module" });
}
