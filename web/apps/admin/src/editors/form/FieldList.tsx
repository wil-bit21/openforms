import { useState, type Dispatch } from "react";
import { SelectControl } from "../shared/controls";
import { problemsUnder } from "../shared/problems";
import { FIELD_TYPES, FIELD_TYPE_LABELS, type Field, type FieldType, type Problem } from "../shared/types";
import type { FormEditorAction } from "./formReducer";

function ProblemBadge({ count }: { count: number }) {
  if (count === 0) return null;
  return (
    <span className="of-ed-badge" aria-label={count === 1 ? "1 problem" : `${count} problems`}>
      {count}
    </span>
  );
}

export function FieldList({
  fields,
  selected,
  problems,
  dispatch,
}: {
  fields: Field[];
  selected: number | null;
  problems: Problem[];
  dispatch: Dispatch<FormEditorAction>;
}) {
  const [newType, setNewType] = useState<FieldType>("text");
  const settingsProblems = problems.filter((p) => !p.path.startsWith("fields")).length;

  return (
    <nav className="of-ed-list" aria-label="Fields">
      <button
        type="button"
        className="of-ed-list__item"
        aria-current={selected === null ? "true" : undefined}
        onClick={() => dispatch({ type: "select", index: null })}
      >
        <span>
          Form settings
          <ProblemBadge count={settingsProblems} />
        </span>
      </button>
      <h3>Questions</h3>
      {fields.length === 0 ? <p className="of-ed-hint">No questions yet. Add the first one below.</p> : null}
      <ol>
        {fields.map((field, i) => {
          const name = field.label || field.key || `Question ${i + 1}`;
          return (
            <li key={i}>
              <button
                type="button"
                className="of-ed-list__item"
                aria-current={selected === i ? "true" : undefined}
                onClick={() => dispatch({ type: "select", index: i })}
              >
                <span>
                  {name}
                  <ProblemBadge count={problemsUnder(problems, `fields[${i}]`).length} />
                </span>
                <small>
                  {field.key} · {FIELD_TYPE_LABELS[field.type] ?? field.type}
                  {field.required ? " · required" : ""}
                  {field.showIf ? " · conditional" : ""}
                </small>
              </button>
              <div className="of-ed-list__actions">
                <button type="button" className="of-ed-icon-btn" aria-label={`Move ${name} up`} disabled={i === 0} onClick={() => dispatch({ type: "moveField", index: i, direction: -1 })}>
                  ↑
                </button>
                <button type="button" className="of-ed-icon-btn" aria-label={`Move ${name} down`} disabled={i === fields.length - 1} onClick={() => dispatch({ type: "moveField", index: i, direction: 1 })}>
                  ↓
                </button>
                <button type="button" className="of-ed-icon-btn" aria-label={`Duplicate ${name}`} onClick={() => dispatch({ type: "duplicateField", index: i })}>
                  ⧉
                </button>
                <button type="button" className="of-ed-icon-btn" aria-label={`Delete ${name}`} onClick={() => dispatch({ type: "removeField", index: i })}>
                  ✕
                </button>
              </div>
            </li>
          );
        })}
      </ol>
      <div className="of-ed-add">
        <SelectControl
          label="New field type"
          value={newType}
          options={FIELD_TYPES.map((t) => ({ value: t, label: FIELD_TYPE_LABELS[t] }))}
          onChange={(v) => setNewType(v as FieldType)}
        />
        <button type="button" className="of-ed-btn" onClick={() => dispatch({ type: "addField", fieldType: newType })}>
          Add field
        </button>
      </div>
    </nav>
  );
}
