import type { ParseResult } from "./stl";
import workerUrl from "./stl.worker.ts?worker&url";
import { createWorker } from "./trustedWorker";

/** parseInWorker reads the file off the main thread: a big STL does not
 * freeze the page. The buffer is transferred, not copied. */
export async function parseInWorker(file: Blob): Promise<ParseResult> {
  const buffer = await file.arrayBuffer();
  const worker = createWorker(workerUrl);
  try {
    return await new Promise<ParseResult>((resolve, reject) => {
      worker.onmessage = (e: MessageEvent<ParseResult>) => { resolve(e.data); };
      worker.onerror = (e) => { reject(new Error(e.message)); };
      worker.postMessage(buffer, [buffer]);
    });
  } finally {
    worker.terminate();
  }
}
