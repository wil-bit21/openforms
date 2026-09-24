import { describe, expect, it } from "vitest";
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { server } from "../test/server";
import { api, apiError } from "../test/handlers";
import { ids, makePrincipal, makeSubmission } from "../test/fixtures";
import { renderApp } from "../test/render";

describe("InboxPage", () => {
  it("lists submissions with form title, summary link, state color and assignee", async () => {
    server.use(http.get(api("/submissions"), () => HttpResponse.json({
      items: [makeSubmission({ state: "screening", stateLabel: "Screening", assignee: { id: ids.reviewer, name: "Rita Reviewer", email: "reviewer@example.com" } })],
      nextCursor: null,
    })));
    renderApp("/submissions");
    const link = await screen.findByRole("link", { name: "Grace Hopper · grace@example.com" });
    expect(link).toHaveAttribute("href", `/submissions/${ids.sub1}`);
    const row = link.closest("tr")!;
    expect(within(row).getByText("Job application")).toBeInTheDocument();
    expect(within(row).getByText("Rita Reviewer")).toBeInTheDocument();
    await waitFor(() => expect(within(row).getByText("Screening")).toHaveAttribute("data-color", "blue"));
  });

  it("sends form, state and assignee filters and mirrors them in the URL", async () => {
    const seen: URLSearchParams[] = [];
    server.use(http.get(api("/submissions"), ({ request }) => {
      seen.push(new URL(request.url).searchParams);
      return HttpResponse.json({ items: [], nextCursor: null });
    }));
    const { user, router } = renderApp("/submissions");
    await screen.findByText("No submissions match these filters.");
    expect(screen.getByLabelText("State")).toBeDisabled();

    await screen.findByRole("option", { name: "Job application" });
    await user.selectOptions(screen.getByLabelText("Form"), "job-application");
    await screen.findByRole("option", { name: "Screening" });
    await user.selectOptions(screen.getByLabelText("State"), "screening");
    await user.selectOptions(screen.getByLabelText("Assignee"), "none");

    await waitFor(() => {
      const last = seen.at(-1)!;
      expect(last.get("form")).toBe("job-application");
      expect(last.get("state")).toBe("screening");
      expect(last.get("assignee")).toBe("none");
    });
    expect(router.state.location.search).toBe("?form=job-application&state=screening&assignee=none");
  });

  it("clears the state filter when the form changes", async () => {
    const { user, router } = renderApp("/submissions?form=job-application&state=screening");
    await screen.findByRole("option", { name: "Job application" });
    await user.selectOptions(screen.getByLabelText("Form"), "");
    await waitFor(() => expect(router.state.location.search).toBe(""));
  });

  it("loads more pages with the cursor", async () => {
    server.use(http.get(api("/submissions"), ({ request }) => {
      const cursor = new URL(request.url).searchParams.get("cursor");
      return cursor === "c2"
        ? HttpResponse.json({ items: [makeSubmission({ id: ids.sub2, data: { name: "Alan Turing", email: "alan@example.com" } })], nextCursor: null })
        : HttpResponse.json({ items: [makeSubmission()], nextCursor: "c2" });
    }));
    const { user } = renderApp("/submissions");
    await user.click(await screen.findByRole("button", { name: "Load more" }));
    expect(await screen.findByRole("link", { name: "Alan Turing · alan@example.com" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Grace Hopper · grace@example.com" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Load more" })).not.toBeInTheDocument();
  });

  it("renders submissions of unknown forms and color-less states with a gray badge", async () => {
    server.use(http.get(api("/submissions"), () => HttpResponse.json({
      items: [makeSubmission({ form: "contact-us", state: "submitted", stateLabel: "Submitted", terminal: true, data: { topic: "Pricing" } })],
      nextCursor: null,
    })));
    renderApp("/submissions");
    const link = await screen.findByRole("link", { name: "Pricing" });
    const row = link.closest("tr")!;
    expect(within(row).getByText("contact-us")).toBeInTheDocument();
    expect(within(row).getByText("Submitted")).toHaveAttribute("data-color", "gray");
  });

  it("sends the user to login when the session expires mid-use", async () => {
    let meCalls = 0;
    server.use(
      http.get(api("/auth/me"), () =>
        ++meCalls === 1 ? HttpResponse.json({ principal: makePrincipal() }) : apiError(401, "unauthenticated", "Sign in required")),
      http.get(api("/submissions"), () => apiError(401, "unauthenticated", "Sign in required")),
    );
    const { router } = renderApp("/submissions");
    expect(await screen.findByRole("heading", { name: "Sign in to openforms" })).toBeInTheDocument();
    expect(router.state.location.pathname).toBe("/login");
  });
});
