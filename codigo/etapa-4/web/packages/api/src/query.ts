// The QueryClient both apps share. A 4xx is an answer, not a failure: it is
// not retried. A 429 waits for Retry-After, and a 5xx or a network error is
// retried twice.
import { QueryClient } from "@tanstack/react-query";

import { ApiError } from "./problem";

export function shouldRetry(failureCount: number, error: unknown): boolean {
  if (error instanceof ApiError) {
    if (error.status === 429) {
      return failureCount < 1;
    }
    if (error.status < 500) {
      return false;
    }
  }
  return failureCount < 2;
}

export function retryDelay(failureCount: number, error: unknown): number {
  if (error instanceof ApiError && error.retryAfter !== undefined) {
    return error.retryAfter * 1000;
  }
  return Math.min(1000 * 2 ** failureCount, 10_000);
}

export function createQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: shouldRetry, retryDelay, staleTime: 10_000, refetchOnWindowFocus: false },
      mutations: { retry: false },
    },
  });
}
