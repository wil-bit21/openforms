import Ajv2020 from "ajv/dist/2020";
import type { ErrorObject } from "ajv";
import formSchema from "@schemas/form.schema.json";
import workflowSchema from "@schemas/workflow.schema.json";
import { dedupeProblems } from "./problems";
import {
  OPTION_FIELD_TYPES,
  STRING_FIELD_TYPES,
  WORKFLOW_FIELD_TYPES,
  type Action,
  type Field,
  type FormDef,
  type Problem,
  type State,
  type Transition,
  type WorkflowDef,
  type WorkflowField,
} from "./types";

// The Go server is authoritative (spec §2). This module mirrors spec §5.3 so the
// editors can show problems inline, using the same path format as the server.

const ajv = new Ajv2020({ allErrors: true, strict: false, validateFormats: false });
// Compile the form schema first so a workflow schema $ref to it resolves.
const validateFormSchema = ajv.compile(formSchema);
const validateWorkflowSchema = ajv.compile(workflowSchema);

const SLUG_RE = /^[a-z0-9][a-z0-9-]{0,62}$/;
const KEY_RE = /^[a-zA-Z][a-zA-Z0-9_]{0,63}$/;
const SLUG_MESSAGE = "must be 1-63 lowercase letters, digits or dashes, starting with a letter or digit";
const KEY_MESSAGE = "must start with a letter and contain only letters, digits and underscores (max 64)";

type Add = (path: string, message: string) => void;

function list<T>(value: unknown): T[] {
  return Array.isArray(value) ? (value as T[]) : [];
}

export function pointerToPath(pointer: string): string {
  if (!pointer) return "";
  return pointer
    .split("/")
    .slice(1)
    .map((seg) => seg.replace(/~1/g, "/").replace(/~0/g, "~"))
    .reduce((acc, seg) => (/^\d+$/.test(seg) ? `${acc}[${seg}]` : acc ? `${acc}.${seg}` : seg), "");
}

function joinPath(base: string, key: string): string {
  return base ? `${base}.${key}` : key;
}

export function ajvProblems(errors: ErrorObject[]): Problem[] {
  const out: Problem[] = [];
  for (const e of errors) {
    if (e.keyword === "if") continue; // always accompanied by the more specific "then" error
    const base = pointerToPath(e.instancePath);
    const params = e.params as Record<string, unknown>;
    if (e.keyword === "required") {
      out.push({ path: joinPath(base, String(params.missingProperty)), message: "is required" });
    } else if (e.keyword === "additionalProperties") {
      out.push({ path: joinPath(base, String(params.additionalProperty)), message: "is not a recognised property" });
    } else {
      out.push({ path: base, message: e.message ?? "is invalid" });
    }
  }
  return dedupeProblems(out);
}

function combine(schemaProblems: Problem[], semantic: Problem[]): Problem[] {
  // When the schema already complains about a path, the semantic message there is noise.
  const schemaPaths = new Set(schemaProblems.map((p) => p.path));
  return dedupeProblems([...schemaProblems, ...semantic.filter((p) => !schemaPaths.has(p.path))]);
}

export function validateForm(def: FormDef): Problem[] {
  const schema = validateFormSchema(def) ? [] : ajvProblems(validateFormSchema.errors ?? []);
  return combine(schema, formSemanticProblems(def));
}

export function validateWorkflow(def: WorkflowDef): Problem[] {
  const schema = validateWorkflowSchema(def) ? [] : ajvProblems(validateWorkflowSchema.errors ?? []);
  return combine(schema, workflowSemanticProblems(def));
}

export function formSemanticProblems(def: FormDef): Problem[] {
  const out: Problem[] = [];
  const add: Add = (path, message) => out.push({ path, message });
  if (!SLUG_RE.test(String(def?.slug ?? ""))) add("slug", SLUG_MESSAGE);

  const fields = list<Field>(def?.fields);
  const firstIndex = new Map<string, number>();
  fields.forEach((field, i) => {
    if (!field || typeof field !== "object") return;
    const base = `fields[${i}]`;
    const key = String(field.key ?? "");
    if (!KEY_RE.test(key)) add(`${base}.key`, KEY_MESSAGE);
    else if (firstIndex.has(key)) add(`${base}.key`, `duplicates the key of fields[${firstIndex.get(key)}]`);
    else firstIndex.set(key, i);

    const options = list<{ value: string }>(field.options);
    if (OPTION_FIELD_TYPES.has(field.type)) {
      if (options.length === 0) add(`${base}.options`, "needs at least one option");
      const seen = new Set<string>();
      options.forEach((o, j) => {
        if (seen.has(o?.value)) add(`${base}.options[${j}].value`, "duplicates another option value");
        seen.add(o?.value);
      });
    } else if (options.length > 0) {
      add(`${base}.options`, "only dropdown and multiple choice fields have options");
    }

    const v = field.validation;
    if (v && typeof v === "object") {
      const isString = STRING_FIELD_TYPES.has(field.type);
      (["minLength", "maxLength", "pattern"] as const).forEach((k) => {
        if (v[k] !== undefined && !isString) add(`${base}.validation.${k}`, "only applies to short text, long text, email and URL fields");
      });
      (["min", "max"] as const).forEach((k) => {
        if (v[k] !== undefined && field.type !== "number") add(`${base}.validation.${k}`, "only applies to number fields");
      });
      if (v.minLength !== undefined && v.maxLength !== undefined && v.minLength > v.maxLength) {
        add(`${base}.validation.minLength`, "must not be greater than maxLength");
      }
      if (v.min !== undefined && v.max !== undefined && v.min > v.max) {
        add(`${base}.validation.min`, "must not be greater than max");
      }
      if (v.pattern) {
        try {
          new RegExp(v.pattern);
        } catch {
          add(`${base}.validation.pattern`, "is not a valid regular expression");
        }
      }
    }

    const c = field.showIf;
    if (c && typeof c === "object") {
      const earlier = fields.slice(0, i).map((f) => f?.key);
      if (!earlier.includes(c.field)) add(`${base}.showIf.field`, "must refer to a field declared earlier in the form");
      const operators = [c.equals, c.notEquals, c.in].filter((x) => x !== undefined).length;
      if (operators !== 1) add(`${base}.showIf`, "needs exactly one of equals, notEquals or in");
    }
  });
  return out;
}

export function workflowSemanticProblems(def: WorkflowDef): Problem[] {
  const out: Problem[] = [];
  const add: Add = (path, message) => out.push({ path, message });
  if (!SLUG_RE.test(String(def?.slug ?? ""))) add("slug", SLUG_MESSAGE);

  const states = list<State>(def?.states);
  const stateIndex = new Map<string, number>();
  states.forEach((s, i) => {
    const key = String(s?.key ?? "");
    if (!KEY_RE.test(key)) add(`states[${i}].key`, KEY_MESSAGE);
    else if (stateIndex.has(key)) add(`states[${i}].key`, `duplicates the key of states[${stateIndex.get(key)}]`);
    else stateIndex.set(key, i);
  });
  const terminal = new Set(states.filter((s) => s?.terminal).map((s) => s.key));
  if (!stateIndex.has(def?.initial)) add("initial", "must be one of the declared states");

  const fieldKeys = new Map<string, number>();
  list<WorkflowField>(def?.fields).forEach((f, i) => {
    const key = String(f?.key ?? "");
    if (!KEY_RE.test(key)) add(`fields[${i}].key`, KEY_MESSAGE);
    else if (fieldKeys.has(key)) add(`fields[${i}].key`, `duplicates the key of fields[${fieldKeys.get(key)}]`);
    else fieldKeys.set(key, i);
    if (!WORKFLOW_FIELD_TYPES.includes(f?.type)) add(`fields[${i}].type`, "is not available for workflow fields");
    if (f?.type === "select" && list(f.options).length === 0) add(`fields[${i}].options`, "needs at least one option");
  });

  const transitionKeys = new Map<string, number>();
  list<Transition>(def?.transitions).forEach((t, i) => {
    const base = `transitions[${i}]`;
    const key = String(t?.key ?? "");
    if (!KEY_RE.test(key)) add(`${base}.key`, KEY_MESSAGE);
    else if (transitionKeys.has(key)) add(`${base}.key`, `duplicates the key of transitions[${transitionKeys.get(key)}]`);
    else transitionKeys.set(key, i);

    const from = list<string>(t?.from);
    if (from.length === 0) add(`${base}.from`, "needs at least one source state");
    from.forEach((k, j) => {
      if (!stateIndex.has(k)) add(`${base}.from[${j}]`, `refers to unknown state "${k}"`);
      else if (terminal.has(k)) add(`${base}.from[${j}]`, `cannot leave terminal state "${k}"`);
    });
    if (!stateIndex.has(t?.to)) add(`${base}.to`, `refers to unknown state "${t?.to ?? ""}"`);
    list<string>(t?.guard?.requireFields).forEach((k, j) => {
      if (!fieldKeys.has(k)) add(`${base}.guard.requireFields[${j}]`, `refers to unknown workflow field "${k}"`);
    });
    list<Action>(t?.actions).forEach((a, j) => actionProblems(a, `${base}.actions[${j}]`, add));
  });
  list<Action>(def?.onSubmit).forEach((a, j) => actionProblems(a, `onSubmit[${j}]`, add));
  return out;
}

function actionProblems(a: Action, base: string, add: Add): void {
  switch (a?.type) {
    case "webhook":
      if (!/^https?:\/\/[^\s/]+/i.test(a.url ?? "")) add(`${base}.url`, "must be an absolute http(s) URL");
      break;
    case "email":
      if (!a.to?.trim()) add(`${base}.to`, "is required");
      if (!a.subject?.trim()) add(`${base}.subject`, "is required");
      break;
    case "assign": {
      const count = (a.user ? 1 : 0) + (a.role ? 1 : 0);
      if (count !== 1) add(base, "needs exactly one of user or role");
      break;
    }
    default:
      add(`${base}.type`, "must be webhook, email or assign");
  }
}
