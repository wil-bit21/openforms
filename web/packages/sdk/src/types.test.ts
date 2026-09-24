import { describe, expect, expectTypeOf, it } from "vitest";
import type {
  Action,
  Condition,
  Field,
  FieldType,
  FormDefinition,
  Option,
  PublicStatus,
  State,
  Submission,
  Transition,
  WorkflowDefinition,
} from "./index.js";

// The spec §5.1 / §5.2 examples must type-check against the generated types.
const form: FormDefinition = {
  slug: "job-application",
  title: "Job application",
  description: "Apply to join the team.",
  workflow: "hiring",
  settings: { public: true, submitLabel: "Send application", confirmationMessage: "Thanks!" },
  fields: [
    { key: "name", type: "text", label: "Full name", required: true, validation: { minLength: 2, maxLength: 100 } },
    {
      key: "role",
      type: "select",
      label: "Role",
      required: true,
      options: [
        { value: "engineer", label: "Engineer" },
        { value: "designer", label: "Designer" },
      ],
    },
    { key: "portfolio", type: "url", label: "Portfolio URL", showIf: { field: "role", equals: "designer" } },
  ],
};

const workflow: WorkflowDefinition = {
  slug: "hiring",
  title: "Hiring pipeline",
  initial: "new",
  states: [
    { key: "new", label: "New", color: "gray" },
    { key: "hired", label: "Hired", color: "green", terminal: true },
  ],
  fields: [{ key: "score", type: "number", label: "Score" }],
  onSubmit: [{ type: "assign", role: "reviewer" }],
  transitions: [
    {
      key: "hire",
      label: "Hire",
      from: ["new"],
      to: "hired",
      guard: { roles: ["hiring-manager"], requireFields: ["score"] },
      actions: [{ type: "webhook", url: "https://example.com/hook" }],
    },
  ],
};

describe("generated definition types", () => {
  it("accepts the spec examples", () => {
    expect(form.slug).toBe("job-application");
    expect(workflow.initial).toBe("new");
  });

  it("derives element types", () => {
    expectTypeOf<"multiselect">().toMatchTypeOf<FieldType>();
    expectTypeOf<{ value: string; label: string }>().toMatchTypeOf<Option>();
    expectTypeOf<{ field: string; in: string[] }>().toMatchTypeOf<Condition>();
    expectTypeOf<{ key: string; label: string }>().toMatchTypeOf<State>();
    expectTypeOf<{ type: "email"; to: string; subject: string; body: string }>().toMatchTypeOf<Action>();
    expectTypeOf<Field>().toHaveProperty("key");
    expectTypeOf<Transition>().toHaveProperty("guard");
  });

  it("exposes wire types", () => {
    expectTypeOf<Submission>().toHaveProperty("stateLabel");
    expectTypeOf<PublicStatus["states"]>().toEqualTypeOf<State[]>();
  });
});
