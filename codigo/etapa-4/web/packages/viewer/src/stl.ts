// Reads a binary STL in the browser, with the server's limits, to show the
// part before the upload finishes. What prices the order is the server's own
// processing; this is only the preview.

export const maxBytes = 100 << 20;
export const maxTriangles = 2_000_000;

const headerBytes = 80;
const triangleBytes = 50;

export interface Mesh {
  positions: Float32Array;
  normals: Float32Array;
  triangles: number;
  /** Size along x, y and z, in millimetres. */
  bbox: [number, number, number];
  /** Signed-tetrahedra volume, in mm³. */
  volumeMm3: number;
}

export type StlProblem = "empty" | "too_large" | "not_binary" | "truncated" | "too_many_triangles";

export type ParseResult = { ok: true; mesh: Mesh } | { ok: false; problem: StlProblem };

/** looksAscii: an ASCII STL starts with "solid" and has no binary layout that
 * matches its size. Some binary files also start with "solid", so the size
 * decides. */
function looksAscii(view: DataView, count: number): boolean {
  const start = String.fromCharCode(...new Uint8Array(view.buffer, view.byteOffset, Math.min(5, view.byteLength)));
  return start === "solid" && view.byteLength !== headerBytes + 4 + count * triangleBytes;
}

export function parseBinaryStl(buffer: ArrayBuffer): ParseResult {
  if (buffer.byteLength === 0) {
    return { ok: false, problem: "empty" };
  }
  if (buffer.byteLength > maxBytes) {
    return { ok: false, problem: "too_large" };
  }
  if (buffer.byteLength < headerBytes + 4) {
    return { ok: false, problem: "not_binary" };
  }
  const view = new DataView(buffer);
  const count = view.getUint32(headerBytes, true);
  if (looksAscii(view, count)) {
    return { ok: false, problem: "not_binary" };
  }
  if (count === 0) {
    return { ok: false, problem: "empty" };
  }
  if (count > maxTriangles) {
    return { ok: false, problem: "too_many_triangles" };
  }
  if (buffer.byteLength < headerBytes + 4 + count * triangleBytes) {
    return { ok: false, problem: "truncated" };
  }

  const positions = new Float32Array(count * 9);
  const normals = new Float32Array(count * 9);
  const min = [Infinity, Infinity, Infinity];
  const max = [-Infinity, -Infinity, -Infinity];
  let volume = 0;
  for (let t = 0; t < count; t++) {
    const base = headerBytes + 4 + t * triangleBytes;
    const nx = view.getFloat32(base, true);
    const ny = view.getFloat32(base + 4, true);
    const nz = view.getFloat32(base + 8, true);
    const v: number[] = [];
    for (let k = 0; k < 9; k++) {
      const value = view.getFloat32(base + 12 + k * 4, true);
      v.push(value);
      positions[t * 9 + k] = value;
      const axis = k % 3;
      min[axis] = Math.min(min[axis] ?? Infinity, value);
      max[axis] = Math.max(max[axis] ?? -Infinity, value);
    }
    for (let corner = 0; corner < 3; corner++) {
      normals[t * 9 + corner * 3] = nx;
      normals[t * 9 + corner * 3 + 1] = ny;
      normals[t * 9 + corner * 3 + 2] = nz;
    }
    const [ax = 0, ay = 0, az = 0, bx = 0, by = 0, bz = 0, cx = 0, cy = 0, cz = 0] = v;
    volume += (ax * (by * cz - bz * cy) - ay * (bx * cz - bz * cx) + az * (bx * cy - by * cx)) / 6;
  }
  const size = (axis: number) => (max[axis] ?? 0) - (min[axis] ?? 0);
  return {
    ok: true,
    mesh: { positions, normals, triangles: count, bbox: [size(0), size(1), size(2)], volumeMm3: Math.abs(volume) },
  };
}
