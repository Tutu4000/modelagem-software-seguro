// Errors from the API follow Problem Details (RFC 9457). The internal cause
// never reaches the client, so the code is what the interface reacts to.
import type { components } from "./customer.gen";

export type Problem = components["schemas"]["Problem"];

export function isProblem(value: unknown): value is Problem {
  if (typeof value !== "object" || value === null) {
    return false;
  }
  const v = value as Record<string, unknown>;
  return typeof v.type === "string" && typeof v.title === "string" && typeof v.status === "number" && typeof v.code === "string";
}

/** ApiError is what the hooks throw: the status, the problem code and, on a
 * 429, how many seconds to wait. */
export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly retryAfter: number | undefined;

  constructor(status: number, code: string, retryAfter?: number) {
    super(`${String(status)} ${code}`);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.retryAfter = retryAfter;
  }
}

/** retryAfterSeconds reads the Retry-After header in its seconds form; the
 * API never sends the date form. */
export function retryAfterSeconds(header: string | null): number | undefined {
  if (header === null || !/^\d+$/.test(header.trim())) {
    return undefined;
  }
  return Number(header.trim());
}

/** toApiError turns a failed response into an ApiError, whether or not its
 * body is a valid problem. */
export function toApiError(response: Response, body: unknown): ApiError {
  const code = isProblem(body) ? body.code : "unexpected";
  return new ApiError(response.status, code, retryAfterSeconds(response.headers.get("Retry-After")));
}
