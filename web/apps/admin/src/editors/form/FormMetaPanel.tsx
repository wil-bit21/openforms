import type { Dispatch } from "react";
import { CheckboxControl, SelectControl, TextControl } from "../shared/controls";
import { unique } from "../shared/objects";
import { problemsAt } from "../shared/problems";
import type { FormDef, Problem } from "../shared/types";
import type { FormEditorAction } from "./formReducer";

export function FormMetaPanel({
  def,
  isNew,
  workflowSlugs,
  problems,
  dispatch,
}: {
  def: FormDef;
  isNew: boolean;
  workflowSlugs: string[];
  problems: Problem[];
  dispatch: Dispatch<FormEditorAction>;
}) {
  const at = (path: string) => problemsAt(problems, path);
  const workflows = unique([...workflowSlugs, ...(def.workflow ? [def.workflow] : [])]);
  return (
    <section className="of-ed-inspector" aria-label="Form settings">
      <h2>Form settings</h2>
      <TextControl
        label="Slug"
        value={def.slug}
        monospace
        readOnly={!isNew}
        problems={at("slug")}
        hint={isNew ? "Lowercase letters, digits and dashes. Used in URLs like /f/<slug>. It can't be changed later." : "The slug can't be changed after the form is created."}
        onChange={(v) => dispatch({ type: "setMeta", patch: { slug: v } })}
      />
      <TextControl label="Title" value={def.title} problems={at("title")} onChange={(v) => dispatch({ type: "setMeta", patch: { title: v } })} />
      <TextControl
        label="Description"
        value={def.description}
        multiline
        problems={at("description")}
        onChange={(v) => dispatch({ type: "setMeta", patch: { description: v } })}
      />
      <SelectControl
        label="Workflow"
        value={def.workflow ?? ""}
        options={[{ value: "", label: "No workflow (submissions are simply received)" }, ...workflows.map((s) => ({ value: s, label: s }))]}
        problems={at("workflow")}
        onChange={(v) => dispatch({ type: "setMeta", patch: { workflow: v } })}
      />
      <CheckboxControl
        label="Public (anyone with the link can submit)"
        checked={Boolean(def.settings.public)}
        onChange={(checked) => dispatch({ type: "setSettings", patch: { public: checked } })}
      />
      <TextControl
        label="Submit button label"
        value={def.settings.submitLabel}
        placeholder="Submit"
        problems={at("settings.submitLabel")}
        onChange={(v) => dispatch({ type: "setSettings", patch: { submitLabel: v } })}
      />
      <TextControl
        label="Confirmation message"
        value={def.settings.confirmationMessage}
        multiline
        problems={at("settings.confirmationMessage")}
        onChange={(v) => dispatch({ type: "setSettings", patch: { confirmationMessage: v } })}
      />
    </section>
  );
}
