import { describe, expect, it } from "vitest";

import { maxTriangles, parseBinaryStl } from "./stl";
import { workerUrl } from "./trustedWorker";

/** box builds a binary STL of an axis-aligned box, like internal/fakestl. */
function box(x: number, y: number, z: number): ArrayBuffer {
  const v = [
    [0, 0, 0], [x, 0, 0], [x, y, 0], [0, y, 0],
    [0, 0, z], [x, 0, z], [x, y, z], [0, y, z],
  ];
  const faces = [
    [0, 2, 1], [0, 3, 2], [4, 5, 6], [4, 6, 7], [0, 1, 5], [0, 5, 4],
    [1, 2, 6], [1, 6, 5], [2, 3, 7], [2, 7, 6], [3, 0, 4], [3, 4, 7],
  ];
  const buf = new ArrayBuffer(84 + faces.length * 50);
  const view = new DataView(buf);
  view.setUint32(80, faces.length, true);
  faces.forEach((f, i) => {
    const base = 84 + i * 50;
    f.forEach((corner, k) => {
      const p = v[corner] ?? [0, 0, 0];
      p.forEach((value, axis) => { view.setFloat32(base + 12 + k * 12 + axis * 4, value, true); });
    });
  });
  return buf;
}

describe("parseBinaryStl", () => {
  it("measures a box", () => {
    const r = parseBinaryStl(box(10, 20, 30));
    expect(r.ok).toBe(true);
    if (r.ok) {
      expect(r.mesh.triangles).toBe(12);
      expect(r.mesh.bbox).toEqual([10, 20, 30]);
      expect(r.mesh.volumeMm3).toBeCloseTo(6000);
      expect(r.mesh.positions).toHaveLength(12 * 9);
    }
  });

  it("refuses what is not a usable binary STL", () => {
    const ascii = new TextEncoder().encode("solid cube\n facet normal 0 0 1\n  outer loop\n   vertex 0 0 0\n".repeat(4)).buffer;
    expect(parseBinaryStl(ascii)).toEqual({ ok: false, problem: "not_binary" });
    expect(parseBinaryStl(new ArrayBuffer(0))).toEqual({ ok: false, problem: "empty" });
    expect(parseBinaryStl(box(1, 1, 1).slice(0, 200))).toEqual({ ok: false, problem: "truncated" });
    const huge = new ArrayBuffer(84);
    new DataView(huge).setUint32(80, maxTriangles + 1, true);
    expect(parseBinaryStl(huge)).toEqual({ ok: false, problem: "too_many_triangles" });
  });
});

describe("workerUrl (Trusted Types policy)", () => {
  const origin = "https://localhost";
  it("accepts only this origin's bundled scripts", () => {
    expect(workerUrl("/assets/stl.worker-abc.js", origin)).toBe("https://localhost/assets/stl.worker-abc.js");
  });

  it("refuses anything else", () => {
    for (const bad of ["https://evil.example/assets/x.js", "javascript:alert(1)", "/src/stl.worker.ts", "/assets/x.js?y=1", "//evil.example/assets/x.js", "data:text/javascript,1"]) {
      expect(() => workerUrl(bad, origin)).toThrow(TypeError);
    }
  });
});
