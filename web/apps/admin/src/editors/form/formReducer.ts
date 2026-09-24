import { applyPatch, clone, isObject, moveItem, nextNumbered, objects, str, uniqueName } from "../shared/objects";
import {
  OPTION_FIELD_TYPES,
  STRING_FIELD_TYPES,
  type Field,
  type FieldType,
  type FormDef,
  type FormSettings,
  type Validation,
} from "../shared/types";

export interface FormEditorState {
  def: FormDef;
  /** Index of the selected field, or null when form settings are shown. */
  selected: number | null;
  dirty: boolean;
}

export type FieldPatch = Partial<Pick<Field, "label" | "help" | "placeholder" | "required" | "options" | "validation" | "showIf">>;

export type FormEditorAction =
  | { type: "setMeta"; patch: Partial<Pick<FormDef, "slug" | "title" | "description" | "workflow">> }
  | { type: "setSettings"; patch: Partial<FormSettings> }
  | { type: "addField"; fieldType: FieldType }
  | { type: "updateField"; index: number; patch: FieldPatch }
  | { type: "setFieldType"; index: number; fieldType: FieldType }
  | { type: "renameField"; index: number; key: string }
  | { type: "removeField"; index: number }
  | { type: "duplicateField"; index: number }
  | { type: "moveField"; index: number; direction: -1 | 1 }
  | { type: "select"; index: number | null }
  | { type: "replace"; def: FormDef }
  | { type: "markSaved" };

const STARTER_OPTIONS = () => [
  { value: "option1", label: "Option 1" },
  { value: "option2", label: "Option 2" },
];

export function newFormDefinition(): FormDef {
  return { slug: "", title: "Untitled form", settings: { public: true }, fields: [] };
}

export function newField(type: FieldType, takenKeys: string[]): Field {
  const field: Field = { key: nextNumbered("field", takenKeys), type, label: "Untitled question" };
  if (OPTION_FIELD_TYPES.has(type)) field.options = STARTER_OPTIONS();
  return field;
}

export function normalizeForm(input: unknown): FormDef {
  const d = isObject(input) ? input : {};
  return {
    ...d,
    slug: str(d.slug),
    title: str(d.title),
    settings: isObject(d.settings) ? (d.settings as unknown as FormSettings) : { public: false },
    fields: objects<Field>(d.fields),
  } as FormDef;
}

export function initFormEditor(def: FormDef): FormEditorState {
  return { def: normalizeForm(clone(def)), selected: null, dirty: false };
}

const inRange = (state: FormEditorState, index: number) => index >= 0 && index < state.def.fields.length;
const countKey = (fields: Field[], key: string) => fields.filter((f) => f.key === key).length;

function withFields(state: FormEditorState, fields: Field[], selected = state.selected): FormEditorState {
  return { ...state, def: { ...state.def, fields }, selected, dirty: true };
}

function withoutShowIf(field: Field): Field {
  const next = { ...field };
  delete next.showIf;
  return next;
}

function fitValidation(validation: Validation | undefined, type: FieldType): Validation | undefined {
  if (!validation) return undefined;
  const next = { ...validation };
  if (!STRING_FIELD_TYPES.has(type)) {
    delete next.minLength;
    delete next.maxLength;
    delete next.pattern;
  }
  if (type !== "number") {
    delete next.min;
    delete next.max;
  }
  return Object.keys(next).length > 0 ? next : undefined;
}

export function formEditorReducer(state: FormEditorState, action: FormEditorAction): FormEditorState {
  const fields = state.def.fields;
  switch (action.type) {
    case "setMeta": {
      const { slug, title, ...optional } = action.patch;
      let def = applyPatch(state.def, optional);
      if (slug !== undefined) def = { ...def, slug };
      if (title !== undefined) def = { ...def, title };
      return { ...state, def, dirty: true };
    }
    case "setSettings":
      return { ...state, def: { ...state.def, settings: applyPatch(state.def.settings, action.patch) }, dirty: true };
    case "addField": {
      const next = [...fields, newField(action.fieldType, fields.map((f) => f.key))];
      return withFields(state, next, next.length - 1);
    }
    case "updateField": {
      if (!inRange(state, action.index)) return state;
      const updated = applyPatch<Field>(fields[action.index], action.patch);
      if (!updated.required) delete updated.required;
      return withFields(state, fields.map((f, i) => (i === action.index ? updated : f)));
    }
    case "setFieldType": {
      if (!inRange(state, action.index)) return state;
      const updated: Field = { ...fields[action.index], type: action.fieldType };
      if (OPTION_FIELD_TYPES.has(action.fieldType)) {
        if (!updated.options?.length) updated.options = STARTER_OPTIONS();
      } else {
        delete updated.options;
      }
      const validation = fitValidation(updated.validation, action.fieldType);
      if (validation) updated.validation = validation;
      else delete updated.validation;
      return withFields(state, fields.map((f, i) => (i === action.index ? updated : f)));
    }
    case "renameField": {
      if (!inRange(state, action.index)) return state;
      const oldKey = fields[action.index].key;
      const followReferences = countKey(fields, oldKey) === 1;
      const next = fields.map((f, i) => {
        if (i === action.index) return { ...f, key: action.key };
        if (followReferences && f.showIf?.field === oldKey) return { ...f, showIf: { ...f.showIf, field: action.key } };
        return f;
      });
      return withFields(state, next);
    }
    case "removeField": {
      if (!inRange(state, action.index)) return state;
      const removedKey = fields[action.index].key;
      let next = fields.filter((_, i) => i !== action.index);
      if (countKey(next, removedKey) === 0) {
        next = next.map((f) => (f.showIf?.field === removedKey ? withoutShowIf(f) : f));
      }
      let selected = state.selected;
      if (selected !== null) {
        if (selected === action.index) selected = next.length === 0 ? null : Math.min(action.index, next.length - 1);
        else if (selected > action.index) selected -= 1;
      }
      return withFields(state, next, selected);
    }
    case "duplicateField": {
      if (!inRange(state, action.index)) return state;
      const original = fields[action.index];
      const copy: Field = {
        ...clone(original),
        key: uniqueName(`${original.key}_copy`, fields.map((f) => f.key)),
        label: `${original.label} (copy)`,
      };
      const next = [...fields.slice(0, action.index + 1), copy, ...fields.slice(action.index + 1)];
      return withFields(state, next, action.index + 1);
    }
    case "moveField": {
      const target = action.index + action.direction;
      if (!inRange(state, action.index) || target < 0 || target >= fields.length) return state;
      let selected = state.selected;
      if (selected === action.index) selected = target;
      else if (selected === target) selected = action.index;
      return withFields(state, moveItem(fields, action.index, target), selected);
    }
    case "select":
      return { ...state, selected: action.index };
    case "replace": {
      const def = normalizeForm(clone(action.def));
      const selected = state.selected !== null && state.selected < def.fields.length ? state.selected : null;
      return { def, selected, dirty: true };
    }
    case "markSaved":
      return { ...state, dirty: false };
  }
}
