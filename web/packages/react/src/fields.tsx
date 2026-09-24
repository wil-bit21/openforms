import type { Field, FieldType } from "@openforms/sdk";
import type { ComponentType } from "react";

export interface FieldProps {
  field: Field;
  /** DOM id for the primary input; the surrounding <label htmlFor> points at it. */
  id: string;
  value: unknown;
  onChange(value: unknown): void;
  invalid: boolean;
  /** Space-separated ids of help/error text, for aria-describedby. */
  describedBy?: string;
  required: boolean;
  disabled: boolean;
}

export type FieldComponent = ComponentType<FieldProps>;
export type FieldComponents = Partial<Record<FieldType, FieldComponent>>;

const asString = (v: unknown) => (typeof v === "string" ? v : "");

function inputProps(p: FieldProps) {
  return {
    id: p.id,
    name: p.field.key,
    "aria-invalid": p.invalid ? ("true" as const) : undefined,
    "aria-describedby": p.describedBy,
    "aria-required": p.required ? ("true" as const) : undefined,
    disabled: p.disabled,
  };
}

function StringInput({ type, ...p }: FieldProps & { type: string }) {
  return (
    <input
      className="of-input"
      type={type}
      {...inputProps(p)}
      placeholder={p.field.placeholder}
      value={asString(p.value)}
      onChange={(e) => p.onChange(e.target.value)}
    />
  );
}

export function TextInput(p: FieldProps) {
  return <StringInput type="text" {...p} />;
}

export function EmailInput(p: FieldProps) {
  return <StringInput type="email" {...p} />;
}

export function UrlInput(p: FieldProps) {
  return <StringInput type="url" {...p} />;
}

export function DateInput(p: FieldProps) {
  return <StringInput type="date" {...p} />;
}

export function TextArea(p: FieldProps) {
  return (
    <textarea
      className="of-input of-textarea"
      rows={5}
      {...inputProps(p)}
      placeholder={p.field.placeholder}
      value={asString(p.value)}
      onChange={(e) => p.onChange(e.target.value)}
    />
  );
}

export function NumberInput(p: FieldProps) {
  // Uncontrolled so partially typed numbers ("1.") are not clobbered by re-renders.
  return (
    <input
      className="of-input"
      type="number"
      inputMode="decimal"
      {...inputProps(p)}
      placeholder={p.field.placeholder}
      defaultValue={typeof p.value === "number" ? String(p.value) : ""}
      onChange={(e) => {
        const raw = e.target.value;
        const n = raw === "" ? undefined : Number(raw);
        p.onChange(n === undefined || Number.isNaN(n) ? undefined : n);
      }}
    />
  );
}

export function Select(p: FieldProps) {
  return (
    <select
      className="of-input of-select"
      {...inputProps(p)}
      value={asString(p.value)}
      onChange={(e) => p.onChange(e.target.value === "" ? undefined : e.target.value)}
    >
      <option value="">{p.field.placeholder || "Select…"}</option>
      {(p.field.options ?? []).map((o) => (
        <option key={o.value} value={o.value}>
          {o.label}
        </option>
      ))}
    </select>
  );
}

export function MultiSelect(p: FieldProps) {
  const options = p.field.options ?? [];
  const selected = Array.isArray(p.value) ? (p.value as string[]) : [];
  const toggle = (value: string, on: boolean) => {
    // Keep the option order stable regardless of click order.
    const next = options.map((o) => o.value).filter((v) => (v === value ? on : selected.includes(v)));
    p.onChange(next.length > 0 ? next : undefined);
  };
  return (
    <div className="of-choices">
      {options.map((o, i) => (
        <label key={o.value} className="of-choice">
          <input
            type="checkbox"
            className="of-checkbox"
            id={i === 0 ? p.id : `${p.id}-${i}`}
            name={p.field.key}
            value={o.value}
            checked={selected.includes(o.value)}
            disabled={p.disabled}
            aria-invalid={p.invalid ? "true" : undefined}
            onChange={(e) => toggle(o.value, e.target.checked)}
          />
          <span>{o.label}</span>
        </label>
      ))}
    </div>
  );
}

export function Checkbox(p: FieldProps) {
  return (
    <input
      className="of-checkbox"
      type="checkbox"
      {...inputProps(p)}
      checked={p.value === true}
      onChange={(e) => p.onChange(e.target.checked)}
    />
  );
}

export const defaultComponents: Record<FieldType, FieldComponent> = {
  text: TextInput,
  textarea: TextArea,
  email: EmailInput,
  number: NumberInput,
  select: Select,
  multiselect: MultiSelect,
  checkbox: Checkbox,
  date: DateInput,
  url: UrlInput,
};
