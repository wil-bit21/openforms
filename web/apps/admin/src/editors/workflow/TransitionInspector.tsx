import type { Dispatch } from "react";
import { CheckboxControl, FieldErrors, SelectControl, TextControl } from "../shared/controls";
import { problemsAt, problemsUnder } from "../shared/problems";
import { TagInput } from "../shared/TagInput";
import type { Problem, WorkflowDef } from "../shared/types";
import { ActionsEditor } from "./ActionsEditor";
import type { WorkflowEditorAction } from "./workflowReducer";

export function TransitionInspector({
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
  const t = def.transitions[index];
  if (!t) return null;
  const base = `transitions[${index}]`;
  const at = (prop: string) => problemsAt(problems, `${base}.${prop}`);
  const target = { kind: "transition" as const, index };
  const fields = def.fields ?? [];
  const toOptions = def.states.map((s) => ({ value: s.key, label: `${s.label} (${s.key})` }));
  if (!def.states.some((s) => s.key === t.to)) toOptions.unshift({ value: t.to, label: `${t.to || "(none)"} (unknown state)` });

  return (
    <section className="of-ed-inspector" aria-label="Transition settings">
      <h2>Transition: {t.label || t.key}</h2>
      <TextControl label="Label" value={t.label} problems={at("label")} onChange={(v) => dispatch({ type: "updateTransition", index, patch: { label: v } })} />
      <TextControl
        label="Key"
        monospace
        value={t.key}
        problems={at("key")}
        hint="Used by the API: POST /submissions/{id}/transitions with this key."
        onChange={(v) => dispatch({ type: "updateTransition", index, patch: { key: v } })}
      />
      <fieldset className="of-ed-fieldset">
        <legend>From states</legend>
        <FieldErrors problems={problemsUnder(problems, `${base}.from`)} />
        {def.states.map((s) => {
          const checked = t.from.includes(s.key);
          return (
            <CheckboxControl
              key={s.key}
              label={`${s.label} (${s.key})${s.terminal ? " · terminal" : ""}`}
              checked={checked}
              disabled={Boolean(s.terminal) && !checked}
              onChange={(on) =>
                dispatch({ type: "updateTransition", index, patch: { from: on ? [...t.from, s.key] : t.from.filter((k) => k !== s.key) } })
              }
            />
          );
        })}
      </fieldset>
      <SelectControl label="To state" value={t.to} options={toOptions} problems={at("to")} onChange={(v) => dispatch({ type: "updateTransition", index, patch: { to: v } })} />
      <fieldset className="of-ed-fieldset">
        <legend>Guard</legend>
        <TagInput
          label="Roles allowed"
          values={t.guard.roles ?? []}
          placeholder="Type a role and press Enter"
          hint="Admins can always perform this transition. Leave empty to allow any signed-in user."
          onChange={(roles) => dispatch({ type: "setGuard", index, guard: { ...t.guard, roles } })}
        />
        {fields.length === 0 ? (
          <p className="of-ed-hint">Add workflow fields to require them before this transition.</p>
        ) : (
          <fieldset className="of-ed-fieldset">
            <legend>Required fields</legend>
            <FieldErrors problems={problemsUnder(problems, `${base}.guard.requireFields`)} />
            {fields.map((f) => {
              const required = t.guard.requireFields ?? [];
              return (
                <CheckboxControl
                  key={f.key}
                  label={`${f.label} (${f.key})`}
                  checked={required.includes(f.key)}
                  onChange={(on) =>
                    dispatch({
                      type: "setGuard",
                      index,
                      guard: { ...t.guard, requireFields: on ? [...required, f.key] : required.filter((k) => k !== f.key) },
                    })
                  }
                />
              );
            })}
          </fieldset>
        )}
      </fieldset>
      <ActionsEditor
        title="Actions when this transition happens"
        actions={t.actions ?? []}
        basePath={`${base}.actions`}
        problems={problems}
        onAdd={(actionType) => dispatch({ type: "addAction", target, actionType })}
        onUpdate={(actionIndex, patch) => dispatch({ type: "updateAction", target, actionIndex, patch })}
        onRemove={(actionIndex) => dispatch({ type: "removeAction", target, actionIndex })}
      />
      <button type="button" className="of-ed-btn of-ed-btn--danger" onClick={() => dispatch({ type: "removeTransition", index })}>
        Delete transition
      </button>
    </section>
  );
}
