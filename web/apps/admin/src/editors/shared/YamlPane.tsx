import { useId, useState } from "react";
import { parseYaml, toYaml } from "./yaml";

// Mounted fresh each time the YAML tab opens, so it always starts from the
// current definition. While the text is invalid, nothing is applied and the
// visual editor keeps the last valid definition.
export function YamlPane({ value, onApply }: { value: unknown; onApply: (next: Record<string, unknown>) => void }) {
  const id = useId();
  const [text, setText] = useState(() => toYaml(value));
  const [error, setError] = useState<string | null>(null);

  return (
    <div className="of-ed-yaml">
      <label htmlFor={id}>YAML</label>
      <textarea
        id={id}
        className="of-ed-input of-ed-input--mono"
        spellCheck={false}
        rows={30}
        value={text}
        aria-invalid={error ? true : undefined}
        aria-describedby={error ? `${id}-err` : `${id}-hint`}
        onChange={(e) => {
          const next = e.target.value;
          setText(next);
          const parsed = parseYaml<Record<string, unknown>>(next);
          if (parsed.ok) {
            setError(null);
            onApply(parsed.value);
          } else {
            setError(parsed.error);
          }
        }}
      />
      {error ? (
        <p id={`${id}-err`} role="alert" className="of-ed-errors">
          {error}
        </p>
      ) : (
        <p id={`${id}-hint`} className="of-ed-hint">
          Changes apply to the visual editor as you type.
        </p>
      )}
    </div>
  );
}
