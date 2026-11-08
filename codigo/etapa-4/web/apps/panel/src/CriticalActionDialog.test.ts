import { describe, expect, it } from "vitest";

import { codePoints, justificationError } from "./CriticalActionDialog";

describe("justification", () => {
  it("counts code points, like the panel API", () => {
    expect(codePoints("ação")).toBe(4);
    expect(codePoints("👍👍")).toBe(2);
  });

  it("asks for 10 to 500 characters, ignoring the edges", () => {
    expect(justificationError("   curto   ")).toBeDefined();
    expect(justificationError("revogação por incidente")).toBeUndefined();
    expect(justificationError("x".repeat(501))).toBeDefined();
    expect(justificationError("x".repeat(500))).toBeUndefined();
  });
});
