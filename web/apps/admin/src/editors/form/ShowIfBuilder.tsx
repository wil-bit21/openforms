import { CheckboxControl, FieldErrors, NumberControl, SelectControl, TextControl } from "../shared/controls";
import { problemsAt } from "../shared/problems";
import type { Condition, Field, Problem } from "../shared/types";

type Operator = "equals" | "notEquals" | "in";

const TOGGLE_LABEL = "Only show this field when a previous answer matches";

export function conditionOperator(c: Condition): Operator {
  if (c.in !== undefined) return "in";
  if (c.notEquals !== undefined) return "notEquals";
  return "equals";
}

export function defaultValueFor(field: Field | undefined): string | number | boolean {
  if (!field) return "";
  if (field.type === "select" || field.type === "multiselect") return field.options?.[0]?.value ?? "";
  if (field.type === "checkbox") return true;
  if (field.type === "number") return 0;
  return "";
}

function makeCondition(field: string, op: Operator, value: unknown): Condition {
  if (op === "in") return { field, in: Array.isArray(value) ? value : [value] };
  const scalar = Array.isArray(value) ? value[0] : value;
  return op === "equals" ? { field, equals: scalar } : { field, notEquals: scalar };
}

export function ShowIfBuilder({
  fields,
  index,
  problems,
  onChange,
}: {
  fields: Field[];
  index: number;
  problems: Problem[];
  onChange: (next: Condition | undefined) => void;
}) {
  const earlier = fields.slice(0, index);
  const cond = fields[index]?.showIf;
  const base = `fields[${index}].showIf`;

  if (earlier.length === 0) {
    return <p className="of-ed-hint">Conditional display is available for fields after the first one.</p>;
  }
  if (!cond) {
    const last = earlier[earlier.length - 1];
    return (
      <CheckboxControl
        label={TOGGLE_LABEL}
        checked={false}
        onChange={(on) => on && onChange(makeCondition(last.key, "equals", defaultValueFor(last)))}
      />
    );
  }

  const controller = earlier.find((f) => f.key === cond.field);
  const op = conditionOperator(cond);
  const value = cond[op];
  const controllerOptions = earlier.map((f) => ({ value: f.key, label: `${f.label || f.key} (${f.key})` }));
  if (!controller) controllerOptions.unshift({ value: cond.field, label: `${cond.field} (not an earlier field)` });

  return (
    <fieldset className="of-ed-fieldset">
      <legend>Conditional display</legend>
      <CheckboxControl label={TOGGLE_LABEL} checked onChange={(on) => !on && onChange(undefined)} />
      <FieldErrors problems={problemsAt(problems, base)} />
      <SelectControl
        label="Previous field"
        value={cond.field}
        options={controllerOptions}
        problems={problemsAt(problems, `${base}.field`)}
        onChange={(key) => onChange(makeCondition(key, op, defaultValueFor(earlier.find((f) => f.key === key))))}
      />
      <SelectControl
        label="Condition"
        value={op}
        options={[
          { value: "equals", label: "is" },
          { value: "notEquals", label: "is not" },
          { value: "in", label: "is one of" },
        ]}
        onChange={(next) => onChange(makeCondition(cond.field, next as Operator, value))}
      />
      <ConditionValue
        controller={controller}
        op={op}
        value={value}
        problems={problemsAt(problems, `${base}.${op}`)}
        onChange={(v) => onChange(makeCondition(cond.field, op, v))}
      />
    </fieldset>
  );
}

function ConditionValue({
  controller,
  op,
  value,
  problems,
  onChange,
}: {
  controller: Field | undefined;
  op: Operator;
  value: unknown;
  problems: Problem[];
  onChange: (value: unknown) => void;
}) {
  const options = controller?.options ?? [];
  if (options.length > 0) {
    if (op === "in") {
      const selected = Array.isArray(value) ? value : [];
      return (
        <fieldset className="of-ed-fieldset">
          <legend>Values</legend>
          {options.map((o) => (
            <CheckboxControl
              key={o.value}
              label={o.label}
              checked={selected.includes(o.value)}
              onChange={(on) => onChange(on ? [...selected, o.value] : selected.filter((v) => v !== o.value))}
            />
          ))}
          <FieldErrors problems={problems} />
        </fieldset>
      );
    }
    return (
      <SelectControl
        label="Value"
        value={String(value ?? "")}
        options={options.map((o) => ({ value: o.value, label: o.label }))}
        problems={problems}
        onChange={onChange}
      />
    );
  }
  if (controller?.type === "checkbox") {
    const scalar = Array.isArray(value) ? value[0] : value;
    return (
      <SelectControl
        label="Value"
        value={scalar === false ? "false" : "true"}
        options={[
          { value: "true", label: "Checked" },
          { value: "false", label: "Not checked" },
        ]}
        problems={problems}
        onChange={(v) => onChange(v === "true")}
      />
    );
  }
  if (controller?.type === "number") {
    const scalar = Array.isArray(value) ? value[0] : value;
    return (
      <NumberControl
        label="Value"
        value={typeof scalar === "number" ? scalar : undefined}
        problems={problems}
        onChange={(n) => onChange(n ?? 0)}
      />
    );
  }
  const text = Array.isArray(value) ? value.join(", ") : String(value ?? "");
  return (
    <TextControl
      label={op === "in" ? "Values (comma separated)" : "Value"}
      value={text}
      problems={problems}
      onChange={(t) => onChange(op === "in" ? t.split(",").map((s) => s.trim()).filter(Boolean) : t)}
    />
  );
}
