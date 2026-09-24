// Editor-side definition types. They mirror spec §5.1/§5.2 exactly and are kept
// local so the editors do not depend on the shape of generated SDK types.
// Conversion to/from SDK types happens only in src/editors/api.ts.

export type FieldType =
  | "text"
  | "textarea"
  | "email"
  | "number"
  | "select"
  | "multiselect"
  | "checkbox"
  | "date"
  | "url";

export const FIELD_TYPES: FieldType[] = [
  "text",
  "textarea",
  "email",
  "number",
  "select",
  "multiselect",
  "checkbox",
  "date",
  "url",
];

export const FIELD_TYPE_LABELS: Record<FieldType, string> = {
  text: "Short text",
  textarea: "Long text",
  email: "Email",
  number: "Number",
  select: "Dropdown",
  multiselect: "Multiple choice",
  checkbox: "Checkbox",
  date: "Date",
  url: "URL",
};

export const WORKFLOW_FIELD_TYPES: FieldType[] = ["text", "textarea", "number", "select", "checkbox", "date"];
export const STRING_FIELD_TYPES: ReadonlySet<FieldType> = new Set<FieldType>(["text", "textarea", "email", "url"]);
export const OPTION_FIELD_TYPES: ReadonlySet<FieldType> = new Set<FieldType>(["select", "multiselect"]);

export interface Option {
  value: string;
  label: string;
}

export interface Validation {
  minLength?: number;
  maxLength?: number;
  pattern?: string;
  min?: number;
  max?: number;
}

export interface Condition {
  field: string;
  equals?: unknown;
  notEquals?: unknown;
  in?: unknown[];
}

export interface Field {
  key: string;
  type: FieldType;
  label: string;
  help?: string;
  placeholder?: string;
  required?: boolean;
  options?: Option[];
  validation?: Validation;
  showIf?: Condition;
}

export interface FormSettings {
  public: boolean;
  submitLabel?: string;
  confirmationMessage?: string;
}

export interface FormDef {
  slug: string;
  title: string;
  description?: string;
  workflow?: string;
  settings: FormSettings;
  fields: Field[];
}

export type StateColor = "gray" | "blue" | "green" | "yellow" | "red" | "purple";
export const STATE_COLORS: StateColor[] = ["gray", "blue", "green", "yellow", "red", "purple"];

export interface State {
  key: string;
  label: string;
  color?: StateColor;
  terminal?: boolean;
}

export interface WorkflowField {
  key: string;
  type: FieldType;
  label: string;
  options?: Option[];
}

export interface Guard {
  roles?: string[];
  requireFields?: string[];
}

export type ActionType = "webhook" | "email" | "assign";
export const ACTION_TYPES: ActionType[] = ["email", "webhook", "assign"];
export const ACTION_LABELS: Record<ActionType, string> = {
  email: "Send email",
  webhook: "Call webhook",
  assign: "Assign reviewer",
};

export interface Action {
  type: ActionType;
  url?: string;
  to?: string;
  subject?: string;
  body?: string;
  user?: string;
  role?: string;
}

export interface Transition {
  key: string;
  label: string;
  from: string[];
  to: string;
  guard: Guard;
  actions?: Action[];
}

export interface WorkflowDef {
  slug: string;
  title: string;
  initial: string;
  states: State[];
  fields?: WorkflowField[];
  onSubmit?: Action[];
  transitions: Transition[];
}

export interface Problem {
  path: string;
  message: string;
}

export type Source = "cli" | "ui" | "api" | "seed";
export const SOURCE_LABELS: Record<Source, string> = {
  cli: "CLI (openforms push)",
  ui: "Admin editor",
  api: "API",
  seed: "Seed",
};

export interface VersionSummary {
  version: number;
  source: Source;
  createdBy: string;
  createdAt: string;
}

export type DefinitionKind = "form" | "workflow";
