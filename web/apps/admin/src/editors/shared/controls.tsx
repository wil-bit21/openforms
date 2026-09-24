import { useId, type ChangeEvent, type ReactNode } from "react";
import type { Problem } from "./types";

export function FieldErrors({ id, problems }: { id?: string; problems: Problem[] }) {
  if (problems.length === 0) return null;
  return (
    <ul id={id} className="of-ed-errors">
      {problems.map((p, i) => (
        <li key={`${i}-${p.message}`}>{p.message}</li>
      ))}
    </ul>
  );
}

interface ControlBase {
  label: string;
  problems?: Problem[];
  hint?: ReactNode;
}

function useDescription(problems: Problem[], hint: ReactNode | undefined) {
  const id = useId();
  const hintId = `${id}-hint`;
  const errId = `${id}-err`;
  const describedBy = [hint ? hintId : "", problems.length ? errId : ""].filter(Boolean).join(" ") || undefined;
  return { id, hintId, errId, describedBy, invalid: problems.length > 0 ? true : undefined };
}

function ControlFooter(props: { hint?: ReactNode; hintId: string; errId: string; problems: Problem[] }) {
  return (
    <>
      {props.hint ? (
        <p id={props.hintId} className="of-ed-hint">
          {props.hint}
        </p>
      ) : null}
      <FieldErrors id={props.errId} problems={props.problems} />
    </>
  );
}

export function TextControl({
  label,
  value,
  onChange,
  problems = [],
  hint,
  multiline = false,
  readOnly = false,
  placeholder,
  monospace = false,
}: ControlBase & {
  value: string | undefined;
  onChange: (value: string) => void;
  multiline?: boolean;
  readOnly?: boolean;
  placeholder?: string;
  monospace?: boolean;
}) {
  const d = useDescription(problems, hint);
  const shared = {
    id: d.id,
    value: value ?? "",
    readOnly,
    placeholder,
    "aria-invalid": d.invalid,
    "aria-describedby": d.describedBy,
    className: monospace ? "of-ed-input of-ed-input--mono" : "of-ed-input",
  };
  return (
    <div className="of-ed-control">
      <label htmlFor={d.id}>{label}</label>
      {multiline ? (
        <textarea rows={4} {...shared} onChange={(e: ChangeEvent<HTMLTextAreaElement>) => onChange(e.target.value)} />
      ) : (
        <input type="text" {...shared} onChange={(e: ChangeEvent<HTMLInputElement>) => onChange(e.target.value)} />
      )}
      <ControlFooter hint={hint} hintId={d.hintId} errId={d.errId} problems={problems} />
    </div>
  );
}

export function NumberControl({
  label,
  value,
  onChange,
  problems = [],
  hint,
  min,
}: ControlBase & { value: number | undefined; onChange: (value: number | undefined) => void; min?: number }) {
  const d = useDescription(problems, hint);
  return (
    <div className="of-ed-control">
      <label htmlFor={d.id}>{label}</label>
      <input
        id={d.id}
        type="number"
        className="of-ed-input"
        min={min}
        value={value ?? ""}
        aria-invalid={d.invalid}
        aria-describedby={d.describedBy}
        onChange={(e) => {
          const raw = e.target.value;
          const n = Number(raw);
          onChange(raw === "" || !Number.isFinite(n) ? undefined : n);
        }}
      />
      <ControlFooter hint={hint} hintId={d.hintId} errId={d.errId} problems={problems} />
    </div>
  );
}

export function CheckboxControl({
  label,
  checked,
  onChange,
  disabled = false,
  hint,
}: { label: string; checked: boolean; onChange: (checked: boolean) => void; disabled?: boolean; hint?: ReactNode }) {
  const d = useDescription([], hint);
  return (
    <div className="of-ed-control of-ed-control--checkbox">
      <input
        id={d.id}
        type="checkbox"
        checked={checked}
        disabled={disabled}
        aria-describedby={d.describedBy}
        onChange={(e) => onChange(e.target.checked)}
      />
      <label htmlFor={d.id}>{label}</label>
      {hint ? (
        <p id={d.hintId} className="of-ed-hint">
          {hint}
        </p>
      ) : null}
    </div>
  );
}

export function SelectControl({
  label,
  value,
  options,
  onChange,
  problems = [],
  hint,
}: ControlBase & { value: string; options: { value: string; label: string }[]; onChange: (value: string) => void }) {
  const d = useDescription(problems, hint);
  return (
    <div className="of-ed-control">
      <label htmlFor={d.id}>{label}</label>
      <select
        id={d.id}
        className="of-ed-input"
        value={value}
        aria-invalid={d.invalid}
        aria-describedby={d.describedBy}
        onChange={(e) => onChange(e.target.value)}
      >
        {options.map((o) => (
          <option key={o.value} value={o.value}>
            {o.label}
          </option>
        ))}
      </select>
      <ControlFooter hint={hint} hintId={d.hintId} errId={d.errId} problems={problems} />
    </div>
  );
}
