import { describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { demoApi } from "../test/server";
import { ReviewerPanel } from "./ReviewerPanel";

describe("ReviewerPanel", () => {
  it("does not log in outside demo mode", async () => {
    render(<ReviewerPanel submissionId="sub-1" demoMode={false} onTransitioned={() => {}} />);
    expect(screen.getByText(/only available when the server runs in demo mode/)).toBeInTheDocument();
    await new Promise((r) => setTimeout(r, 50));
    expect(demoApi.loginCalls).toEqual([]);
  });

  it("performs transitions as the reviewer, collecting required fields", async () => {
    const onTransitioned = vi.fn();
    render(<ReviewerPanel submissionId="sub-1" demoMode onTransitioned={onTransitioned} />);

    await userEvent.click(await screen.findByRole("button", { name: "Start screening" }));
    await waitFor(() => expect(onTransitioned).toHaveBeenCalledTimes(1));
    expect(demoApi.loginCalls[0]).toBe("reviewer@demo.local");

    const invite = await screen.findByRole("button", { name: "Invite to interview" });
    expect(invite).toBeDisabled();
    await userEvent.type(screen.getByLabelText("Score"), "4");
    expect(invite).toBeEnabled();
    await userEvent.click(invite);

    await waitFor(() => expect(onTransitioned).toHaveBeenCalledTimes(2));
    expect(demoApi.transitionBodies[1]).toMatchObject({ transition: "invite", fields: { score: 4 }, expectedState: "screening" });
  });

  it("role-guarded transition is disabled until switching persona", async () => {
    demoApi.state = "interview";
    render(<ReviewerPanel submissionId="sub-1" demoMode onTransitioned={() => {}} />);

    const hire = await screen.findByRole("button", { name: "Hire" });
    expect(hire).toBeDisabled();
    expect(screen.getByText("Not allowed for Reviewer")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Hiring manager" }));
    await waitFor(() => expect(demoApi.loginCalls).toContain("manager@demo.local"));
    await waitFor(() => expect(screen.getByRole("button", { name: "Hire" })).toBeEnabled());
  });

  it("shows a final-state message for terminal submissions", async () => {
    demoApi.state = "hired";
    render(<ReviewerPanel submissionId="sub-1" demoMode onTransitioned={() => {}} />);
    expect(await screen.findByText(/reached a final state: Hired/)).toBeInTheDocument();
  });
});
