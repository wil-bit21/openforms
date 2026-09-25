import { describe, expect, it } from "vitest";
import { screen, within } from "@testing-library/react";
import { renderApp } from "../test/render";
import { actionText } from "./WorkflowOverviewPage";

describe("WorkflowsPage", () => {
  it("lists workflows with state count and version", async () => {
    renderApp("/workflows");
    const link = await screen.findByRole("link", { name: "Hiring pipeline" });
    expect(link).toHaveAttribute("href", "/workflows/hiring");
    const row = link.closest("tr")!;
    expect(within(row).getByText("5")).toBeInTheDocument();
    expect(within(row).getByText("v2")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "New workflow" })).toHaveAttribute("href", "/workflows/new");
  });
});

describe("WorkflowOverviewPage", () => {
  it("shows states, transitions, fields, actions, forms and versions", async () => {
    renderApp("/workflows/hiring");
    expect(await screen.findByRole("heading", { level: 1, name: "Hiring pipeline" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Edit" })).toHaveAttribute("href", "/workflows/hiring/edit");

    const states = screen.getByRole("region", { name: "States" });
    expect(within(states).getByText("Screening")).toHaveAttribute("data-color", "blue");
    expect(within(states).getByText("New").closest("li")).toHaveTextContent("initial");
    expect(within(states).getByText("Hired").closest("li")).toHaveTextContent("terminal");

    const transitions = screen.getByRole("region", { name: "Transitions" });
    const reject = within(transitions).getByText("Reject").closest("tr")!;
    expect(reject).toHaveTextContent("New, Screening, Interview");
    expect(reject).toHaveTextContent("Rejected");
    expect(reject).toHaveTextContent("reviewer, hiring-manager");
    expect(reject).toHaveTextContent("Rejection reason");
    expect(reject).toHaveTextContent("Email to {{submission.data.email}}");

    expect(within(screen.getByRole("region", { name: "Workflow fields" })).getByText("Score")).toBeInTheDocument();
    expect(within(screen.getByRole("region", { name: "On submit" })).getByText("Assign to role reviewer")).toBeInTheDocument();
    expect(await within(screen.getByRole("region", { name: "Used by forms" })).findByRole("link", { name: "Job application" }))
      .toHaveAttribute("href", "/forms/job-application");
    expect(await within(screen.getByRole("region", { name: "Versions" })).findByText("v2")).toBeInTheDocument();
  });

  it("shows not found for unknown workflows", async () => {
    renderApp("/workflows/missing");
    expect(await screen.findByRole("heading", { name: "Workflow not found" })).toBeInTheDocument();
  });
});

describe("actionText", () => {
  it("describes each action type", () => {
    expect(actionText({ type: "webhook", url: "https://x.test/h" })).toBe("Webhook to https://x.test/h");
    expect(actionText({ type: "email", to: "ops@x.test" })).toBe("Email to ops@x.test");
    expect(actionText({ type: "assign", user: "a@x.test" })).toBe("Assign to a@x.test");
    expect(actionText({ type: "assign", role: "reviewer" })).toBe("Assign to role reviewer");
  });
});
