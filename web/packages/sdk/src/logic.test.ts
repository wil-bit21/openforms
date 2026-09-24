import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import {
  FORM_ERROR_KEY,
  isProvided,
  problemsToErrors,
  validateSubmission,
  visible,
  type LogicForm,
  type Values,
} from "./logic.js";

function fixture<T>(name: string): T {
  // web/packages/sdk/src → repo root is four levels up.
  return JSON.parse(readFileSync(new URL(`../../../../schemas/fixtures/${name}`, import.meta.url), "utf8")) as T;
}

interface VisibilityCase {
  name: string;
  form: LogicForm;
  data: Values;
  visible: Record<string, boolean>;
}
interface SubmissionCase {
  name: string;
  form: LogicForm;
  data: Values;
  clean: Values | null;
  errorPaths: string[];
}

const uniqSorted = (xs: string[]) => [...new Set(xs)].sort();

describe("conformance fixtures (shared with Go)", () => {
  const visibility = fixture<VisibilityCase[]>("visibility.json");
  const submission = fixture<SubmissionCase[]>("submission.json");

  it("has fixtures to run", () => {
    expect(visibility.length).toBeGreaterThan(0);
    expect(submission.length).toBeGreaterThan(0);
  });

  it.each(visibility)("visibility: $name", (c) => {
    const got = visible(c.form, c.data);
    for (const [key, want] of Object.entries(c.visible)) {
      expect({ key, visible: got[key] }).toEqual({ key, visible: want });
    }
  });

  it.each(submission)("submission: $name", (c) => {
    const got = validateSubmission(c.form, c.data);
    if (c.clean !== null) {
      expect(got.problems).toEqual([]);
      expect(got.clean).toEqual(c.clean);
    } else {
      expect(got.clean).toBeNull();
      expect(uniqSorted(got.problems.map((p) => p.path))).toEqual(uniqSorted(c.errorPaths));
    }
  });
});

const hiring: LogicForm = {
  fields: [
    { key: "role", type: "select", required: true, options: [{ value: "engineer" }, { value: "designer" }] },
    { key: "portfolio", type: "url", required: true, showIf: { field: "role", equals: "designer" } },
    { key: "portfolioNote", type: "text", showIf: { field: "portfolio", notEquals: "https://skip.example" } },
    { key: "skills", type: "multiselect", options: [{ value: "go" }, { value: "ts" }] },
    { key: "goYears", type: "number", validation: { min: 0, max: 50 }, showIf: { field: "skills", equals: "go" } },
    { key: "start", type: "date" },
    { key: "consent", type: "checkbox", required: true },
  ],
};

describe("visible", () => {
  it("hides a field whose condition fails and cascades to its dependents", () => {
    const v = visible(hiring, { role: "engineer", portfolio: "https://a.example" });
    expect(v).toMatchObject({ role: true, portfolio: false, portfolioNote: false });
  });

  it("shows dependents when the chain holds", () => {
    const v = visible(hiring, { role: "designer", portfolio: "https://a.example" });
    expect(v).toMatchObject({ portfolio: true, portfolioNote: true });
  });

  it("treats equals on a multiselect controller as 'contains'", () => {
    expect(visible(hiring, { skills: ["ts", "go"] }).goYears).toBe(true);
    expect(visible(hiring, { skills: ["ts"] }).goYears).toBe(false);
    expect(visible(hiring, {}).goYears).toBe(false);
  });

  it("supports `in`", () => {
    const form: LogicForm = {
      fields: [
        { key: "plan", type: "select", options: [{ value: "free" }, { value: "pro" }, { value: "team" }] },
        { key: "seats", type: "number", showIf: { field: "plan", in: ["pro", "team"] } },
      ],
    };
    expect(visible(form, { plan: "team" }).seats).toBe(true);
    expect(visible(form, { plan: "free" }).seats).toBe(false);
  });
});

describe("validateSubmission", () => {
  const valid = { role: "engineer", consent: true };

  it("drops hidden and unknown values from the clean data", () => {
    const r = validateSubmission(hiring, { ...valid, portfolio: "https://stale.example", extra: 1 });
    expect(r).toEqual({ clean: { role: "engineer", consent: true }, problems: [] });
  });

  it("requires visible required fields and treats empty values as missing", () => {
    const r = validateSubmission(hiring, { role: "designer", portfolio: "", consent: true });
    expect(r.clean).toBeNull();
    expect(r.problems.map((p) => p.path)).toEqual(["data.portfolio"]);
  });

  it("requires a required checkbox to be true", () => {
    expect(validateSubmission(hiring, { role: "engineer", consent: false }).problems.map((p) => p.path)).toEqual([
      "data.consent",
    ]);
  });

  it("rejects values outside the option list and duplicate multiselect values", () => {
    const r = validateSubmission(hiring, { role: "manager", skills: ["go", "go"], consent: true });
    expect(r.problems.map((p) => p.path)).toEqual(["data.role", "data.skills"]);
  });

  it("checks number bounds and types", () => {
    expect(validateSubmission(hiring, { ...valid, skills: ["go"], goYears: 51 }).problems.map((p) => p.path)).toEqual([
      "data.goYears",
    ]);
    expect(validateSubmission(hiring, { ...valid, skills: ["go"], goYears: "3" }).problems.map((p) => p.path)).toEqual([
      "data.goYears",
    ]);
    expect(validateSubmission(hiring, { ...valid, skills: ["go"], goYears: 3 }).clean).toEqual({
      ...valid,
      skills: ["go"],
      goYears: 3,
    });
  });

  it("validates calendar dates", () => {
    expect(validateSubmission(hiring, { ...valid, start: "2025-02-30" }).problems.map((p) => p.path)).toEqual([
      "data.start",
    ]);
    expect(validateSubmission(hiring, { ...valid, start: "2024-02-29" }).clean).toEqual({ ...valid, start: "2024-02-29" });
  });

  it("validates emails, urls, lengths and patterns", () => {
    const form: LogicForm = {
      fields: [
        { key: "email", type: "email" },
        { key: "site", type: "url" },
        { key: "code", type: "text", validation: { minLength: 2, maxLength: 4, pattern: "^[A-Z]+$" } },
      ],
    };
    const bad = validateSubmission(form, { email: "Ada <ada@example.com>", site: "ftp://x.example", code: "abc" });
    expect(bad.problems.map((p) => p.path)).toEqual(["data.email", "data.site", "data.code"]);
    expect(validateSubmission(form, { code: "ABCDE" }).problems.map((p) => p.path)).toEqual(["data.code"]);
    expect(validateSubmission(form, { email: "ada@example.com", site: "https://ada.dev/x", code: "AB" })).toEqual({
      clean: { email: "ada@example.com", site: "https://ada.dev/x", code: "AB" },
      problems: [],
    });
  });

  it("counts characters, not UTF-16 units, for length limits", () => {
    const form: LogicForm = { fields: [{ key: "emoji", type: "text", validation: { maxLength: 2 } }] };
    expect(validateSubmission(form, { emoji: "👍👍" }).problems).toEqual([]);
  });
});

describe("helpers", () => {
  it("isProvided", () => {
    expect([undefined, null, "", []].map(isProvided)).toEqual([false, false, false, false]);
    expect([0, false, "x", ["a"]].map(isProvided)).toEqual([true, true, true, true]);
  });

  it("problemsToErrors keeps the first message per field and routes other paths to _form", () => {
    expect(
      problemsToErrors([
        { path: "data.email", message: "first" },
        { path: "data.email", message: "second" },
        { path: "data", message: "too large" },
      ]),
    ).toEqual({ email: "first", [FORM_ERROR_KEY]: "too large" });
  });
});
