import { describe, expect, it } from "vitest";
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import type { SubmissionDetail } from "@openforms/sdk";
import { server } from "../../test/server";
import { api, apiError } from "../../test/handlers";
import { ids, makeDetail, makeReviewerPrincipal, makeSubmission, makeTransition } from "../../test/fixtures";
import { renderApp } from "../../test/render";

type Calls = { gets: number; transitions: unknown[]; fields: unknown[]; comments: unknown[]; assign: unknown[] };

function mockDetail(initial: SubmissionDetail = makeDetail(), opts: { afterTransition?: SubmissionDetail } = {}) {
  let current = initial;
  const calls: Calls = { gets: 0, transitions: [], fields: [], comments: [], assign: [] };
  const updated = (patch: Partial<SubmissionDetail["submission"]>) => {
    current = { ...current, submission: { ...current.submission, ...patch, updatedAt: new Date(Date.now() + calls.gets * 1000).toISOString() } };
    return HttpResponse.json({ submission: current.submission });
  };
  server.use(
    http.get(api("/submissions/:id"), () => {
      calls.gets++;
      return HttpResponse.json(current);
    }),
    http.post(api("/submissions/:id/transitions"), async ({ request }) => {
      calls.transitions.push(await request.json());
      current = opts.afterTransition ?? makeDetail({
        submission: makeSubmission({ state: "screening", stateLabel: "Screening", updatedAt: "2026-09-23T12:00:00Z" }),
        transitions: [],
      });
      return HttpResponse.json({ submission: current.submission });
    }),
    http.patch(api("/submissions/:id/fields"), async ({ request }) => {
      const body = (await request.json()) as { fields: Record<string, unknown> };
      calls.fields.push(body);
      return updated({ fields: { ...current.submission.fields, ...body.fields } });
    }),
    http.post(api("/submissions/:id/comments"), async ({ request }) => {
      calls.comments.push(await request.json());
      return HttpResponse.json({ event: { id: 9, type: "comment", fromState: null, toState: null, transition: null, actor: { type: "user", id: ids.admin, name: "Ada Admin" }, payload: { body: "x" }, createdAt: "2026-09-23T12:00:00Z" } }, { status: 201 });
    }),
    http.put(api("/submissions/:id/assignee"), async ({ request }) => {
      const body = (await request.json()) as { userId: string | null };
      calls.assign.push(body);
      return updated({ assignee: body.userId ? { id: body.userId, name: "Rita Reviewer", email: "reviewer@example.com" } : null });
    }),
  );
  return calls;
}

const path = `/submissions/${ids.sub1}`;

describe("submission actions", () => {
  it("performs a transition without required fields immediately, with expectedState", async () => {
    const calls = mockDetail();
    const { user } = renderApp(path);
    await user.click(await screen.findByRole("button", { name: "Start screening" }));
    await waitFor(() => expect(calls.transitions).toEqual([{ transition: "screen", expectedState: "new" }]));
    expect(await screen.findByText("Screening", { selector: ".badge" })).toBeInTheDocument();
  });

  it("asks for required fields and an optional comment before transitioning", async () => {
    const calls = mockDetail();
    const { user } = renderApp(path);
    await user.click(await screen.findByRole("button", { name: "Reject" }));
    const dialog = await screen.findByRole("dialog", { name: "Reject" });

    await user.click(within(dialog).getByRole("button", { name: "Reject" }));
    expect(within(dialog).getByText("Required")).toBeInTheDocument();
    expect(calls.transitions).toHaveLength(0);

    await user.type(within(dialog).getByLabelText("Rejection reason"), "Not enough experience");
    await user.type(within(dialog).getByLabelText("Comment (optional)"), "Discussed with Sam");
    await user.click(within(dialog).getByRole("button", { name: "Reject" }));

    await waitFor(() => expect(calls.transitions).toEqual([{
      transition: "reject", expectedState: "new",
      fields: { rejectionReason: "Not enough experience" }, comment: "Discussed with Sam",
    }]));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  });

  it("shows server validation problems next to the dialog field", async () => {
    mockDetail();
    server.use(http.post(api("/submissions/:id/transitions"), () =>
      apiError(422, "validation_failed", "Invalid fields", [{ path: "fields.rejectionReason", message: "must be at least 10 characters" }])));
    const { user } = renderApp(path);
    await user.click(await screen.findByRole("button", { name: "Reject" }));
    const dialog = await screen.findByRole("dialog", { name: "Reject" });
    await user.type(within(dialog).getByLabelText("Rejection reason"), "Too short");
    await user.click(within(dialog).getByRole("button", { name: "Reject" }));
    expect(await within(dialog).findByText("must be at least 10 characters")).toBeInTheDocument();
  });

  it("refetches and explains when someone else changed the submission (409)", async () => {
    const calls = mockDetail();
    server.use(http.post(api("/submissions/:id/transitions"), () => apiError(409, "state_conflict", "state changed")));
    const { user } = renderApp(path);
    await user.click(await screen.findByRole("button", { name: "Start screening" }));
    expect(await screen.findByText("This submission changed while you were viewing it.")).toBeInTheDocument();
    await waitFor(() => expect(calls.gets).toBeGreaterThanOrEqual(2));
  });

  it("disables transitions the user may not perform, with an explanation", async () => {
    mockDetail(makeDetail({ transitions: [makeTransition({ key: "hire", label: "Hire", to: "hired", toLabel: "Hired", allowed: false, reason: "role" })] }));
    renderApp(path);
    const button = await screen.findByRole("button", { name: "Hire" });
    expect(button).toBeDisabled();
    expect(button).toHaveAttribute("title", "You don't have a role that can perform this transition.");
  });

  it("saves changed workflow fields with PATCH", async () => {
    const calls = mockDetail();
    const { user } = renderApp(path);
    const save = await screen.findByRole("button", { name: "Save fields" });
    expect(save).toBeDisabled();
    await user.type(screen.getByLabelText("Score"), "4");
    await user.click(save);
    await waitFor(() => expect(calls.fields).toEqual([{ fields: { score: 4 } }]));
  });

  it("adds a comment", async () => {
    const calls = mockDetail();
    const { user } = renderApp(path);
    await user.type(await screen.findByLabelText("Comment"), "Call candidate on Monday");
    await user.click(screen.getByRole("button", { name: "Add comment" }));
    await waitFor(() => expect(calls.comments).toEqual([{ body: "Call candidate on Monday" }]));
    await waitFor(() => expect(screen.getByLabelText("Comment")).toHaveValue(""));
  });

  it("lets admins assign anyone from the user list", async () => {
    const calls = mockDetail();
    const { user } = renderApp(path);
    await screen.findByRole("option", { name: "Rita Reviewer (reviewer@example.com)" });
    await user.selectOptions(screen.getByLabelText("Assign to"), ids.reviewer);
    await waitFor(() => expect(calls.assign).toEqual([{ userId: ids.reviewer }]));
  });

  it("gives reviewers 'Assign to me' without calling the admin-only users endpoint", async () => {
    let usersCalled = false;
    const calls = mockDetail();
    server.use(
      http.get(api("/auth/me"), () => HttpResponse.json({ principal: makeReviewerPrincipal() })),
      http.get(api("/users"), () => {
        usersCalled = true;
        return apiError(403, "forbidden", "admin only");
      }),
    );
    const { user } = renderApp(path);
    await user.click(await screen.findByRole("button", { name: "Assign to me" }));
    await waitFor(() => expect(calls.assign).toEqual([{ userId: ids.reviewer }]));
    expect(await screen.findByRole("button", { name: "Unassign" })).toBeInTheDocument();
    expect(usersCalled).toBe(false);
  });
});
