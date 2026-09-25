import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { WorkflowDiagram } from "./WorkflowDiagram";
import { sampleWorkflow } from "../test/samples";

vi.mock("@xyflow/react", () => import("../test/xyflowMock"));

describe("WorkflowDiagram", () => {
  it("renders state nodes with initial and terminal markers", () => {
    render(<WorkflowDiagram def={sampleWorkflow()} selection={{ kind: "workflow" }} onSelect={() => {}} />);
    const newNode = screen.getByRole("button", { name: "node new" });
    expect(within(newNode).getByText("New")).toBeInTheDocument();
    expect(within(newNode).getByLabelText("Initial state")).toBeInTheDocument();
    expect(within(screen.getByRole("button", { name: "node hired" })).getByText("Hired").closest(".of-wf-node")).toHaveClass("of-wf-node--terminal");
  });

  it("selects states and transitions on click", async () => {
    const user = userEvent.setup();
    const onSelect = vi.fn();
    render(<WorkflowDiagram def={sampleWorkflow()} selection={{ kind: "workflow" }} onSelect={onSelect} />);
    await user.click(screen.getByRole("button", { name: "node screening" }));
    expect(onSelect).toHaveBeenLastCalledWith({ kind: "state", index: 1 });
    await user.click(screen.getAllByRole("button", { name: /^edge reject:/ })[0]);
    expect(onSelect).toHaveBeenLastCalledWith({ kind: "transition", index: 2 });
  });
});
