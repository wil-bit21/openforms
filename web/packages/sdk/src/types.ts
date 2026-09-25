import type { FormDefinition } from "./generated/form.js";
import type { WorkflowDefinition } from "./generated/workflow.js";

export type { FormDefinition, WorkflowDefinition };

// Element/property helpers that work whether the generator emits a flat
// interface or a union per field type.
type Item<T> = T extends readonly (infer U)[] ? U : never;
type Prop<T, K extends PropertyKey> = T extends unknown ? (K extends keyof T ? T[K] : never) : never;

export type Field = Item<NonNullable<FormDefinition["fields"]>>;
export type FieldType = Prop<Field, "type">;
export type Option = Item<NonNullable<Prop<Field, "options">>>;
export type Validation = NonNullable<Prop<Field, "validation">>;
export type Condition = NonNullable<Prop<Field, "showIf">>;
export type FormSettings = NonNullable<FormDefinition["settings"]>;

export type State = Item<NonNullable<WorkflowDefinition["states"]>>;
export type StateColor = NonNullable<Prop<State, "color">>;
export type WorkflowField = Item<NonNullable<WorkflowDefinition["fields"]>>;
export type Transition = Item<NonNullable<WorkflowDefinition["transitions"]>>;
export type Guard = NonNullable<Prop<Transition, "guard">>;
export type Action = Item<NonNullable<Prop<Transition, "actions">>>;
export type ActionType = Prop<Action, "type">;
