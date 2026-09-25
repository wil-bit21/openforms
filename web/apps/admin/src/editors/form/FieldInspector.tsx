import type { Dispatch } from "react";
import { CheckboxControl, SelectControl, TextControl } from "../shared/controls";
import { OptionsEditor } from "../shared/OptionsEditor";
import { problemsAt } from "../shared/problems";
import { FIELD_TYPES, FIELD_TYPE_LABELS, OPTION_FIELD_TYPES, type FieldType, type FormDef, type Problem } from "../shared/types";
import type { FieldPatch, FormEditorAction } from "./formReducer";
import { ShowIfBuilder } from "./ShowIfBuilder";
import { ValidationEditor } from "./ValidationEditor";

export function FieldInspector({
  def,
  index,
  problems,
  dispatch,
}: {
  def: FormDef;
  index: number;
  problems: Problem[];
  dispatch: Dispatch<FormEditorAction>;
}) {
  const field = def.fields[index];
  if (!field) return null;
  const base = `fields[${index}]`;
  const at = (prop: string) => problemsAt(problems, `${base}.${prop}`);
  const update = (patch: FieldPatch) => dispatch({ type: "updateField", index, patch });

  return (
    <section className="of-ed-inspector" aria-label="Field settings">
      <h2>{field.label || field.key || `Question ${index + 1}`}</h2>
      <TextControl label="Label" value={field.label} problems={at("label")} onChange={(v) => update({ label: v })} />
      <TextControl
        label="Key"
        value={field.key}
        monospace
        problems={at("key")}
        hint="Used in the API, exports and conditions. Renaming updates conditions that refer to it."
        onChange={(v) => dispatch({ type: "renameField", index, key: v })}
      />
      <SelectControl
        label="Type"
        value={field.type}
        options={FIELD_TYPES.map((t) => ({ value: t, label: FIELD_TYPE_LABELS[t] }))}
        problems={at("type")}
        onChange={(t) => dispatch({ type: "setFieldType", index, fieldType: t as FieldType })}
      />
      <CheckboxControl
        label={field.type === "checkbox" ? "Required (must be checked)" : "Required"}
        checked={Boolean(field.required)}
        onChange={(checked) => update({ required: checked })}
      />
      {field.type !== "checkbox" ? (
        <TextControl label="Placeholder" value={field.placeholder} problems={at("placeholder")} onChange={(v) => update({ placeholder: v })} />
      ) : null}
      <TextControl label="Help text" value={field.help} multiline problems={at("help")} onChange={(v) => update({ help: v })} />
      {OPTION_FIELD_TYPES.has(field.type) ? (
        <OptionsEditor options={field.options ?? []} basePath={`${base}.options`} problems={problems} onChange={(options) => update({ options })} />
      ) : null}
      <ValidationEditor field={field} base={`${base}.validation`} problems={problems} onChange={(validation) => update({ validation })} />
      <ShowIfBuilder fields={def.fields} index={index} problems={problems} onChange={(showIf) => update({ showIf })} />
    </section>
  );
}
