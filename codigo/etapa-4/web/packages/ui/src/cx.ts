/** cx joins the class names that are set. CSS Module lookups may be
 * undefined under noUncheckedIndexedAccess, and so may the caller's own. */
export function cx(...parts: (string | false | null | undefined)[]): string {
  return parts.filter((p): p is string => typeof p === "string" && p !== "").join(" ");
}
