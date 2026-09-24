import { describe, expect, it } from "vitest";
import {
  initWorkflowEditor,
  newAction,
  newWorkflowDefinition,
  normalizeWorkflow,
  workflowEditorReducer as reduce,
} from "./workflowReducer";
import { sampleWorkflow } from "../test/samples";

const init = () => initWorkflowEditor(sampleWorkflow());
const t = (s: ReturnType<typeof init>, key: string) => s.def.transitions.find((x) => x.key === key);

describe("init and meta", () => {
  it("starts clean with workflow settings selected", () => {
    expect(init()).toEqual({ def: sampleWorkflow(), selection: { kind: "workflow" }, dirty: false });
  });

  it("sets slug, title and initial state", () => {
    let s = reduce(init(), { type: "setMeta", patch: { slug: "", title: "Pipeline" } });
    s = reduce(s, { type: "setInitial", key: "screening" });
    expect(s.def).toMatchObject({ slug: "", title: "Pipeline", initial: "screening" });
    expect(s.dirty).toBe(true);
  });
});

describe("states", () => {
  it("adds a uniquely keyed state and selects it", () => {
    const s = reduce(init(), { type: "addState" });
    expect(s.def.states[4]).toEqual({ key: "state1", label: "New state", color: "gray" });
    expect(s.selection).toEqual({ kind: "state", index: 4 });
  });

  it("makes the first state of an empty workflow the initial state", () => {
    const empty = initWorkflowEditor({ ...sampleWorkflow(), initial: "", states: [], transitions: [] });
    expect(reduce(empty, { type: "addState" }).def.initial).toBe("state1");
  });

  it("updates a state and drops terminal=false", () => {
    let s = reduce(init(), { type: "updateState", index: 2, patch: { label: "Offer accepted", color: "purple" } });
    expect(s.def.states[2]).toMatchObject({ label: "Offer accepted", color: "purple", terminal: true });
    s = reduce(s, { type: "updateState", index: 2, patch: { terminal: false } });
    expect("terminal" in s.def.states[2]).toBe(false);
  });

  it("renaming a state updates initial, from and to", () => {
    const s = reduce(init(), { type: "renameState", index: 0, key: "received" });
    expect(s.def.initial).toBe("received");
    expect(t(s, "screen")?.from).toEqual(["received"]);
    expect(t(s, "reject")?.from).toEqual(["received", "screening"]);
    const s2 = reduce(init(), { type: "renameState", index: 1, key: "review" });
    expect(t(s2, "screen")?.to).toBe("review");
    expect(t(s2, "hire")?.from).toEqual(["review"]);
  });

  it("renaming one of two duplicate state keys leaves references alone", () => {
    let s = reduce(init(), { type: "renameState", index: 1, key: "new" });
    s = reduce(s, { type: "renameState", index: 1, key: "screening" });
    expect(s.def.initial).toBe("new");
    expect(t(s, "screen")?.from).toEqual(["new"]);
  });

  it("removing a state deletes transitions into it and prunes it from sources", () => {
    const s = reduce(reduce(init(), { type: "select", selection: { kind: "state", index: 1 } }), { type: "removeState", index: 1 });
    expect(s.def.states.map((x) => x.key)).toEqual(["new", "hired", "rejected"]);
    expect(s.def.transitions.map((x) => x.key)).toEqual(["reject"]);
    expect(t(s, "reject")?.from).toEqual(["new"]);
    expect(s.selection).toEqual({ kind: "workflow" });
  });

  it("removing the initial state moves initial to the first remaining state", () => {
    const s = reduce(init(), { type: "removeState", index: 0 });
    expect(s.def.initial).toBe("screening");
    expect(s.def.transitions.map((x) => x.key)).toEqual(["hire", "reject"]);
  });
});

describe("transitions", () => {
  it("adds a transition from the initial state to another state", () => {
    const s = reduce(init(), { type: "addTransition" });
    expect(s.def.transitions[3]).toEqual({ key: "transition1", label: "New transition", from: ["new"], to: "screening", guard: {} });
    expect(s.selection).toEqual({ kind: "transition", index: 3 });
  });

  it("orders and de-duplicates sources by state order", () => {
    const s = reduce(init(), { type: "updateTransition", index: 0, patch: { from: ["screening", "new", "new"] } });
    expect(s.def.transitions[0].from).toEqual(["new", "screening"]);
  });

  it("keeps empty key and label so validation can flag them", () => {
    const s = reduce(init(), { type: "updateTransition", index: 0, patch: { key: "", label: "" } });
    expect(s.def.transitions[0]).toMatchObject({ key: "", label: "" });
  });

  it("normalises guards", () => {
    const s = reduce(init(), { type: "setGuard", index: 0, guard: { roles: ["a", "a", "b"], requireFields: [] } });
    expect(s.def.transitions[0].guard).toEqual({ roles: ["a", "b"] });
  });

  it("removes a transition", () => {
    const s = reduce(init(), { type: "removeTransition", index: 1 });
    expect(s.def.transitions.map((x) => x.key)).toEqual(["screen", "reject"]);
  });
});

describe("workflow fields", () => {
  it("adds, retypes and renames fields, keeping requireFields in sync", () => {
    let s = reduce(init(), { type: "addField" });
    expect(s.def.fields?.[2]).toEqual({ key: "field1", type: "text", label: "New field" });
    s = reduce(s, { type: "updateField", index: 2, patch: { type: "select" } });
    expect(s.def.fields?.[2].options).toHaveLength(2);
    s = reduce(s, { type: "updateField", index: 2, patch: { type: "text" } });
    expect("options" in (s.def.fields?.[2] ?? {})).toBe(false);
    s = reduce(s, { type: "renameField", index: 0, key: "rating" });
    expect(t(s, "hire")?.guard.requireFields).toEqual(["rating"]);
  });

  it("removing a field prunes requireFields and drops empty collections", () => {
    let s = reduce(init(), { type: "removeField", index: 1 });
    expect(t(s, "reject")?.guard).toEqual({});
    s = reduce(s, { type: "removeField", index: 0 });
    expect(t(s, "hire")?.guard).toEqual({ roles: ["hiring-manager"] });
    expect("fields" in s.def).toBe(false);
  });
});

describe("actions", () => {
  it("adds, edits and removes onSubmit actions", () => {
    let s = reduce(init(), { type: "addAction", target: { kind: "onSubmit" }, actionType: "email" });
    expect(s.def.onSubmit).toEqual([{ type: "email", to: "", subject: "", body: "" }]);
    s = reduce(s, { type: "updateAction", target: { kind: "onSubmit" }, actionIndex: 0, patch: { subject: "" } });
    expect(s.def.onSubmit?.[0].subject).toBe("");
    s = reduce(s, { type: "removeAction", target: { kind: "onSubmit" }, actionIndex: 0 });
    expect("onSubmit" in s.def).toBe(false);
  });

  it("switches assign between role and user keeping the empty value", () => {
    let s = reduce(init(), { type: "addAction", target: { kind: "onSubmit" }, actionType: "assign" });
    expect(s.def.onSubmit?.[0]).toEqual({ type: "assign", role: "" });
    s = reduce(s, { type: "updateAction", target: { kind: "onSubmit" }, actionIndex: 0, patch: { user: "", role: undefined } });
    expect(s.def.onSubmit?.[0]).toEqual({ type: "assign", user: "" });
  });

  it("changing an action's type replaces it with a fresh action", () => {
    const s = reduce(init(), { type: "updateAction", target: { kind: "transition", index: 2 }, actionIndex: 0, patch: { type: "webhook" } });
    expect(s.def.transitions[2].actions).toEqual([{ type: "webhook", url: "https://" }]);
  });

  it("adds and removes transition actions, dropping the empty list", () => {
    let s = reduce(init(), { type: "addAction", target: { kind: "transition", index: 0 }, actionType: "webhook" });
    expect(s.def.transitions[0].actions).toEqual([{ type: "webhook", url: "https://" }]);
    s = reduce(s, { type: "removeAction", target: { kind: "transition", index: 0 }, actionIndex: 0 });
    expect("actions" in s.def.transitions[0]).toBe(false);
  });
});

describe("replace and helpers", () => {
  it("normalises malformed YAML input", () => {
    const s = reduce(init(), {
      type: "replace",
      def: { slug: "x", transitions: [{ key: "a", label: "A", from: "new", to: "b" }, 3] } as never,
    });
    expect(s.def).toEqual({
      slug: "x",
      title: "",
      initial: "",
      states: [],
      transitions: [{ key: "a", label: "A", from: [], to: "b", guard: {} }],
    });
    expect(s.dirty).toBe(true);
  });

  it("falls back to workflow settings when the selection disappears", () => {
    let s = reduce(init(), { type: "select", selection: { kind: "state", index: 3 } });
    s = reduce(s, { type: "replace", def: { ...sampleWorkflow(), states: sampleWorkflow().states.slice(0, 2) } });
    expect(s.selection).toEqual({ kind: "workflow" });
  });

  it("markSaved clears dirty", () => {
    expect(reduce(reduce(init(), { type: "addState" }), { type: "markSaved" }).dirty).toBe(false);
  });

  it("newWorkflowDefinition and newAction produce valid starting shapes", () => {
    expect(newWorkflowDefinition()).toEqual({
      slug: "",
      title: "Untitled workflow",
      initial: "new",
      states: [
        { key: "new", label: "New", color: "gray" },
        { key: "done", label: "Done", color: "green", terminal: true },
      ],
      transitions: [{ key: "complete", label: "Mark done", from: ["new"], to: "done", guard: {} }],
    });
    expect(newAction("assign")).toEqual({ type: "assign", role: "" });
  });

  it("normalizeWorkflow keeps non-empty fields and onSubmit", () => {
    const def = normalizeWorkflow({ ...sampleWorkflow(), onSubmit: [{ type: "assign", role: "reviewer" }] });
    expect(def.fields).toHaveLength(2);
    expect(def.onSubmit).toEqual([{ type: "assign", role: "reviewer" }]);
  });
});
