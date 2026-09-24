import { useReducer } from "react";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { initWorkflowEditor, workflowEditorReducer } from "./workflowReducer";
import { WorkflowOutline } from "./WorkflowOutline";
import { WorkflowMetaPanel } from "./WorkflowMetaPanel";
import { StateInspector } from "./StateInspector";
import { TransitionInspector } from "./TransitionInspector";
import { WorkflowFieldInspector } from "./WorkflowFieldInspector";
import { sampleWorkflow } from "../test/samples";
import type { Problem, WorkflowDef } from "../shared/types";

function Harness({ problems = [], isNew = false }: { problems?: Problem[]; isNew?: boolean }) {
  const [state, dispatch] = useReducer(workflowEditorReducer, sampleWorkflow(), initWorkflowEditor);
  const sel = state.selection;
  return (
    <>
      <WorkflowOutline def={state.def} selection={sel} problems={problems} dispatch={dispatch} />
      {sel.kind === "workflow" ? <WorkflowMetaPanel def={state.def} isNew={isNew} problems={problems} dispatch={dispatch} /> : null}
      {sel.kind === "state" ? <StateInspector def={state.def} index={sel.index} problems={problems} dispatch={dispatch} /> : null}
      {sel.kind === "transition" ? <TransitionInspector def={state.def} index={sel.index} problems={problems} dispatch={dispatch} /> : null}
      {sel.kind === "field" ? <WorkflowFieldInspector def={state.def} index={sel.index} problems={problems} dispatch={dispatch} /> : null}
      <output data-testid="def">{JSON.stringify(state.def)}</output>
    </>
  );
}

const current = (): WorkflowDef => JSON.parse(screen.getByTestId("def").textContent!);
const outline = () => screen.getByRole("navigation", { name: "Workflow outline" });
const pick = (user: ReturnType<typeof userEvent.setup>, name: string) =>
  user.click(within(outline()).getAllByRole("button", { name: new RegExp(`^${name}\\b`) })[0]);

describe("WorkflowOutline", () => {
  it("lists states, transitions and fields and marks problems", () => {
    render(<Harness problems={[{ path: "transitions[1].to", message: "bad" }]} />);
    expect(within(outline()).getByRole("button", { name: /^New/ })).toHaveTextContent("initial");
    expect(within(outline()).getByRole("button", { name: /^Hire\b/ })).toHaveTextContent("screening → hired");
    expect(within(outline()).getByLabelText("1 problem")).toBeInTheDocument();
  });

  it("adds a state and opens it", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await user.click(screen.getByRole("button", { name: "Add state" }));
    expect(screen.getByRole("heading", { name: "State: New state" })).toBeInTheDocument();
  });
});

describe("StateInspector", () => {
  it("renames a state and updates transitions", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await pick(user, "Screening");
    const key = within(screen.getByRole("region", { name: "State settings" })).getByLabelText("Key");
    await user.clear(key);
    await user.type(key, "review");
    expect(current().transitions[0].to).toBe("review");
  });

  it("deletes a state together with its dependent transitions", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await pick(user, "Screening");
    await user.click(screen.getByRole("button", { name: "Delete state" }));
    expect(current().transitions.map((t) => t.key)).toEqual(["reject"]);
  });

  it("sets the initial state", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await pick(user, "Screening");
    await user.click(screen.getByLabelText("Initial state (new submissions start here)"));
    expect(current().initial).toBe("screening");
  });
});

describe("TransitionInspector", () => {
  it("edits sources, disables terminal sources, and edits guards", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await pick(user, "Start screening");
    const region = screen.getByRole("region", { name: "Transition settings" });
    expect(within(region).getByLabelText("Hired (hired) · terminal")).toBeDisabled();
    await user.click(within(region).getByLabelText("Screening (screening)"));
    expect(current().transitions[0].from).toEqual(["new", "screening"]);
    await user.type(within(region).getByLabelText("Roles allowed"), "lead{Enter}");
    expect(current().transitions[0].guard.roles).toEqual(["reviewer", "lead"]);
    await user.click(within(region).getByLabelText("Score (score)"));
    expect(current().transitions[0].guard.requireFields).toEqual(["score"]);
  });

  it("edits, adds and switches actions", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await pick(user, "Reject");
    const region = screen.getByRole("region", { name: "Transition settings" });
    const email = within(region).getByRole("group", { name: "Action 1: Send email" });
    await user.clear(within(email).getByLabelText("Subject"));
    await user.type(within(email).getByLabelText("Subject"), "About your application");
    expect(current().transitions[2].actions?.[0].subject).toBe("About your application");

    await user.selectOptions(within(region).getByLabelText("New action type"), "assign");
    await user.click(within(region).getByRole("button", { name: "Add action" }));
    const assign = within(region).getByRole("group", { name: "Action 2: Assign reviewer" });
    await user.selectOptions(within(assign).getByLabelText("Assign to"), "user");
    await user.type(within(assign).getByLabelText("User email"), "lead@example.com");
    expect(current().transitions[2].actions?.[1]).toEqual({ type: "assign", user: "lead@example.com" });

    await user.click(within(region).getByRole("button", { name: "Remove action 1" }));
    expect(current().transitions[2].actions).toHaveLength(1);
  });
});

describe("WorkflowFieldInspector and WorkflowMetaPanel", () => {
  it("renames a workflow field and keeps requireFields in sync", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await pick(user, "Score");
    const key = within(screen.getByRole("region", { name: "Workflow field settings" })).getByLabelText("Key");
    await user.clear(key);
    await user.type(key, "rating");
    expect(current().transitions[1].guard.requireFields).toEqual(["rating"]);
  });

  it("adds onSubmit actions from workflow settings", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    const region = screen.getByRole("region", { name: "Workflow settings" });
    expect(within(region).getByLabelText("Slug")).toHaveAttribute("readonly");
    await user.selectOptions(within(region).getByLabelText("New action type"), "assign");
    await user.click(within(region).getByRole("button", { name: "Add action" }));
    await user.type(within(region).getByLabelText("Role"), "reviewer");
    expect(current().onSubmit).toEqual([{ type: "assign", role: "reviewer" }]);
  });
});
