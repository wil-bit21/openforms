import { describe, expect, it, vi } from "vitest";
import { OpenFormsClient } from "./client";

describe("getPublicConfig", () => {
  it("GETs /api/v1/public/config and returns the body", async () => {
    const fetchMock = vi.fn<typeof fetch>(
      async () =>
        new Response(JSON.stringify({ demo: true }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
    );
    const client = new OpenFormsClient({ baseUrl: "http://api.test", fetch: fetchMock });

    await expect(client.getPublicConfig()).resolves.toEqual({ demo: true });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(String(url)).toBe("http://api.test/api/v1/public/config");
    expect(init?.method ?? "GET").toBe("GET");
  });
});
