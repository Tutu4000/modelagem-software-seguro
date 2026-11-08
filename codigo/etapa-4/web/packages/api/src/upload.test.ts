import { describe, expect, it } from "vitest";

import { uploadStl, type XhrLike } from "./upload";

class FakeXhr implements XhrLike {
  status = 0;
  responseText = "";
  headers: Record<string, string> = {};
  sent: Blob | undefined;
  aborted = false;
  readonly upload: { onprogress: ((e: ProgressEvent) => void) | null } = { onprogress: null };
  onload: ((e: ProgressEvent) => void) | null = null;
  onerror: ((e: ProgressEvent) => void) | null = null;
  onabort: ((e: ProgressEvent) => void) | null = null;
  responseHeaders: Record<string, string> = {};

  opened: string | undefined;

  open(method: string, url: string): void {
    this.opened = `${method} ${url}`;
  }
  setRequestHeader(name: string, value: string): void {
    this.headers[name] = value;
  }
  send(body: Blob): void {
    this.sent = body;
  }
  abort(): void {
    this.aborted = true;
    this.onabort?.(new ProgressEvent("abort"));
  }
  getResponseHeader(name: string): string | null {
    return this.responseHeaders[name] ?? null;
  }
  answer(status: number, body: string, headers: Record<string, string> = {}): void {
    this.status = status;
    this.responseText = body;
    this.responseHeaders = headers;
    this.onload?.(new ProgressEvent("load"));
  }
}

const file = new Blob([new Uint8Array(200)]);

describe("uploadStl", () => {
  it("sends the file with the token and reports the progress", async () => {
    const xhr = new FakeXhr();
    const seen: number[] = [];
    const done = uploadStl(file, { token: "abc", xhr: () => xhr, onProgress: (f) => seen.push(f) });
    xhr.upload.onprogress?.(new ProgressEvent("progress", { lengthComputable: true, loaded: 50, total: 200 }));
    xhr.answer(202, JSON.stringify({ id: "f1", status: "waiting" }));
    await expect(done).resolves.toEqual({ id: "f1", status: "waiting" });
    expect(seen).toEqual([0.25]);
    expect(xhr.headers).toEqual({ "Content-Type": "model/stl", Authorization: "Bearer abc" });
    expect(xhr.sent).toBe(file);
    expect(xhr.opened).toBe("POST /api/v1/files");
  });

  it("turns refusals into ApiError codes", async () => {
    const invalid = new FakeXhr();
    const p1 = uploadStl(file, { token: "t", xhr: () => invalid });
    invalid.answer(422, JSON.stringify({ type: "/errors/invalid_stl", title: "Invalid STL", status: 422, code: "invalid_stl" }));
    await expect(p1).rejects.toMatchObject({ status: 422, code: "invalid_stl" });

    const big = new FakeXhr();
    const p2 = uploadStl(file, { token: "t", xhr: () => big });
    big.answer(413, "<html>413</html>");
    await expect(p2).rejects.toMatchObject({ status: 413, code: "file_too_large" });

    const limited = new FakeXhr();
    const p3 = uploadStl(file, { token: "t", xhr: () => limited });
    limited.answer(429, JSON.stringify({ type: "x", title: "x", status: 429, code: "rate_limited" }), { "Retry-After": "7" });
    await expect(p3).rejects.toMatchObject({ code: "rate_limited", retryAfter: 7 });
  });

  it("can be cancelled", async () => {
    const xhr = new FakeXhr();
    const ctrl = new AbortController();
    const p = uploadStl(file, { token: "t", xhr: () => xhr, signal: ctrl.signal });
    ctrl.abort();
    await expect(p).rejects.toMatchObject({ name: "AbortError" });
    expect(xhr.aborted).toBe(true);
  });
});
