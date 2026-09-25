import type { Dispatch } from "react";
import { SelectControl, TextControl } from "../shared/controls";
import { problemsAt } from "../shared/problems";
import type { Problem, WorkflowDef } from "../shared/types";
import { ActionsEditor } from "./ActionsEditor";
import type { WorkflowEditorAction } from "./workflowReducer";

export function WorkflowMetaPanel({
  def,
  isNew,
  problems,
  dispatch,
}: {
  def: WorkflowDef;
  isNew: boolean;
  problems: Problem[];
  dispatch: Dispatch<WorkflowEditorAction>;
}) {
  const at = (path: string) => problemsAt(problems, path);
  const target = { kind: "onSubmit" as const };
  const initialOptions = def.states.map((s) => ({ value: s.key, label: `${s.label} (${s.key})` }));
  if (!def.states.some((s) => s.key === def.initial)) initialOptions.unshift({ value: def.initial, label: `${def.initial || "(none)"} (unknown state)` });

  return (
    <section className="of-ed-inspector" aria-label="Workflow settings">
      <h2>Workflow settings</h2>
      <TextControl
        label="Slug"
        monospace
        value={def.slug}
        readOnly={!isNew}
        problems={at("slug")}
        hint={isNew ? "Forms reference the workflow by this slug. It can't be changed later." : "The slug can't be changed after the workflow is created."}
        onChange={(v) => dispatch({ type: "setMeta", patch: { slug: v } })}
      />
      <TextControl label="Title" value={def.title} problems={at("title")} onChange={(v) => dispatch({ type: "setMeta", patch: { title: v } })} />
      <SelectControl label="Initial state" value={def.initial} options={initialOptions} problems={at("initial")} onChange={(key) => dispatch({ type: "setInitial", key })} />
      <ActionsEditor
        title="Actions when a submission is created"
        actions={def.onSubmit ?? []}
        basePath="onSubmit"
        problems={problems}
        onAdd={(actionType) => dispatch({ type: "addAction", target, actionType })}
        onUpdate={(actionIndex, patch) => dispatch({ type: "updateAction", target, actionIndex, patch })}
        onRemove={(actionIndex) => dispatch({ type: "removeAction", target, actionIndex })}
      />
    </section>
  );
}
