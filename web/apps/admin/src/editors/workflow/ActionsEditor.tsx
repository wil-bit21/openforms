import { useState } from "react";
import { FieldErrors, SelectControl, TextControl } from "../shared/controls";
import { problemsAt } from "../shared/problems";
import { ACTION_LABELS, ACTION_TYPES, type Action, type ActionType, type Problem } from "../shared/types";

export const TEMPLATE_HINT =
  "Placeholders: {{submission.data.<key>}}, {{submission.fields.<key>}}, {{submission.id}}, {{submission.stateLabel}}, {{submission.url}}, {{form.title}}, {{transition.label}}.";

export function ActionsEditor({
  title,
  actions,
  basePath,
  problems,
  onAdd,
  onUpdate,
  onRemove,
}: {
  title: string;
  actions: Action[];
  basePath: string;
  problems: Problem[];
  onAdd: (type: ActionType) => void;
  onUpdate: (index: number, patch: Partial<Action>) => void;
  onRemove: (index: number) => void;
}) {
  const [newType, setNewType] = useState<ActionType>("email");
  return (
    <fieldset className="of-ed-fieldset">
      <legend>{title}</legend>
      {actions.length === 0 ? <p className="of-ed-hint">No actions.</p> : null}
      {actions.map((action, i) => {
        const base = `${basePath}[${i}]`;
        const at = (key: string) => problemsAt(problems, `${base}.${key}`);
        const name = ACTION_LABELS[action.type] ?? action.type;
        return (
          <div key={i} className="of-ed-card" role="group" aria-label={`Action ${i + 1}: ${name}`}>
            <div className="of-ed-card__head">
              <strong>{name}</strong>
              <button type="button" className="of-ed-icon-btn" aria-label={`Remove action ${i + 1}`} onClick={() => onRemove(i)}>
                ✕
              </button>
            </div>
            <FieldErrors problems={[...problemsAt(problems, base), ...at("type")]} />
            {action.type === "webhook" ? (
              <TextControl
                label="URL"
                monospace
                value={action.url}
                problems={at("url")}
                hint="Receives a signed JSON POST. Failed deliveries are retried with backoff."
                onChange={(v) => onUpdate(i, { url: v })}
              />
            ) : null}
            {action.type === "email" ? (
              <>
                <TextControl label="To" value={action.to} problems={at("to")} onChange={(v) => onUpdate(i, { to: v })} />
                <TextControl label="Subject" value={action.subject} problems={at("subject")} onChange={(v) => onUpdate(i, { subject: v })} />
                <TextControl label="Body" multiline value={action.body} problems={at("body")} hint={TEMPLATE_HINT} onChange={(v) => onUpdate(i, { body: v })} />
              </>
            ) : null}
            {action.type === "assign" ? (
              <>
                <SelectControl
                  label="Assign to"
                  value={action.user !== undefined ? "user" : "role"}
                  options={[
                    { value: "role", label: "Least busy person with a role" },
                    { value: "user", label: "A specific person (email)" },
                  ]}
                  onChange={(mode) => onUpdate(i, mode === "user" ? { user: "", role: undefined } : { role: "", user: undefined })}
                />
                {action.user !== undefined ? (
                  <TextControl label="User email" value={action.user} problems={at("user")} onChange={(v) => onUpdate(i, { user: v })} />
                ) : (
                  <TextControl label="Role" value={action.role} problems={at("role")} onChange={(v) => onUpdate(i, { role: v })} />
                )}
              </>
            ) : null}
          </div>
        );
      })}
      <div className="of-ed-add">
        <SelectControl
          label="New action type"
          value={newType}
          options={ACTION_TYPES.map((t) => ({ value: t, label: ACTION_LABELS[t] }))}
          onChange={(v) => setNewType(v as ActionType)}
        />
        <button type="button" className="of-ed-btn" onClick={() => onAdd(newType)}>
          Add action
        </button>
      </div>
    </fieldset>
  );
}
