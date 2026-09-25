import { useId, useState, type ReactNode } from "react";

export function TagInput({
  label,
  values,
  onChange,
  placeholder,
  hint,
}: {
  label: string;
  values: string[];
  onChange: (next: string[]) => void;
  placeholder?: string;
  hint?: ReactNode;
}) {
  const id = useId();
  const [draft, setDraft] = useState("");

  const commit = (text: string) => {
    const parts = text.split(",").map((s) => s.trim()).filter(Boolean);
    setDraft("");
    if (parts.length === 0) return;
    const next = [...values];
    for (const part of parts) if (!next.includes(part)) next.push(part);
    if (next.length !== values.length) onChange(next);
  };

  return (
    <div className="of-ed-control">
      <label htmlFor={id}>{label}</label>
      <div className="of-ed-tags">
        <ul aria-label={`${label} values`}>
          {values.map((v) => (
            <li key={v} className="of-ed-tag">
              {v}
              <button type="button" aria-label={`Remove ${v}`} onClick={() => onChange(values.filter((x) => x !== v))}>
                ✕
              </button>
            </li>
          ))}
        </ul>
        <input
          id={id}
          className="of-ed-input"
          value={draft}
          placeholder={placeholder}
          aria-describedby={hint ? `${id}-hint` : undefined}
          onChange={(e) => {
            const next = e.target.value;
            if (next.includes(",")) commit(next);
            else setDraft(next);
          }}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              commit(draft);
            } else if (e.key === "Backspace" && draft === "" && values.length > 0) {
              onChange(values.slice(0, -1));
            }
          }}
          onBlur={() => commit(draft)}
        />
      </div>
      {hint ? (
        <p id={`${id}-hint`} className="of-ed-hint">
          {hint}
        </p>
      ) : null}
    </div>
  );
}
