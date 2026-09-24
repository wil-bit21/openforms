import type { Dispatch } from "react";
import { SelectControl, TextControl } from "../shared/controls";
import { OptionsEditor } from "../shared/OptionsEditor";
import { problemsAt } from "../shared/problems";
import { FIELD_TYPE_LABELS, WORKFLOW_FIELD_TYPES, type FieldType, type Problem, type WorkflowDef } from "../shared/types";
import type { WorkflowEditorAction } from "./workflowReducer";

export function WorkflowFieldInspector({
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
  const field = def.fields?.[index];
  if (!field) return null;
  const base = `fields[${index}]`;
  const at = (prop: string) => problemsAt(problems, `${base}.${prop}`);
  return (
    <section className="of-ed-inspector" aria-label="Workflow field settings">
      <h2>Field: {field.label || field.key}</h2>
      <p className="of-ed-hint">Workflow fields are filled in by reviewers, not respondents.</p>
      <TextControl label="Label" value={field.label} problems={at("label")} onChange={(v) => dispatch({ type: "updateField", index, patch: { label: v } })} />
      <TextControl
        label="Key"
        monospace
        value={field.key}
        problems={at("key")}
        hint="Renaming updates transitions that require this field."
        onChange={(v) => dispatch({ type: "renameField", index, key: v })}
      />
      <SelectControl
        label="Type"
        value={field.type}
        options={WORKFLOW_FIELD_TYPES.map((t) => ({ value: t, label: FIELD_TYPE_LABELS[t] }))}
        problems={at("type")}
        onChange={(t) => dispatch({ type: "updateField", index, patch: { type: t as FieldType } })}
      />
      {field.type === "select" ? (
        <OptionsEditor
          options={field.options ?? []}
          basePath={`${base}.options`}
          problems={problems}
          onChange={(options) => dispatch({ type: "updateField", index, patch: { options } })}
        />
      ) : null}
      <button type="button" className="of-ed-btn of-ed-btn--danger" onClick={() => dispatch({ type: "removeField", index })}>
        Delete field
      </button>
    </section>
  );
}
