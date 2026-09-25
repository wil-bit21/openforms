import { describe, expect, it } from "vitest";
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { server } from "../test/server";
import { api, apiError } from "../test/handlers";
import { ids, makeUser } from "../test/fixtures";
import { renderApp } from "../test/render";

function capture() {
  const calls = { create: [] as unknown[], update: [] as unknown[], del: [] as string[] };
  server.use(
    http.post(api("/users"), async ({ request }) => {
      calls.create.push(await request.json());
      return HttpResponse.json({ user: makeUser({ id: ids.manager, email: "sam@example.com", name: "Sam Manager", roles: ["hiring-manager"] }) }, { status: 201 });
    }),
    http.patch(api("/users/:id"), async ({ request, params }) => {
      calls.update.push({ id: params.id, body: await request.json() });
      return HttpResponse.json({ user: makeUser() });
    }),
    http.delete(api("/users/:id"), ({ params }) => {
      calls.del.push(String(params.id));
      return new HttpResponse(null, { status: 204 });
    }),
  );
  return calls;
}

describe("UsersPage", () => {
  it("lists users with roles", async () => {
    renderApp("/users");
    const row = (await screen.findByText("Rita Reviewer")).closest("tr")!;
    expect(within(row).getByText("reviewer@example.com")).toBeInTheDocument();
    expect(within(row).getByText("reviewer")).toHaveClass("tag");
  });

  it("creates a user with parsed roles", async () => {
    const calls = capture();
    const { user } = renderApp("/users");
    await user.click(await screen.findByRole("button", { name: "Add user" }));
    const dialog = screen.getByRole("dialog", { name: "Add user" });
    await user.type(within(dialog).getByLabelText("Name"), "Sam Manager");
    await user.type(within(dialog).getByLabelText("Email"), "sam@example.com");
    await user.type(within(dialog).getByLabelText("Password"), "s3cret-pass");
    await user.type(within(dialog).getByLabelText("Roles"), "hiring-manager, reviewer");
    await user.click(within(dialog).getByRole("button", { name: "Add user" }));
    await waitFor(() => expect(calls.create).toEqual([{ email: "sam@example.com", name: "Sam Manager", password: "s3cret-pass", roles: ["hiring-manager", "reviewer"] }]));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  });

  it("explains a duplicate email", async () => {
    server.use(http.post(api("/users"), () => apiError(409, "email_taken", "email already in use")));
    const { user } = renderApp("/users");
    await user.click(await screen.findByRole("button", { name: "Add user" }));
    const dialog = screen.getByRole("dialog", { name: "Add user" });
    await user.type(within(dialog).getByLabelText("Name"), "Rita Again");
    await user.type(within(dialog).getByLabelText("Email"), "reviewer@example.com");
    await user.type(within(dialog).getByLabelText("Password"), "s3cret-pass");
    await user.click(within(dialog).getByRole("button", { name: "Add user" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent("A user with this email already exists.");
  });

  it("edits only changed properties", async () => {
    const calls = capture();
    const { user } = renderApp("/users");
    const row = (await screen.findByText("Rita Reviewer")).closest("tr")!;
    await user.click(within(row).getByRole("button", { name: "Edit" }));
    const dialog = screen.getByRole("dialog", { name: "Edit Rita Reviewer" });
    const roles = within(dialog).getByLabelText("Roles");
    await user.clear(roles);
    await user.type(roles, "reviewer, hiring-manager");
    await user.click(within(dialog).getByRole("button", { name: "Save" }));
    await waitFor(() => expect(calls.update).toEqual([{ id: ids.reviewer, body: { roles: ["reviewer", "hiring-manager"] } }]));
  });

  it("deletes a user after confirmation but not yourself", async () => {
    const calls = capture();
    const { user } = renderApp("/users");
    const selfRow = (await screen.findByText("Ada Admin", { selector: "td" })).closest("tr")!;
    expect(within(selfRow).getByRole("button", { name: "Delete" })).toBeDisabled();

    const row = screen.getByText("Rita Reviewer").closest("tr")!;
    await user.click(within(row).getByRole("button", { name: "Delete" }));
    const dialog = screen.getByRole("dialog", { name: "Delete Rita Reviewer?" });
    await user.click(within(dialog).getByRole("button", { name: "Delete user" }));
    await waitFor(() => expect(calls.del).toEqual([ids.reviewer]));
  });
});
