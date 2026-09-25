import { OpenFormsClient } from "@openforms/sdk";

/**
 * Resolves same-origin relative URLs against the page origin and looks up
 * `globalThis.fetch` at call time, so tests intercepted by MSW (which patches
 * fetch after module load) and non-browser runtimes both work.
 */
export function resolvingFetch(input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
  const url = typeof input === "string" && input.startsWith("/")
    ? new URL(input, window.location.origin).toString()
    : input;
  return globalThis.fetch(url, init);
}

export const client = new OpenFormsClient({ baseUrl: "", fetch: resolvingFetch as typeof fetch });
