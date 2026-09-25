import { FieldErrors, TextControl } from "./controls";
import { nextNumbered } from "./objects";
import { problemsAt } from "./problems";
import type { Option, Problem } from "./types";

export function OptionsEditor({
  options,
  basePath,
  problems,
  onChange,
}: {
  options: Option[];
  basePath: string;
  problems: Problem[];
  onChange: (next: Option[]) => void;
}) {
  const update = (index: number, patch: Partial<Option>) =>
    onChange(options.map((o, i) => (i === index ? { ...o, ...patch } : o)));
  const add = () => {
    const value = nextNumbered("option", options.map((o) => o.value));
    onChange([...options, { value, label: `Option ${value.slice("option".length)}` }]);
  };
  return (
    <fieldset className="of-ed-fieldset">
      <legend>Options</legend>
      <FieldErrors problems={problemsAt(problems, basePath)} />
      {options.map((o, i) => (
        <div className="of-ed-option" key={i}>
          <TextControl
            label={`Option ${i + 1} value`}
            value={o.value}
            monospace
            onChange={(v) => update(i, { value: v })}
            problems={problemsAt(problems, `${basePath}[${i}].value`)}
          />
          <TextControl
            label={`Option ${i + 1} label`}
            value={o.label}
            onChange={(v) => update(i, { label: v })}
            problems={problemsAt(problems, `${basePath}[${i}].label`)}
          />
          <button
            type="button"
            className="of-ed-icon-btn"
            aria-label={`Remove option ${i + 1}`}
            onClick={() => onChange(options.filter((_, j) => j !== i))}
          >
            ✕
          </button>
        </div>
      ))}
      <button type="button" className="of-ed-btn" onClick={add}>
        Add option
      </button>
    </fieldset>
  );
}
