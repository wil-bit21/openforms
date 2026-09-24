import { describe, expect, it } from "vitest";
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { server } from "../test/server";
import { api } from "../test/handlers";
import { makeReviewerPrincipal } from "../test/fixtures";
import { renderApp } from "../test/render";

describe("Shell", () => {
  it("redirects / to the inbox and shows admin navigation to admins", async () => {
    const { router } = renderApp("/");
    const nav = await screen.findByRole("navigation", { name: "Main" });
    await waitFor(() => expect(router.state.location.pathname).toBe("/submissions"));
    for (const label of ["Inbox", "Forms", "Workflows", "Users", "API keys", "Jobs"]) {
      expect(within(nav).getByRole("link", { name: label })).toBeInTheDocument();
    }
    expect(screen.getByText("Ada Admin")).toBeInTheDocument();
  });

  it("hides admin-only navigation and blocks admin pages for reviewers", async () => {
    server.use(http.get(api("/auth/me"), () => HttpResponse.json({ principal: makeReviewerPrincipal() })));
    renderApp("/users");
    const nav = await screen.findByRole("navigation", { name: "Main" });
    expect(within(nav).queryByRole("link", { name: "Users" })).not.toBeInTheDocument();
    expect(within(nav).getByRole("link", { name: "Inbox" })).toBeInTheDocument();
    expect(await screen.findByRole("heading", { name: "Admins only" })).toBeInTheDocument();
  });

  it("signs out and returns to the login page", async () => {
    let loggedOut = false;
    server.use(http.post(api("/auth/logout"), () => {
      loggedOut = true;
      return new HttpResponse(null, { status: 204 });
    }));
    const { user, router } = renderApp("/submissions");
    await user.click(await screen.findByRole("button", { name: "Sign out" }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/login"));
    expect(loggedOut).toBe(true);
  });

  it("shows a not-found page for unknown admin paths", async () => {
    renderApp("/nope");
    expect(await screen.findByRole("heading", { name: "Page not found" })).toBeInTheDocument();
  });
});
