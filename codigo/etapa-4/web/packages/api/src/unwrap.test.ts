import { describe, expect, it } from "vitest";

import { ApiError } from "./problem";
import { isStepUp, isUnauthenticated, notice, unwrap } from "./unwrap";

describe("unwrap", () => {
  it("returns the data of a success", async () => {
    await expect(unwrap(Promise.resolve({ data: { v: 1 }, response: new Response(null, { status: 200 }) }))).resolves.toEqual({ v: 1 });
  });

  it("throws the problem code of a failure", async () => {
    const response = new Response(null, { status: 401, headers: { "WWW-Authenticate": 'Bearer error="insufficient_user_authentication", max_age=300' } });
    const call = unwrap(Promise.resolve({ error: { type: "/errors/step_up_required", title: "x", status: 401, code: "step_up_required" }, response }));
    await expect(call).rejects.toMatchObject({ status: 401, code: "step_up_required" });
  });
});

describe("classification", () => {
  it("tells a step-up from a lost session", () => {
    expect(isStepUp(new ApiError(401, "step_up_required"))).toBe(true);
    expect(isUnauthenticated(new ApiError(401, "step_up_required"))).toBe(false);
    expect(isUnauthenticated(new ApiError(401, "unauthenticated"))).toBe(true);
    expect(isStepUp(new ApiError(403, "forbidden"))).toBe(false);
  });

  it("has a notice for limits, outages and partners only", () => {
    expect(notice(new ApiError(429, "rate_limited", 12))?.text).toContain("12 s");
    expect(notice(new ApiError(502, "partner_unavailable"))?.tone).toBe("warning");
    expect(notice(new ApiError(503, "unavailable"))?.tone).toBe("danger");
    expect(notice(new TypeError("Failed to fetch"))?.tone).toBe("danger");
    expect(notice(new ApiError(409, "conflict"))).toBeUndefined();
    expect(notice(new ApiError(403, "forbidden"))).toBeUndefined();
  });
});
