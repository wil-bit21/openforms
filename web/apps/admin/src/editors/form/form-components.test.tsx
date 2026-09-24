import { useReducer } from "react";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { formEditorReducer, initFormEditor } from "./formReducer";
import { FieldList } from "./FieldList";
import { FieldInspector } from "./FieldInspector";
import { FormMetaPanel } from "./FormMetaPanel";
import { sampleForm } from "../test/samples";
import type { FormDef, Problem } from "../shared/types";

function Harness({ def = sampleForm(), problems = [], isNew = false }: { def?: FormDef; problems?: Problem[]; isNew?: boolean }) {
  const [state, dispatch] = useReducer(formEditorReducer, def, initFormEditor);
  return (
    <>
      <FieldList fields={state.def.fields} selected={state.selected} problems={problems} dispatch={dispatch} />
      {state.selected === null ? (
        <FormMetaPanel def={state.def} isNew={isNew} workflowSlugs={["hiring"]} problems={problems} dispatch={dispatch} />
      ) : (
        <FieldInspector def={state.def} index={state.selected} problems={problems} dispatch={dispatch} />
      )}
      <output data-testid="def">{JSON.stringify(state.def)}</output>
    </>
  );
}

const current = (): FormDef => JSON.parse(screen.getByTestId("def").textContent!);
const nav = () => screen.getByRole("navigation", { name: "Fields" });
const selectField = (user: ReturnType<typeof userEvent.setup>, label: string) =>
  user.click(within(nav()).getAllByRole("button", { name: new RegExp(`^${label}`) })[0]);
const inspector = () => screen.getByRole("region", { name: "Field settings" });

describe("FieldList", () => {
  it("adds a field of the chosen type and selects it", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await user.selectOptions(screen.getByLabelText("New field type"), "select");
    await user.click(screen.getByRole("button", { name: "Add field" }));
    expect(current().fields[4]).toMatchObject({ key: "field1", type: "select" });
    expect(within(inspector()).getByRole("heading", { name: "Untitled question" })).toBeInTheDocument();
  });

  it("reorders and deletes fields", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await user.click(screen.getByRole("button", { name: "Move Email down" }));
    expect(current().fields.map((f) => f.key)).toEqual(["name", "role", "email", "portfolio"]);
    await user.click(screen.getByRole("button", { name: "Delete Portfolio URL" }));
    expect(current().fields).toHaveLength(3);
  });

  it("marks fields that have problems", () => {
    render(<Harness problems={[{ path: "fields[0].label", message: "label is bad" }]} />);
    expect(within(nav()).getByLabelText("1 problem")).toBeInTheDocument();
  });
});

describe("FieldInspector", () => {
  it("shows problems next to the matching control", async () => {
    const user = userEvent.setup();
    render(<Harness problems={[{ path: "fields[0].label", message: "label is bad" }]} />);
    await selectField(user, "Full name");
    expect(within(inspector()).getByText("label is bad")).toBeInTheDocument();
    expect(within(inspector()).getByLabelText("Label")).toHaveAttribute("aria-invalid", "true");
  });

  it("renaming a key updates conditions that use it", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await selectField(user, "Role");
    const key = within(inspector()).getByLabelText("Key");
    await user.clear(key);
    await user.type(key, "position");
    expect(current().fields[3].showIf).toEqual({ field: "position", equals: "designer" });
  });

  it("changing type to dropdown adds options and removes text validation", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await selectField(user, "Full name");
    await user.selectOptions(within(inspector()).getByLabelText("Type"), "select");
    expect(current().fields[0].options).toHaveLength(2);
    expect(current().fields[0].validation).toBeUndefined();
    expect(within(inspector()).getByRole("group", { name: "Options" })).toBeInTheDocument();
  });
});

describe("ShowIfBuilder", () => {
  it("only offers fields declared earlier", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await selectField(user, "Email");
    await user.click(within(inspector()).getByLabelText("Only show this field when a previous answer matches"));
    const options = within(within(inspector()).getByLabelText("Previous field")).getAllByRole("option");
    expect(options.map((o) => (o as HTMLOptionElement).value)).toEqual(["name"]);
    expect(current().fields[1].showIf).toEqual({ field: "name", equals: "" });

    await selectField(user, "Portfolio URL");
    const portfolioOptions = within(within(inspector()).getByLabelText("Previous field")).getAllByRole("option");
    expect(portfolioOptions.map((o) => (o as HTMLOptionElement).value)).toEqual(["name", "email", "role"]);
  });

  it("uses the controlling field's options for values and supports 'is one of'", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await selectField(user, "Portfolio URL");
    expect(within(inspector()).getByLabelText("Value")).toHaveValue("designer");
    await user.selectOptions(within(inspector()).getByLabelText("Condition"), "in");
    expect(current().fields[3].showIf).toEqual({ field: "role", in: ["designer"] });
    await user.click(within(inspector()).getByLabelText("Engineer"));
    expect(current().fields[3].showIf).toEqual({ field: "role", in: ["designer", "engineer"] });
  });

  it("removes the condition when unchecked", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await selectField(user, "Portfolio URL");
    await user.click(within(inspector()).getByLabelText("Only show this field when a previous answer matches"));
    expect(current().fields[3].showIf).toBeUndefined();
  });

  it("explains that the first field cannot be conditional", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await selectField(user, "Full name");
    expect(within(inspector()).getByText("Conditional display is available for fields after the first one.")).toBeInTheDocument();
  });
});

describe("FormMetaPanel", () => {
  it("locks the slug for existing forms", () => {
    render(<Harness />);
    expect(screen.getByLabelText("Slug")).toHaveAttribute("readonly");
  });

  it("edits slug, workflow and publishing for new forms", async () => {
    const user = userEvent.setup();
    render(<Harness isNew def={{ slug: "", title: "Untitled form", settings: { public: true }, fields: [] }} />);
    await user.type(screen.getByLabelText("Slug"), "contact");
    await user.selectOptions(screen.getByLabelText("Workflow"), "hiring");
    await user.click(screen.getByLabelText("Public (anyone with the link can submit)"));
    expect(current()).toMatchObject({ slug: "contact", workflow: "hiring", settings: { public: false } });
  });
});
