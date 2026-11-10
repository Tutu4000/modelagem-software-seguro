// The STL upload. fetch cannot report the progress of a request body, so the
// upload uses XMLHttpRequest, wrapped to look like the rest: a promise, an
// AbortSignal and the same ApiError.
import type { components } from "./customer.gen";
import { ApiError, isProblem, retryAfterSeconds } from "./problem";

export type UploadAccepted = components["schemas"]["UploadAccepted"];

/** XhrLike is the part of XMLHttpRequest the upload uses; the browser's
 * XMLHttpRequest is one, and so is the tests' fake. */
export interface XhrLike {
  open(method: string, url: string): void;
  setRequestHeader(name: string, value: string): void;
  send(body: Blob): void;
  abort(): void;
  getResponseHeader(name: string): string | null;
  readonly status: number;
  readonly responseText: string;
  readonly upload: { onprogress: ((e: ProgressEvent) => void) | null };
  onload: ((e: ProgressEvent) => void) | null;
  onerror: ((e: ProgressEvent) => void) | null;
  onabort: ((e: ProgressEvent) => void) | null;
}

export interface UploadOptions {
  token: string | undefined;
  signal?: AbortSignal;
  /** Fraction sent, from 0 to 1. */
  onProgress?: (fraction: number) => void;
  /** For the tests; the browser's otherwise. */
  xhr?: () => XhrLike;
}

function parse(text: string): unknown {
  try {
    return JSON.parse(text) as unknown;
  } catch {
    return undefined;
  }
}

function isAccepted(v: unknown): v is UploadAccepted {
  return typeof v === "object" && v !== null && "id" in v && "status" in v && typeof v.id === "string" && typeof v.status === "string";
}

export function uploadStl(file: Blob, o: UploadOptions): Promise<UploadAccepted> {
  return new Promise((resolve, reject) => {
    const req: XhrLike = (o.xhr ?? (() => new XMLHttpRequest()))();
    req.open("POST", "/api/v1/files");
    req.setRequestHeader("Content-Type", "model/stl");
    if (o.token !== undefined) {
      req.setRequestHeader("Authorization", `Bearer ${o.token}`);
    }
    req.upload.onprogress = (e) => {
      if (e.lengthComputable) {
        o.onProgress?.(e.loaded / e.total);
      }
    };
    req.onload = () => {
      const body = parse(req.responseText);
      if (req.status === 202 && isAccepted(body)) {
        resolve(body);
        return;
      }
      // The 413 comes from nginx, before the API, with no problem body.
      const code = isProblem(body) ? body.code : req.status === 413 ? "file_too_large" : "unexpected";
      reject(new ApiError(req.status, code, retryAfterSeconds(req.getResponseHeader("Retry-After"))));
    };
    req.onerror = () => { reject(new TypeError("network error")); };
    req.onabort = () => { reject(new DOMException("upload cancelled", "AbortError")); };
    o.signal?.addEventListener("abort", () => { req.abort(); }, { once: true });
    req.send(file);
  });
}
