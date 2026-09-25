/* eslint-disable */
/**
 * GENERATED from schemas/form.schema.json by web/packages/sdk/scripts/gen-types.mjs.
 * Do not edit by hand: run `pnpm -C web/packages/sdk gen`.
 */

export type Slug = string;
export type Key = string;
export type FieldType =
  "text" | "textarea" | "email" | "number" | "select" | "multiselect" | "checkbox" | "date" | "url";

export interface FormDefinition {
  $schema?: string;
  slug: Slug;
  title: string;
  description?: string;
  workflow?: Slug;
  settings?: Settings;
  /**
   * @minItems 1
   * @maxItems 200
   */
  fields: [Field, ...Field[]];
}
export interface Settings {
  public?: boolean;
  submitLabel?: string;
  confirmationMessage?: string;
}
export interface Field {
  key: Key;
  type: FieldType;
  label: string;
  help?: string;
  placeholder?: string;
  required?: boolean;
  /**
   * @maxItems 500
   */
  options?: Option[];
  validation?: Validation;
  showIf?: Condition;
}
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
  field: Key;
  equals?: unknown;
  notEquals?: unknown;
  in?: unknown[];
}
