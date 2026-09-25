import { applyPatch, clone, isObject, nextNumbered, objects, str, unique } from "../shared/objects";
import type { Action, ActionType, Guard, State, Transition, WorkflowDef, WorkflowField } from "../shared/types";

export type WorkflowSelection =
  | { kind: "workflow" }
  | { kind: "state"; index: number }
  | { kind: "transition"; index: number }
  | { kind: "field"; index: number };

export type ActionTarget = { kind: "transition"; index: number } | { kind: "onSubmit" };

export interface WorkflowEditorState {
  def: WorkflowDef;
  selection: WorkflowSelection;
  dirty: boolean;
}

export type StatePatch = Partial<Pick<State, "label" | "color" | "terminal">>;
export type TransitionPatch = Partial<Pick<Transition, "key" | "label" | "from" | "to">>;
export type WorkflowFieldPatch = Partial<Pick<WorkflowField, "label" | "type" | "options">>;

export type WorkflowEditorAction =
  | { type: "setMeta"; patch: Partial<Pick<WorkflowDef, "slug" | "title">> }
  | { type: "setInitial"; key: string }
  | { type: "addState" }
  | { type: "updateState"; index: number; patch: StatePatch }
  | { type: "renameState"; index: number; key: string }
  | { type: "removeState"; index: number }
  | { type: "addTransition" }
  | { type: "updateTransition"; index: number; patch: TransitionPatch }
  | { type: "setGuard"; index: number; guard: Guard }
  | { type: "removeTransition"; index: number }
  | { type: "addField" }
  | { type: "updateField"; index: number; patch: WorkflowFieldPatch }
  | { type: "renameField"; index: number; key: string }
  | { type: "removeField"; index: number }
  | { type: "addAction"; target: ActionTarget; actionType: ActionType }
  | { type: "updateAction"; target: ActionTarget; actionIndex: number; patch: Partial<Action> }
  | { type: "removeAction"; target: ActionTarget; actionIndex: number }
  | { type: "select"; selection: WorkflowSelection }
  | { type: "replace"; def: WorkflowDef }
  | { type: "markSaved" };

const STARTER_OPTIONS = () => [
  { value: "option1", label: "Option 1" },
  { value: "option2", label: "Option 2" },
];

export function newWorkflowDefinition(): WorkflowDef {
  return {
    slug: "",
    title: "Untitled workflow",
    initial: "new",
    states: [
      { key: "new", label: "New", color: "gray" },
      { key: "done", label: "Done", color: "green", terminal: true },
    ],
    transitions: [{ key: "complete", label: "Mark done", from: ["new"], to: "done", guard: {} }],
  };
}

export function newAction(type: ActionType): Action {
  switch (type) {
    case "webhook":
      return { type, url: "https://" };
    case "email":
      return { type, to: "", subject: "", body: "" };
    case "assign":
      return { type, role: "" };
  }
}

export function normalizeWorkflow(input: unknown): WorkflowDef {
  const d = isObject(input) ? input : {};
  const out = {
    ...d,
    slug: str(d.slug),
    title: str(d.title),
    initial: str(d.initial),
    states: objects<State>(d.states),
    transitions: objects<Transition>(d.transitions).map((t) => ({
      ...t,
      from: Array.isArray(t.from) ? t.from.filter((k): k is string => typeof k === "string") : [],
      guard: isObject(t.guard) ? (t.guard as Guard) : {},
    })),
  } as WorkflowDef;
  const fields = objects<WorkflowField>(d.fields);
  if (fields.length) out.fields = fields;
  else delete out.fields;
  const onSubmit = objects<Action>(d.onSubmit);
  if (onSubmit.length) out.onSubmit = onSubmit;
  else delete out.onSubmit;
  return out;
}

export function initWorkflowEditor(def: WorkflowDef): WorkflowEditorState {
  return { def: normalizeWorkflow(clone(def)), selection: { kind: "workflow" }, dirty: false };
}

function validSelection(def: WorkflowDef, selection: WorkflowSelection): WorkflowSelection {
  if (selection.kind === "workflow") return selection;
  const size =
    selection.kind === "state" ? def.states.length : selection.kind === "transition" ? def.transitions.length : (def.fields ?? []).length;
  return selection.index < size ? selection : { kind: "workflow" };
}

const change = (state: WorkflowEditorState, def: WorkflowDef, selection = state.selection): WorkflowEditorState => ({
  def,
  selection,
  dirty: true,
});

const count = (items: { key: string }[], key: string) => items.filter((i) => i.key === key).length;

function orderByStates(def: WorkflowDef, from: string[]): string[] {
  const wanted = unique(from);
  const known = def.states.map((s) => s.key).filter((k) => wanted.includes(k));
  return unique([...known, ...wanted.filter((k) => !known.includes(k))]);
}

function normalizeGuard(guard: Guard): Guard {
  const out: Guard = {};
  const roles = unique((guard.roles ?? []).map((r) => r.trim()).filter(Boolean));
  const requireFields = unique(guard.requireFields ?? []);
  if (roles.length) out.roles = roles;
  if (requireFields.length) out.requireFields = requireFields;
  return out;
}

function getActions(def: WorkflowDef, target: ActionTarget): Action[] {
  return target.kind === "onSubmit" ? def.onSubmit ?? [] : def.transitions[target.index]?.actions ?? [];
}

function setActions(def: WorkflowDef, target: ActionTarget, actions: Action[]): WorkflowDef {
  if (target.kind === "onSubmit") {
    if (actions.length === 0) {
      const { onSubmit: _omit, ...rest } = def;
      return rest;
    }
    return { ...def, onSubmit: actions };
  }
  return {
    ...def,
    transitions: def.transitions.map((t, i) => {
      if (i !== target.index) return t;
      if (actions.length === 0) {
        const { actions: _omit, ...rest } = t;
        return rest;
      }
      return { ...t, actions };
    }),
  };
}

function withRequireFields(t: Transition, map: (keys: string[]) => string[]): Transition {
  const guard = normalizeGuard({ ...t.guard, requireFields: map(t.guard.requireFields ?? []) });
  return { ...t, guard };
}

export function workflowEditorReducer(state: WorkflowEditorState, action: WorkflowEditorAction): WorkflowEditorState {
  const def = state.def;
  const fields = def.fields ?? [];
  switch (action.type) {
    case "setMeta":
      return change(state, { ...def, ...action.patch });
    case "setInitial":
      return change(state, { ...def, initial: action.key });

    case "addState": {
      const key = nextNumbered("state", def.states.map((s) => s.key));
      const states = [...def.states, { key, label: "New state", color: "gray" as const }];
      const initial = def.states.some((s) => s.key === def.initial) ? def.initial : key;
      return change(state, { ...def, states, initial }, { kind: "state", index: states.length - 1 });
    }
    case "updateState": {
      if (!def.states[action.index]) return state;
      const states = def.states.map((s, i) => {
        if (i !== action.index) return s;
        const next = { ...s, ...action.patch };
        if (!next.terminal) delete next.terminal;
        if (!next.color) delete next.color;
        return next;
      });
      return change(state, { ...def, states });
    }
    case "renameState": {
      const target = def.states[action.index];
      if (!target) return state;
      const oldKey = target.key;
      const follow = count(def.states, oldKey) === 1;
      const states = def.states.map((s, i) => (i === action.index ? { ...s, key: action.key } : s));
      if (!follow) return change(state, { ...def, states });
      const swap = (k: string) => (k === oldKey ? action.key : k);
      return change(state, {
        ...def,
        states,
        initial: swap(def.initial),
        transitions: def.transitions.map((t) => ({ ...t, from: t.from.map(swap), to: swap(t.to) })),
      });
    }
    case "removeState": {
      const target = def.states[action.index];
      if (!target) return state;
      const states = def.states.filter((_, i) => i !== action.index);
      let transitions = def.transitions;
      let initial = def.initial;
      if (count(states, target.key) === 0) {
        transitions = transitions
          .filter((t) => t.to !== target.key)
          .map((t) => ({ ...t, from: t.from.filter((k) => k !== target.key) }))
          .filter((t) => t.from.length > 0);
        if (initial === target.key) initial = states[0]?.key ?? "";
      }
      return change(state, { ...def, states, transitions, initial }, { kind: "workflow" });
    }

    case "addTransition": {
      const key = nextNumbered("transition", def.transitions.map((t) => t.key));
      const source = def.states.some((s) => s.key === def.initial) ? def.initial : def.states[0]?.key;
      const from = source ? [source] : [];
      const to = def.states.find((s) => s.key !== source)?.key ?? source ?? "";
      const transitions = [...def.transitions, { key, label: "New transition", from, to, guard: {} }];
      return change(state, { ...def, transitions }, { kind: "transition", index: transitions.length - 1 });
    }
    case "updateTransition": {
      if (!def.transitions[action.index]) return state;
      const transitions = def.transitions.map((t, i) => {
        if (i !== action.index) return t;
        const next = { ...t, ...action.patch };
        if (action.patch.from) next.from = orderByStates(def, action.patch.from);
        return next;
      });
      return change(state, { ...def, transitions });
    }
    case "setGuard": {
      if (!def.transitions[action.index]) return state;
      const transitions = def.transitions.map((t, i) => (i === action.index ? { ...t, guard: normalizeGuard(action.guard) } : t));
      return change(state, { ...def, transitions });
    }
    case "removeTransition": {
      if (!def.transitions[action.index]) return state;
      return change(state, { ...def, transitions: def.transitions.filter((_, i) => i !== action.index) }, { kind: "workflow" });
    }

    case "addField": {
      const key = nextNumbered("field", fields.map((f) => f.key));
      const next = [...fields, { key, type: "text" as const, label: "New field" }];
      return change(state, { ...def, fields: next }, { kind: "field", index: next.length - 1 });
    }
    case "updateField": {
      if (!fields[action.index]) return state;
      const next = fields.map((f, i) => {
        if (i !== action.index) return f;
        const updated = { ...f, ...action.patch };
        if (updated.type === "select") {
          if (!updated.options?.length) updated.options = STARTER_OPTIONS();
        } else {
          delete updated.options;
        }
        return updated;
      });
      return change(state, { ...def, fields: next });
    }
    case "renameField": {
      const target = fields[action.index];
      if (!target) return state;
      const follow = count(fields, target.key) === 1;
      const next = fields.map((f, i) => (i === action.index ? { ...f, key: action.key } : f));
      const transitions = follow
        ? def.transitions.map((t) =>
            t.guard.requireFields?.includes(target.key)
              ? withRequireFields(t, (keys) => keys.map((k) => (k === target.key ? action.key : k)))
              : t,
          )
        : def.transitions;
      return change(state, { ...def, fields: next, transitions });
    }
    case "removeField": {
      const target = fields[action.index];
      if (!target) return state;
      const next = fields.filter((_, i) => i !== action.index);
      const transitions =
        count(next, target.key) === 0
          ? def.transitions.map((t) =>
              t.guard.requireFields?.includes(target.key) ? withRequireFields(t, (keys) => keys.filter((k) => k !== target.key)) : t,
            )
          : def.transitions;
      const updated: WorkflowDef = { ...def, fields: next, transitions };
      if (next.length === 0) delete updated.fields;
      return change(state, updated, { kind: "workflow" });
    }

    case "addAction":
      return change(state, setActions(def, action.target, [...getActions(def, action.target), newAction(action.actionType)]));
    case "updateAction": {
      const actions = getActions(def, action.target);
      const current = actions[action.actionIndex];
      if (!current) return state;
      const replaced =
        action.patch.type && action.patch.type !== current.type
          ? newAction(action.patch.type)
          : applyPatch(current, action.patch, { dropEmptyStrings: false });
      return change(state, setActions(def, action.target, actions.map((a, i) => (i === action.actionIndex ? replaced : a))));
    }
    case "removeAction": {
      const actions = getActions(def, action.target);
      if (!actions[action.actionIndex]) return state;
      return change(state, setActions(def, action.target, actions.filter((_, i) => i !== action.actionIndex)));
    }

    case "select":
      return { ...state, selection: validSelection(def, action.selection) };
    case "replace": {
      const next = normalizeWorkflow(clone(action.def));
      return { def: next, selection: validSelection(next, state.selection), dirty: true };
    }
    case "markSaved":
      return { ...state, dirty: false };
  }
}
