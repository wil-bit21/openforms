import type { ReactNode } from "react";

export type WorkflowFieldLike = { key: string; type: string; label: string; options?: { value: string; label: string }[] };

export function isEmpty(v: unknown): boolean {
  return v === undefined || v === null || v === "";
}

const str = (v: unknown) => (v === null || v === undefined ? "" : String(v));

export function WorkflowFieldInput({ field, value, onChange, error, idPrefix }: {
  field: WorkflowFieldLike;
  value: unknown;
  onChange: (v: unknown) => void;
  error?: string;
  idPrefix: string;
}) {
  const id = `${idPrefix}-${field.key}`;
  const errorId = `${id}-error`;
  const common = { id, "aria-invalid": error ? true : undefined, "aria-describedby": error ? errorId : undefined };
  const errorNode = error ? <p id={errorId} className="field-error">{error}</p> : null;

  if (field.type === "checkbox") {
    return (
      <div className="field field-checkbox">
        <label htmlFor={id}>
          <input {...common} type="checkbox" checked={value === true} onChange={(e) => onChange(e.target.checked)} /> {field.label}
        </label>
        {errorNode}
      </div>
    );
  }

  let input: ReactNode;
  switch (field.type) {
    case "textarea":
      input = <textarea {...common} rows={3} value={str(value)} onChange={(e) => onChange(e.target.value)} />;
      break;
    case "number":
      input = (
        <input {...common} type="number" value={str(value)}
          onChange={(e) => onChange(e.target.value === "" ? null : Number(e.target.value))} />
      );
      break;
    case "select":
      input = (
        <select {...common} value={str(value)} onChange={(e) => onChange(e.target.value === "" ? null : e.target.value)}>
          <option value="">—</option>
          {field.options?.map((o) => (
            <option key={o.value} value={o.value}>{o.label}</option>
          ))}
        </select>
      );
      break;
    case "date":
      input = <input {...common} type="date" value={str(value)} onChange={(e) => onChange(e.target.value === "" ? null : e.target.value)} />;
      break;
    default:
      input = <input {...common} type="text" value={str(value)} onChange={(e) => onChange(e.target.value)} />;
  }

  return (
    <div className="field">
      <label htmlFor={id}>{field.label}</label>
      {input}
      {errorNode}
    </div>
  );
}
