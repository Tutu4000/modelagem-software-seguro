// The part of the Trusted Types API the app uses. TypeScript's DOM library
// does not declare it yet.
interface TrustedScriptURL {
  toString(): string;
}

interface TrustedTypePolicy {
  createScriptURL(input: string): TrustedScriptURL;
}

interface TrustedTypePolicyFactory {
  createPolicy(name: string, rules: { createScriptURL: (input: string) => string }): TrustedTypePolicy;
}

interface Window {
  trustedTypes?: TrustedTypePolicyFactory;
}
