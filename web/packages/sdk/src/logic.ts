// Client-side mirror of the Go `definition` package's Visible / ValidateSubmission (spec §5.4).
// The server is authoritative; this exists for UX and must pass schemas/fixtures/*.json.
import type { Problem } from "./api-types.js";

export type Values = Record<string, unknown>;

export interface LogicField {
  key: string;
  type: string;
  required?: boolean;
  options?: readonly { value: string }[];
  validation?: { minLength?: number; maxLength?: number; pattern?: string; min?: number; max?: number };
  showIf?: { field: string; equals?: unknown; notEquals?: unknown; in?: readonly unknown[] };
}

export interface LogicForm {
  fields?: readonly LogicField[];
}

export interface ValidationResult {
  clean: Values | null;
  problems: Problem[];
}

export const FORM_ERROR_KEY = "_form";

export const messages = {
  required: "This field is required",
  mustTick: "Please tick this box to continue",
  text: "Enter text",
  number: "Enter a number",
  boolean: "Choose yes or no",
  choice: "Choose one of the listed options",
  duplicate: "Each option can only be chosen once",
  email: "Enter a valid email address, like name@example.com",
  url: "Enter a full web address starting with http:// or https://",
  date: "Enter a real date in the format YYYY-MM-DD",
  pattern: "Use the format shown",
  minLength: (n: number) => `Use at least ${n} ${n === 1 ? "character" : "characters"}`,
  maxLength: (n: number) => `Use at most ${n} ${n === 1 ? "character" : "characters"}`,
  min: (n: number) => `Must be ${n} or more`,
  max: (n: number) => `Must be ${n} or less`,
};

export function isProvided(value: unknown): boolean {
  return !(value === undefined || value === null || value === "" || (Array.isArray(value) && value.length === 0));
}

export function jsonEqual(a: unknown, b: unknown): boolean {
  if (a === b) return true;
  if (Array.isArray(a) || Array.isArray(b)) {
    return Array.isArray(a) && Array.isArray(b) && a.length === b.length && a.every((x, i) => jsonEqual(x, b[i]));
  }
  if (a !== null && b !== null && typeof a === "object" && typeof b === "object") {
    const ka = Object.keys(a);
    const kb = Object.keys(b);
    return ka.length === kb.length && ka.every((k) => jsonEqual((a as Values)[k], (b as Values)[k]));
  }
  return false;
}

function conditionHolds(
  cond: NonNullable<LogicField["showIf"]>,
  controller: LogicField,
  value: unknown,
  provided: boolean,
): boolean {
  const multi = controller.type === "multiselect" && Array.isArray(value);
  const matches = (target: unknown) =>
    provided && (multi ? (value as unknown[]).some((v) => jsonEqual(v, target)) : jsonEqual(value, target));

  // null counts as absent, mirroring Go's `omitempty` on `any`.
  if (cond.equals != null) return matches(cond.equals);
  if (cond.notEquals != null) return !matches(cond.notEquals);
  if (Array.isArray(cond.in)) return cond.in.some((t) => matches(t));
  return true;
}

interface Evaluation {
  shown: Record<string, boolean>;
  clean: Values;
  problems: Problem[];
}

/**
 * Walks fields in declaration order, like Go's `evaluate`: a condition sees the
 * cleaned value of its (earlier) controller, an invalid value counts as absent,
 * and a hidden controller hides its dependants.
 */
function evaluate(form: LogicForm, data: Values): Evaluation {
  const shown: Record<string, boolean> = {};
  const clean: Values = {};
  const problems: Problem[] = [];
  const seen = new Map<string, LogicField>();
  for (const field of form.fields ?? []) {
    let vis = true;
    if (field.showIf) {
      const controller = seen.get(field.showIf.field);
      vis =
        controller !== undefined &&
        shown[controller.key] === true &&
        conditionHolds(field.showIf, controller, clean[controller.key], controller.key in clean);
    }
    shown[field.key] = vis;
    seen.set(field.key, field);
    if (!vis) continue;

    const path = `data.${field.key}`;
    const res = checkValue(field, data[field.key]);
    if (res.error) {
      problems.push({ path, message: res.error });
    } else if (!res.provided) {
      if (field.required) problems.push({ path, message: messages.required });
    } else if (field.type === "checkbox" && field.required && res.value !== true) {
      problems.push({ path, message: messages.mustTick });
    } else {
      clean[field.key] = res.value;
    }
  }
  return { shown, clean, problems };
}

export function visible(form: LogicForm, data: Values): Record<string, boolean> {
  return evaluate(form, data).shown;
}

const DATE_RE = /^(\d{4})-(\d{2})-(\d{2})$/;
const EMAIL_RE = /^[^\s@<>()[\]\\,;:"]+@[^\s@<>()[\]\\,;:"]+$/;

function isDate(s: string): boolean {
  const m = DATE_RE.exec(s);
  if (!m) return false;
  const [y, mo, d] = [Number(m[1]), Number(m[2]), Number(m[3])];
  const dt = new Date(Date.UTC(y, mo - 1, d));
  return dt.getUTCFullYear() === y && dt.getUTCMonth() === mo - 1 && dt.getUTCDate() === d;
}

function isHttpUrl(s: string): boolean {
  try {
    const u = new URL(s);
    return (u.protocol === "http:" || u.protocol === "https:") && u.hostname !== "";
  } catch {
    return false;
  }
}

function matchesPattern(pattern: string, s: string): boolean {
  try {
    // Anchored: the whole value must match (like HTML `pattern`).
    return new RegExp(`^(?:${pattern})$`, "u").test(s);
  } catch {
    return true; // invalid patterns are rejected when the definition is saved; don't block respondents
  }
}

interface Checked {
  value?: unknown;
  provided: boolean;
  error?: string;
}

/** Validates and normalizes one answer: strings are trimmed; blank, null and [] are "not provided". */
function checkValue(field: LogicField, raw: unknown): Checked {
  if (raw === undefined || raw === null) return { provided: false };
  const optionValues = new Set((field.options ?? []).map((o) => o.value));
  const v = field.validation;

  switch (field.type) {
    case "number": {
      if (typeof raw !== "number" || !Number.isFinite(raw)) return { provided: false, error: messages.number };
      if (v?.min != null && raw < v.min) return { provided: false, error: messages.min(v.min) };
      if (v?.max != null && raw > v.max) return { provided: false, error: messages.max(v.max) };
      return { value: raw, provided: true };
    }
    case "checkbox":
      return typeof raw === "boolean" ? { value: raw, provided: true } : { provided: false, error: messages.boolean };
    case "multiselect": {
      if (!Array.isArray(raw) || !raw.every((x) => typeof x === "string")) return { provided: false, error: messages.choice };
      if (raw.length === 0) return { provided: false };
      if (!raw.every((x) => optionValues.has(x))) return { provided: false, error: messages.choice };
      if (new Set(raw).size !== raw.length) return { provided: false, error: messages.duplicate };
      return { value: [...raw], provided: true };
    }
  }

  if (typeof raw !== "string") return { provided: false, error: messages.text };
  const value = raw.trim();
  if (value === "") return { provided: false };
  switch (field.type) {
    case "email":
      if (!EMAIL_RE.test(value)) return { provided: false, error: messages.email };
      break;
    case "url":
      if (!isHttpUrl(value)) return { provided: false, error: messages.url };
      break;
    case "date":
      if (!isDate(value)) return { provided: false, error: messages.date };
      break;
    case "select":
      if (!optionValues.has(value)) return { provided: false, error: messages.choice };
      break;
  }
  const length = Array.from(value).length;
  if (v?.minLength != null && length < v.minLength) return { provided: false, error: messages.minLength(v.minLength) };
  if (v?.maxLength != null && length > v.maxLength) return { provided: false, error: messages.maxLength(v.maxLength) };
  if (v?.pattern && !matchesPattern(v.pattern, value)) return { provided: false, error: messages.pattern };
  return { value, provided: true };
}

export function validateSubmission(form: LogicForm, data: Values): ValidationResult {
  const { clean, problems } = evaluate(form, data);
  return problems.length > 0 ? { clean: null, problems } : { clean, problems };
}

export function problemsToErrors(problems: readonly Problem[]): Record<string, string> {
  const errors: Record<string, string> = {};
  for (const p of problems) {
    const key = p.path.startsWith("data.") ? p.path.slice("data.".length) : FORM_ERROR_KEY;
    if (!(key in errors)) errors[key] = p.message;
  }
  return errors;
}
