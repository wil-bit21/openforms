import { describe, expect, it } from "vitest";
import { screen, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { server } from "../test/server";
import { api, apiError } from "../test/handlers";
import { ids, makeDetail, makeEvent, makeSubmission } from "../test/fixtures";
import { renderApp } from "../test/render";

function answer(label: string): string | null {
  return screen.getByText(label, { selector: "dt" }).nextElementSibling?.textContent ?? null;
}

describe("SubmissionPage", () => {
  it("shows answers with pinned labels, including legacy keys, and a timeline newest first", async () => {
    server.use(http.get(api("/submissions/:id"), () => HttpResponse.json(makeDetail({
      submission: makeSubmission({
        state: "screening", stateLabel: "Screening",
        data: { name: "Grace Hopper", email: "grace@example.com", role: "engineer", relocate: true, legacyField: "kept" },
      }),
      events: [
        makeEvent(),
        makeEvent({ id: 2, type: "transition", fromState: "new", toState: "screening", transition: "screen",
          actor: { type: "user", id: ids.reviewer, name: "Rita Reviewer" }, payload: { comment: "Looks promising" } }),
      ],
    }))));
    renderApp(`/submissions/${ids.sub1}`);

    expect(await screen.findByRole("heading", { level: 1 })).toHaveTextContent("Job application");
    expect(answer("Full name")).toBe("Grace Hopper");
    expect(answer("Role")).toBe("Engineer");
    expect(answer("Willing to relocate")).toBe("Yes");
    expect(answer("legacyField")).toBe("kept");
    expect(screen.queryByText("Portfolio URL", { selector: "dt" })).not.toBeInTheDocument();
    expect(screen.getByText("Screening", { selector: ".badge" })).toHaveAttribute("data-color", "blue");

    const items = within(screen.getByRole("list", { name: "Activity" })).getAllByRole("listitem");
    expect(items[0]).toHaveTextContent("Rita Reviewer: Start screening (New → Screening)");
    expect(items[0]).toHaveTextContent("Looks promising");
    expect(items[1]).toHaveTextContent("Submitted");
  });

  it("shows a not-found state", async () => {
    server.use(http.get(api("/submissions/:id"), () => apiError(404, "not_found", "submission not found")));
    renderApp(`/submissions/${ids.sub2}`);
    expect(await screen.findByRole("heading", { name: "Submission not found" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Back to the inbox" })).toHaveAttribute("href", "/submissions");
  });
});
