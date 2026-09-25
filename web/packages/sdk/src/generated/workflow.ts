/* eslint-disable */
/**
 * GENERATED from schemas/workflow.schema.json by web/packages/sdk/scripts/gen-types.mjs.
 * Do not edit by hand: run `pnpm -C web/packages/sdk gen`.
 */

export type Slug = string;
export type Key = string;

export interface WorkflowDefinition {
  $schema?: string;
  slug: Slug;
  title: string;
  initial: Key;
  /**
   * @minItems 1
   * @maxItems 100
   */
  states: [State, ...State[]];
  /**
   * @maxItems 100
   */
  fields?: WorkflowField[];
  /**
   * @maxItems 20
   */
  onSubmit?:
    | []
    | [Action]
    | [Action, Action]
    | [Action, Action, Action]
    | [Action, Action, Action, Action]
    | [Action, Action, Action, Action, Action]
    | [Action, Action, Action, Action, Action, Action]
    | [Action, Action, Action, Action, Action, Action, Action]
    | [Action, Action, Action, Action, Action, Action, Action, Action]
    | [Action, Action, Action, Action, Action, Action, Action, Action, Action]
    | [Action, Action, Action, Action, Action, Action, Action, Action, Action, Action]
    | [Action, Action, Action, Action, Action, Action, Action, Action, Action, Action, Action]
    | [
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
      ]
    | [
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
      ]
    | [
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
      ]
    | [
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
      ]
    | [
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
      ]
    | [
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
      ]
    | [
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
      ]
    | [
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
      ]
    | [
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
      ];
  /**
   * @maxItems 200
   */
  transitions?: Transition[];
}
export interface State {
  key: Key;
  label: string;
  color?: "gray" | "blue" | "green" | "yellow" | "red" | "purple";
  terminal?: boolean;
}
export interface WorkflowField {
  key: Key;
  type: "text" | "textarea" | "number" | "select" | "checkbox" | "date";
  label: string;
  /**
   * @maxItems 500
   */
  options?: Option[];
}
export interface Option {
  value: string;
  label: string;
}
export interface Action {
  type: "webhook" | "email" | "assign";
  url?: string;
  to?: string;
  subject?: string;
  body?: string;
  user?: string;
  role?: string;
}
export interface Transition {
  key: Key;
  label: string;
  /**
   * @minItems 1
   */
  from: [Key, ...Key[]];
  to: Key;
  guard?: Guard;
  /**
   * @maxItems 20
   */
  actions?:
    | []
    | [Action]
    | [Action, Action]
    | [Action, Action, Action]
    | [Action, Action, Action, Action]
    | [Action, Action, Action, Action, Action]
    | [Action, Action, Action, Action, Action, Action]
    | [Action, Action, Action, Action, Action, Action, Action]
    | [Action, Action, Action, Action, Action, Action, Action, Action]
    | [Action, Action, Action, Action, Action, Action, Action, Action, Action]
    | [Action, Action, Action, Action, Action, Action, Action, Action, Action, Action]
    | [Action, Action, Action, Action, Action, Action, Action, Action, Action, Action, Action]
    | [
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
      ]
    | [
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
      ]
    | [
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
      ]
    | [
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
      ]
    | [
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
      ]
    | [
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
      ]
    | [
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
      ]
    | [
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
      ]
    | [
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
        Action,
      ];
}
export interface Guard {
  roles?: string[];
  requireFields?: Key[];
}
