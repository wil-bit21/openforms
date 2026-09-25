import { describe, expect, it } from "vitest";
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { server } from "../test/server";
import { api } from "../test/handlers";
import { ids, makeApiKey } from "../test/fixtures";
import { renderApp } from "../test/render";

const PLAINTEXT = "ofk_Zx9QwErTy1234567890abcdefghijklmnopqrst";

function keysApi() {
  const calls = { created: [] as unknown[], revoked: [] as string[] };
  server.use(
    http.get(api("/api-keys"), () => HttpResponse.json({ items: [
      makeApiKey(),
      makeApiKey({ id: ids.key2, name: "Old integration", prefix: "ofk_Old1", roles: ["reviewer"], revokedAt: "2026-09-15T10:00:00Z" }),
    ] })),
    http.post(api("/api-keys"), async ({ request }) => {
      calls.created.push(await request.json());
      return HttpResponse.json({ apiKey: makeApiKey({ id: "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee", name: "Zapier", prefix: "ofk_Zx9Q" }), key: PLAINTEXT }, { status: 201 });
    }),
    http.delete(api("/api-keys/:id"), ({ params }) => {
      calls.revoked.push(String(params.id));
      return new HttpResponse(null, { status: 204 });
    }),
  );
  return calls;
}

describe("ApiKeysPage", () => {
  it("lists active and revoked keys", async () => {
    keysApi();
    renderApp("/api-keys");
    const active = (await screen.findByText("CI deploy")).closest("tr")!;
    expect(within(active).getByText("Active")).toBeInTheDocument();
    expect(within(active).getByText("ofk_Ab12…")).toBeInTheDocument();
    const revoked = screen.getByText("Old integration").closest("tr")!;
    expect(within(revoked).getByText("Revoked")).toBeInTheDocument();
    expect(within(revoked).queryByRole("button", { name: "Revoke" })).not.toBeInTheDocument();
  });

  it("creates a key and shows the plaintext exactly once", async () => {
    const calls = keysApi();
    const { user } = renderApp("/api-keys");
    await user.click(await screen.findByRole("button", { name: "Create API key" }));
    const create = screen.getByRole("dialog", { name: "Create API key" });
    await user.type(within(create).getByLabelText("Name"), "Zapier");
    await user.type(within(create).getByLabelText("Roles"), "reviewer");
    await user.click(within(create).getByRole("button", { name: "Create key" }));

    await waitFor(() => expect(calls.created).toEqual([{ name: "Zapier", roles: ["reviewer"] }]));
    const show = await screen.findByRole("dialog", { name: "Copy your API key" });
    expect(within(show).getByDisplayValue(PLAINTEXT)).toBeInTheDocument();
    expect(within(show).getByText(/won't be able to see it again/)).toBeInTheDocument();
    await user.click(within(show).getByRole("button", { name: "Copy" }));
    expect(await within(show).findByRole("button", { name: "Copied" })).toBeInTheDocument();

    await user.click(within(show).getByRole("button", { name: "Done" }));
    expect(screen.queryByDisplayValue(PLAINTEXT)).not.toBeInTheDocument();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("revokes a key after confirmation", async () => {
    const calls = keysApi();
    const { user } = renderApp("/api-keys");
    const row = (await screen.findByText("CI deploy")).closest("tr")!;
    await user.click(within(row).getByRole("button", { name: "Revoke" }));
    const dialog = screen.getByRole("dialog", { name: "Revoke CI deploy?" });
    await user.click(within(dialog).getByRole("button", { name: "Revoke key" }));
    await waitFor(() => expect(calls.revoked).toEqual([ids.key1]));
  });
});
