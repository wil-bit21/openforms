import { NumberControl, TextControl } from "../shared/controls";
import { applyPatch } from "../shared/objects";
import { problemsAt } from "../shared/problems";
import { STRING_FIELD_TYPES, type Field, type Problem, type Validation } from "../shared/types";

export function ValidationEditor({
  field,
  base,
  problems,
  onChange,
}: {
  field: Field;
  base: string;
  problems: Problem[];
  onChange: (next: Validation | undefined) => void;
}) {
  const v = field.validation ?? {};
  const set = (patch: Partial<Validation>) => {
    const next = applyPatch(v, patch);
    onChange(Object.keys(next).length > 0 ? next : undefined);
  };
  const at = (key: keyof Validation) => problemsAt(problems, `${base}.${key}`);

  if (STRING_FIELD_TYPES.has(field.type)) {
    return (
      <fieldset className="of-ed-fieldset">
        <legend>Validation</legend>
        <NumberControl label="Minimum length" min={0} value={v.minLength} problems={at("minLength")} onChange={(n) => set({ minLength: n })} />
        <NumberControl label="Maximum length" min={0} value={v.maxLength} problems={at("maxLength")} onChange={(n) => set({ maxLength: n })} />
        <TextControl
          label="Pattern (regular expression)"
          monospace
          value={v.pattern}
          problems={at("pattern")}
          hint="The server uses RE2 syntax: lookarounds and backreferences are not supported."
          onChange={(p) => set({ pattern: p })}
        />
      </fieldset>
    );
  }
  if (field.type === "number") {
    return (
      <fieldset className="of-ed-fieldset">
        <legend>Validation</legend>
        <NumberControl label="Minimum" value={v.min} problems={at("min")} onChange={(n) => set({ min: n })} />
        <NumberControl label="Maximum" value={v.max} problems={at("max")} onChange={(n) => set({ max: n })} />
      </fieldset>
    );
  }
  return null;
}
