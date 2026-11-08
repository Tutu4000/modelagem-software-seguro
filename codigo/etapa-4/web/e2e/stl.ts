// Binary STL boxes for the tests, like internal/fakestl.Box.
export function box(x: number, y: number, z: number): Buffer {
  const v = [
    [0, 0, 0], [x, 0, 0], [x, y, 0], [0, y, 0],
    [0, 0, z], [x, 0, z], [x, y, z], [0, y, z],
  ];
  const faces = [
    [0, 2, 1], [0, 3, 2], [4, 5, 6], [4, 6, 7], [0, 1, 5], [0, 5, 4],
    [1, 2, 6], [1, 6, 5], [2, 3, 7], [2, 7, 6], [3, 0, 4], [3, 4, 7],
  ];
  const buf = Buffer.alloc(84 + faces.length * 50);
  buf.writeUInt32LE(faces.length, 80);
  faces.forEach((f, i) => {
    f.forEach((corner, k) => {
      (v[corner] ?? [0, 0, 0]).forEach((value, axis) => {
        buf.writeFloatLE(value, 84 + i * 50 + 12 + k * 12 + axis * 4);
      });
    });
  });
  return buf;
}
