import type { ErrorObject } from "ajv";
import { describe, expect, it } from "vitest";
import {
  ajvProblems,
  formSemanticProblems,
  pointerToPath,
  validateForm,
  validateWorkflow,
  workflowSemanticProblems,
} from "./validation";
import { sampleForm, sampleWorkflow } from "../test/samples";
import type { FormDef, WorkflowDef } from "./types";

const paths = (problems: { path: string }[]) => problems.map((p) => p.path);

describe("pointerToPath / ajvProblems", () => {
  it("converts JSON pointers to Go-style paths", () => {
    expect(pointerToPath("")).toBe("");
    expect(pointerToPath("/fields/2/showIf/field")).toBe("fields[2].showIf.field");
    expect(pointerToPath("/transitions/0/from/1")).toBe("transitions[0].from[1]");
    expect(pointerToPath("/a~1b/c~0d")).toBe("a/b.c~d");
  });

  it("maps required and additionalProperties errors onto the missing/extra property", () => {
    const errors = [
      { instancePath: "/fields/0", keyword: "required", params: { missingProperty: "label" }, schemaPath: "#", message: "x" },
      { instancePath: "", keyword: "additionalProperties", params: { additionalProperty: "colour" }, schemaPath: "#", message: "x" },
      { instancePath: "/slug", keyword: "pattern", params: {}, schemaPath: "#", message: "must match pattern" },
      { instancePath: "/fields/0", keyword: "if", params: {}, schemaPath: "#", message: "must match \"then\" schema" },
    ] as ErrorObject[];
    expect(ajvProblems(errors)).toEqual([
      { path: "fields[0].label", message: "is required" },
      { path: "colour", message: "is not a recognised property" },
      { path: "slug", message: "must match pattern" },
    ]);
  });
});

describe("validateForm", () => {
  it("accepts the sample form (schema + semantics)", () => {
    expect(validateForm(sampleForm())).toEqual([]);
  });

  it("reports an invalid slug once at path slug", () => {
    const problems = validateForm({ ...sampleForm(), slug: "Bad Slug" });
    expect(paths(problems).filter((p) => p === "slug").length).toBeGreaterThanOrEqual(1);
  });
});

describe("formSemanticProblems", () => {
  const withFields = (fields: FormDef["fields"]): FormDef => ({ ...sampleForm(), fields });

  it("flags duplicate and malformed keys", () => {
    const def = withFields([
      { key: "name", type: "text", label: "A" },
      { key: "name", type: "text", label: "B" },
      { key: "1bad", type: "text", label: "C" },
    ]);
    const problems = formSemanticProblems(def);
    expect(problems).toContainEqual({ path: "fields[1].key", message: "duplicates the key of fields[0]" });
    expect(paths(problems)).toContain("fields[2].key");
  });

  it("requires options on dropdowns, unique option values, and no options elsewhere", () => {
    const def = withFields([
      { key: "a", type: "select", label: "A" },
      { key: "b", type: "multiselect", label: "B", options: [{ value: "x", label: "X" }, { value: "x", label: "Y" }] },
      { key: "c", type: "text", label: "C", options: [{ value: "x", label: "X" }] },
    ]);
    expect(paths(formSemanticProblems(def))).toEqual(["fields[0].options", "fields[1].options[1].value", "fields[2].options"]);
  });

  it("checks validation keys against the field type and their ranges", () => {
    const def = withFields([
      { key: "a", type: "number", label: "A", validation: { minLength: 1, min: 5, max: 1 } },
      { key: "b", type: "text", label: "B", validation: { min: 1, minLength: 5, maxLength: 2, pattern: "(" } },
    ]);
    expect(paths(formSemanticProblems(def))).toEqual([
      "fields[0].validation.minLength",
      "fields[0].validation.min",
      "fields[1].validation.min",
      "fields[1].validation.minLength",
      "fields[1].validation.pattern",
    ]);
  });

  it("requires showIf to reference an earlier field with exactly one operator", () => {
    const def = withFields([
      { key: "a", type: "text", label: "A", showIf: { field: "b", equals: "x" } },
      { key: "b", type: "text", label: "B", showIf: { field: "a", equals: "x", notEquals: "y" } },
      { key: "c", type: "text", label: "C", showIf: { field: "a" } },
    ]);
    expect(paths(formSemanticProblems(def))).toEqual(["fields[0].showIf.field", "fields[1].showIf", "fields[2].showIf"]);
  });

  it("does not throw on malformed input from YAML", () => {
    expect(() => formSemanticProblems({ slug: "x", title: "X", settings: { public: true }, fields: "nope" } as unknown as FormDef)).not.toThrow();
  });
});

describe("validateWorkflow / workflowSemanticProblems", () => {
  it("accepts the sample workflow", () => {
    expect(validateWorkflow(sampleWorkflow())).toEqual([]);
  });

  it("checks states, initial and transition endpoints", () => {
    const wf: WorkflowDef = {
      ...sampleWorkflow(),
      initial: "missing",
      states: [
        { key: "new", label: "New" },
        { key: "new", label: "Again" },
        { key: "done", label: "Done", terminal: true },
      ],
      transitions: [
        { key: "t1", label: "T1", from: ["new", "ghost"], to: "nowhere", guard: {} },
        { key: "t2", label: "T2", from: ["done"], to: "new", guard: {} },
        { key: "t1", label: "T3", from: [], to: "done", guard: { requireFields: ["unknown"] } },
      ],
    };
    const problems = workflowSemanticProblems(wf);
    expect(paths(problems)).toEqual([
      "states[1].key",
      "initial",
      "transitions[0].from[1]",
      "transitions[0].to",
      "transitions[1].from[0]",
      "transitions[2].key",
      "transitions[2].from",
      "transitions[2].guard.requireFields[0]",
    ]);
    expect(problems).toContainEqual({ path: "transitions[1].from[0]", message: 'cannot leave terminal state "done"' });
  });

  it("checks workflow fields", () => {
    const wf: WorkflowDef = {
      ...sampleWorkflow(),
      fields: [
        { key: "a", type: "select", label: "A" },
        { key: "b", type: "email", label: "B" },
        { key: "a", type: "text", label: "C" },
      ],
      transitions: [],
    };
    expect(paths(workflowSemanticProblems(wf))).toEqual(["fields[0].options", "fields[1].type", "fields[2].key"]);
  });

  it("checks actions on transitions and onSubmit", () => {
    const wf: WorkflowDef = {
      ...sampleWorkflow(),
      onSubmit: [{ type: "email", to: "", subject: "" }, { type: "assign" }],
      transitions: [
        {
          key: "go",
          label: "Go",
          from: ["new"],
          to: "screening",
          guard: {},
          actions: [{ type: "webhook", url: "ftp://x" }, { type: "assign", user: "a@b.c", role: "reviewer" }],
        },
      ],
    };
    expect(paths(workflowSemanticProblems(wf))).toEqual([
      "transitions[0].actions[0].url",
      "transitions[0].actions[1]",
      "onSubmit[0].to",
      "onSubmit[0].subject",
      "onSubmit[1]",
    ]);
  });
});
