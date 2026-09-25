import type { Dispatch } from "react";
import { CheckboxControl, SelectControl, TextControl } from "../shared/controls";
import { problemsAt } from "../shared/problems";
import { STATE_COLORS, type Problem, type StateColor, type WorkflowDef } from "../shared/types";
import type { WorkflowEditorAction } from "./workflowReducer";

const capitalize = (s: string) => s.charAt(0).toUpperCase() + s.slice(1);

export function StateInspector({
  def,
  index,
  problems,
  dispatch,
}: {
  def: WorkflowDef;
  index: number;
  problems: Problem[];
  dispatch: Dispatch<WorkflowEditorAction>;
}) {
  const state = def.states[index];
  if (!state) return null;
  const at = (prop: string) => problemsAt(problems, `states[${index}].${prop}`);
  const isInitial = def.initial === state.key;
  return (
    <section className="of-ed-inspector" aria-label="State settings">
      <h2>State: {state.label || state.key}</h2>
      <TextControl label="Label" value={state.label} problems={at("label")} onChange={(v) => dispatch({ type: "updateState", index, patch: { label: v } })} />
      <TextControl
        label="Key"
        monospace
        value={state.key}
        problems={at("key")}
        hint="Stored on submissions. Renaming updates transitions and the initial state; existing submissions keep their old state key."
        onChange={(v) => dispatch({ type: "renameState", index, key: v })}
      />
      <SelectControl
        label="Color"
        value={state.color ?? "gray"}
        options={STATE_COLORS.map((c) => ({ value: c, label: capitalize(c) }))}
        onChange={(c) => dispatch({ type: "updateState", index, patch: { color: c as StateColor } })}
      />
      <CheckboxControl
        label="Terminal (no transitions can leave this state)"
        checked={Boolean(state.terminal)}
        onChange={(checked) => dispatch({ type: "updateState", index, patch: { terminal: checked } })}
      />
      <CheckboxControl
        label="Initial state (new submissions start here)"
        checked={isInitial}
        disabled={isInitial}
        hint={isInitial ? "To change it, mark another state as initial." : undefined}
        onChange={(checked) => checked && dispatch({ type: "setInitial", key: state.key })}
      />
      <button type="button" className="of-ed-btn of-ed-btn--danger" onClick={() => dispatch({ type: "removeState", index })}>
        Delete state
      </button>
      <p className="of-ed-hint">Deleting a state also deletes transitions into it and removes it from other transitions' sources.</p>
    </section>
  );
}
