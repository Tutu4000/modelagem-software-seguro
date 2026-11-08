// Parses the STL off the main thread and hands the arrays back without a copy.
import { parseBinaryStl, type ParseResult } from "./stl";

export type WorkerAnswer = ParseResult;

self.onmessage = (e: MessageEvent<ArrayBuffer>) => {
  const result = parseBinaryStl(e.data);
  if (result.ok) {
    postMessage(result, { transfer: [result.mesh.positions.buffer, result.mesh.normals.buffer] });
  } else {
    postMessage(result);
  }
};
