import { describe, expect, it } from "vitest";

import { createCustomerClient } from "./client";
import { ApiError, isProblem, retryAfterSeconds, toApiError } from "./problem";
import { retryDelay, shouldRetry } from "./query";

describe("Problem Details", () => {
  it("recognizes a problem and nothing else", () => {
    expect(isProblem({ type: "/errors/conflict", title: "Conflict", status: 409, code: "conflict" })).toBe(true);
    expect(isProblem({ type: "x", title: "x", status: "409", code: "conflict" })).toBe(false);
    expect(isProblem(null)).toBe(false);
    expect(isProblem("conflict")).toBe(false);
  });

  it("reads Retry-After in seconds only", () => {
    expect(retryAfterSeconds("17")).toBe(17);
    expect(retryAfterSeconds(" 3 ")).toBe(3);
    expect(retryAfterSeconds("Wed, 21 Oct 2026 07:28:00 GMT")).toBeUndefined();
    expect(retryAfterSeconds(null)).toBeUndefined();
  });

  it("turns a response into an ApiError", () => {
    const limited = new Response(null, { status: 429, headers: { "Retry-After": "5" } });
    const err = toApiError(limited, { type: "/errors/rate_limited", title: "Too many requests", status: 429, code: "rate_limited" });
    expect(err).toMatchObject({ status: 429, code: "rate_limited", retryAfter: 5 });
    expect(toApiError(new Response(null, { status: 502 }), "<html>").code).toBe("unexpected");
  });
});

describe("retries", () => {
  it("does not retry a 4xx, waits for Retry-After on a 429 and retries a 5xx", () => {
    expect(shouldRetry(0, new ApiError(403, "forbidden"))).toBe(false);
    expect(shouldRetry(0, new ApiError(429, "rate_limited", 2))).toBe(true);
    expect(shouldRetry(1, new ApiError(429, "rate_limited", 2))).toBe(false);
    expect(shouldRetry(1, new ApiError(503, "unavailable"))).toBe(true);
    expect(retryDelay(0, new ApiError(429, "rate_limited", 2))).toBe(2000);
    expect(retryDelay(3, new Error("network"))).toBe(8000);
  });
});

describe("typed client", () => {
  it("sends the token only when there is a session", async () => {
    const seen: (string | null)[] = [];
    const fetch: typeof globalThis.fetch = (input) => {
      const request = input instanceof Request ? input : new Request(input);
      seen.push(request.headers.get("Authorization"));
      return Promise.resolve(Response.json({ version: 1, currency: "BRL", rules: {} }));
    };
    const session: { token?: string } = {};
    const client = createCustomerClient({ baseUrl: "https://localhost", fetch, token: () => session.token });
    await client.GET("/api/v1/catalog");
    session.token = "abc";
    await client.GET("/api/v1/catalog");
    expect(seen).toEqual([null, "Bearer abc"]);
  });
});
