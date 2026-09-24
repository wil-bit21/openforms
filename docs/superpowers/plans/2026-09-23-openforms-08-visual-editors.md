# openforms Plan 08: Visual Editors Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Tasks marked with a **Parallel group** may be dispatched to concurrent subagents (superpowers:dispatching-parallel-agents), one git worktree per lane (superpowers:using-git-worktrees).

**Goal:** Add a visual form editor, a visual workflow editor (with a React Flow state diagram) and a version history with a side-by-side diff and restore to the admin app. All saves go through the existing definitions API.

**Architecture:** Everything lives under `web/apps/admin/src/editors/`. Each editor is a **pure reducer** (`formReducer.ts`, `workflowReducer.ts`), unit-tested without a DOM, and thin React panels dispatch actions to it. Shared modules cover:
- YAML round-tripping (`yaml`)
- client-side validation: ajv against the repo's `schemas/*.schema.json`, plus a TS port of the semantic rules in spec §5.3, with problems keyed by Go-style paths such as `fields[2].showIf.field`
- a save hook that maps server 422 `details` onto the same paths.

All SDK calls go through one adapter file (`editors/api.ts`), so the editors depend on Plan 06's SDK shapes in exactly one place. Plan 07's extension points are used unchanged: `routes.tsx` gets four entries and `extensions/OverviewExtras.tsx` gets a new body.

**Tech Stack:** React 18, TypeScript 5, Vite 6, TanStack Query 5, react-router-dom 6, `@xyflow/react` + `@dagrejs/dagre`, `ajv` (2020-12 dialect), `yaml`, `diff`, `@openforms/sdk` and `@openforms/react` (workspace), Vitest, @testing-library/react + user-event, MSW 2.

**Spec:** `docs/superpowers/specs/2026-09-23-openforms-design.md`. It binds §5 (definition semantics), §5.5 (versioning and `source`), §7.2 (PUT/versions endpoints), §9.1 (SDK names), §9.2 (`<OpenForm definition>`) and §9.5 (form editor, workflow editor, version history). The roadmap is `docs/superpowers/plans/2026-09-23-openforms-00-roadmap.md`: this plan is Wave 4, Lane A.

## Global Constraints

- Node 22 LTS, pnpm 9, TypeScript 5, React 18, Vite 6, Vitest, @testing-library/react, MSW 2. Run all commands from the repo root in Git Bash.
- New runtime deps for `web/apps/admin` only: `@xyflow/react`, `@dagrejs/dagre`, `ajv`, `yaml`, `diff`, `@openforms/react` (`workspace:*`). Add no others.
- Code lives in `web/apps/admin/src/editors/**`. The only edits outside it are: `src/routes.tsx` (4 appended entries), the body of `src/extensions/OverviewExtras.tsx`, and `vite.config.ts` / `tsconfig` / `package.json` of `web/apps/admin`.
- Saves use `PUT /api/v1/forms/{slug}?source=ui` and `PUT /api/v1/workflows/{slug}?source=ui` (spec §7.2) through the SDK's `putForm` / `putWorkflow`. Restore does the same with the old definition.
- The Go server is authoritative. Client validation is for UX only. Server 422 `details` are always shown, even when their path matches no control.
- Problem paths use the Go format: `fields[2].showIf.field`, `transitions[0].from[1]`, `onSubmit[0].to`.
- The code-managed banner text is verbatim from spec §9.5: "This form is managed in code. Changes made here will be overwritten by the next `openforms push` unless you run `openforms pull`." The workflow editor uses the same sentence with "workflow".
- The banner shows when the loaded record's `source` is `"cli"`.
- The slug is editable only when creating. `forms/new` and `workflows/new` refuse to save if the slug already exists, because PUT is an upsert.
- Every `@openforms/sdk` call goes through `src/editors/api.ts`.
- Editor-owned TanStack Query keys start with `"editor"`. After a save, the editors also invalidate Plan 07's `qk.form(slug)`, `qk.formVersions(slug)`, `qk.forms` (and the workflow equivalents).
- No placeholder copy such as "Lorem ipsum".
- Commit after every task with a conventional prefix (`feat(admin):`, `test(admin):`, `chore(admin):`).

## Review Focus

1. **Creating a form or workflow whose slug already exists.** The user expects "already exists", not a silent overwrite of the live definition (PUT upserts). Pinned by `useDefinitionSave` tests (Task 4) and the "new form with a taken slug" / "new workflow with a taken slug" page tests (Tasks 7, 11).
2. **Renaming or deleting a field key, state key or workflow field.** The user expects `showIf`, `initial`, `from`/`to` and `requireFields` references to follow renames and be cleaned up on delete, without touching references to a *different* item that shares a duplicate key. Pinned by reducer tests (Tasks 5, 8).
3. **Typing invalid YAML in the YAML tab.** The user expects an inline error while the visual editor keeps the last valid definition, with no crash and no data loss. Pinned by the `YamlPane` test (Task 3) and page tests (Tasks 7, 11). Non-mapping YAML and missing collections are normalised by the reducers (Tasks 5, 8).
4. **Previewing a semantically invalid form** (e.g. a dropdown with no options, or `showIf` pointing at a missing field). The user expects the editor to keep working and the preview to degrade gracefully. Pinned by the `FormPreview` / `PreviewBoundary` tests (Task 6).
5. **Server 422 problems whose `path` matches no control** (or whose path format differs slightly from the client port). The user expects them listed rather than dropped. Pinned by the "unmapped server problem" page tests (Tasks 7, 11).

---

## File Structure

```
web/apps/admin/
  package.json                      (modify: deps)
  vite.config.ts                    (modify: @schemas alias, fs.allow)
  tsconfig(.app).json               (modify: resolveJsonModule, @schemas paths)
  src/routes.tsx                    (modify: +4 editor routes)
  src/extensions/OverviewExtras.tsx (replace body: VersionHistory)
  src/editors/
    api.ts                          SDK adapter + editor query keys + useIsAdmin
    api.test.ts
    shared/
      types.ts                      editor-side definition types & constants
      objects.ts / objects.test.ts  applyPatch, nextNumbered, uniqueName, moveItem, clone, isObject…
      yaml.ts / yaml.test.ts        toYaml, parseYaml
      problems.ts / problems.test.ts
      validation.ts / validation.test.ts
      controls.tsx                  TextControl, NumberControl, CheckboxControl, SelectControl, FieldErrors
      OptionsEditor.tsx, TagInput.tsx, ProblemList.tsx, CodeManagedBanner.tsx, YamlPane.tsx
      useUnsavedChangesGuard.ts
      shared-components.test.tsx
      useDefinitionSave.ts / useDefinitionSave.test.tsx
      EditorLayout.tsx
      editors.css
    form/
      formReducer.ts / formReducer.test.ts
      FieldList.tsx, FieldInspector.tsx, ShowIfBuilder.tsx, ValidationEditor.tsx, FormMetaPanel.tsx, FormPreview.tsx
      form-components.test.tsx, FormPreview.test.tsx
      FormEditorPage.tsx / FormEditorPage.test.tsx
      form-editor.css
    workflow/
      workflowReducer.ts / workflowReducer.test.ts
      layout.ts / layout.test.ts
      WorkflowDiagram.tsx / WorkflowDiagram.test.tsx
      ActionsEditor.tsx, StateInspector.tsx, TransitionInspector.tsx, WorkflowFieldInspector.tsx,
      WorkflowMetaPanel.tsx, WorkflowOutline.tsx, workflow-components.test.tsx
      WorkflowEditorPage.tsx / WorkflowEditorPage.test.tsx
      workflow-editor.css
    history/
      lineDiff.ts / lineDiff.test.tsx   (sideBySide + DiffView tests)
      DiffView.tsx, VersionHistory.tsx / VersionHistory.test.tsx
      history.css
    test/
      samples.ts                    sampleForm(), sampleWorkflow()
      records.ts                    formRecordJson, workflowRecordJson, versionList, applyItem
      xyflowMock.tsx                light @xyflow/react stand-in for jsdom
  src/editors/routes.test.ts
```

## Execution order and lanes

| Order | Tasks | Notes |
|---|---|---|
| Sequential | 1 → 2 → 3 → 4 | Shared foundation. Everything below depends on it. |
| **Parallel group P8-A** (form lane) | 5 → 6 → 7 | Touches only `editors/form/**`. |
| **Parallel group P8-B** (workflow lane) | 8 → 9 → 10 → 11 | Touches only `editors/workflow/**` and `editors/test/xyflowMock.tsx`. |
| **Parallel group P8-C** (history lane) | 12 | Touches only `editors/history/**` and `src/extensions/OverviewExtras.tsx`. |
| Sequential | 13 | Route registration and full verification after all lanes merge. |

Within a lane, tasks run in order. The three lanes run concurrently.

## Assumptions about Plans 06/07 (verify in Task 1, Step 1)

Plan 08 relies on these names from the fixed contract:
- **Plan 07:** `src/routes.tsx` (`AdminRoute`, `routes`), `src/extensions/OverviewExtras.tsx`, `src/api.ts` (`client`), `src/queryKeys.ts` (`qk.me`, `qk.forms`, `qk.form(slug)`, `qk.formVersions(slug)`, `qk.workflows`, `qk.workflow(slug)`, `qk.workflowVersions(slug)`), `src/test/render.tsx` (`renderWithProviders(ui, { route?, path? })`), `src/test/server.ts` (`server`) and `src/test/fixtures.ts` (`makeFormRecord`, `makeWorkflowRecord`, `makePrincipal`, …).
- **Plan 06 SDK, assumed return values** (unwrapped from the §7.2 envelopes):
  - `getForm(slug) → FormRecord`
  - `putForm(slug, def, { source }) → ApplyItem`
  - `formVersions(slug) → VersionInfo[]`
  - `formVersion(slug, n) → FormRecord`
  - `listWorkflows() → WorkflowSummary[]`
  - the workflow equivalents
  - `me() → Principal`
  - errors are thrown as `OpenFormsError { status, code, message, details }`.
- **Plan 06 React:** `<OpenForm definition onSubmit />` renders a definition without a client, and calls `onSubmit(data)` instead of the API (the `useOpenForm({ definition, onSubmit })` mode of §9.2).

If any SDK name or shape differs, change only `src/editors/api.ts` (and `FormPreview.tsx` for the `OpenForm` prop). `api.test.ts` pins these shapes against real HTTP through MSW, so a mismatch fails there first.

---

### Task 1: Editor foundation: dependencies, schema alias, types, object utilities, YAML

**Files:**
- Modify: `web/apps/admin/package.json` (via pnpm)
- Modify: `web/apps/admin/vite.config.ts`
- Modify: `web/apps/admin/tsconfig.json` (or `tsconfig.app.json` if that is the one that includes `src`)
- Create: `web/apps/admin/src/editors/shared/types.ts`
- Create: `web/apps/admin/src/editors/shared/objects.ts`
- Create: `web/apps/admin/src/editors/shared/yaml.ts`
- Create: `web/apps/admin/src/editors/test/samples.ts`
- Test: `web/apps/admin/src/editors/shared/objects.test.ts`, `web/apps/admin/src/editors/shared/yaml.test.ts`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces (`shared/types.ts`):
  - Types: `FieldType`, `Option`, `Validation`, `Condition`, `Field`, `FormSettings`, `FormDef`, `StateColor`, `State`, `WorkflowField`, `Guard`, `ActionType`, `Action`, `Transition`, `WorkflowDef`, `Problem`, `Source`, `VersionSummary`, `DefinitionKind`.
  - Constants: `FIELD_TYPES`, `FIELD_TYPE_LABELS`, `WORKFLOW_FIELD_TYPES`, `STRING_FIELD_TYPES`, `OPTION_FIELD_TYPES`, `STATE_COLORS`, `ACTION_TYPES`, `ACTION_LABELS`, `SOURCE_LABELS`.
- Produces (`shared/objects.ts`):
  - `applyPatch<T>(target, patch, { dropEmptyStrings? = true }): T`
  - `nextNumbered(prefix, taken): string` (returns `prefix1`, `prefix2`, … whichever is free first)
  - `uniqueName(base, taken): string` (returns `base`, `base2`, `base3`, …)
  - `moveItem<T>(items, from, to): T[]`
  - `clone<T>(v): T`
  - `isObject(v): v is Record<string, unknown>`
  - `str(v): string`
  - `objects<T>(v): T[]`
  - `unique<T>(items): T[]`
- Produces (`shared/yaml.ts`):
  - `toYaml(value): string`
  - `parseYaml<T>(text): ParseResult<T>`, where `ParseResult<T> = { ok: true; value: T } | { ok: false; error: string }`
- Produces (`test/samples.ts`): `sampleForm(): FormDef`, `sampleWorkflow(): WorkflowDef`.

- [ ] **Step 1: Verify the Plan 06/07 contract exists**

Run:
```bash
ls web/apps/admin/src/routes.tsx web/apps/admin/src/extensions/OverviewExtras.tsx web/apps/admin/src/api.ts web/apps/admin/src/queryKeys.ts web/apps/admin/src/test/render.tsx web/apps/admin/src/test/server.ts web/apps/admin/src/test/fixtures.ts
grep -n "putForm\|formVersions\|formVersion\|putWorkflow\|workflowVersions\|workflowVersion\|listWorkflows\|getForm\|getWorkflow\|me(" web/packages/sdk/src/client.ts
grep -n "definition\|onSubmit" web/packages/react/src/OpenForm.tsx
```
Expected: all seven files are listed and every SDK method name appears. If a name differs, note it now; Task 4's `api.ts` is the only place that needs adjusting.

- [ ] **Step 2: Install dependencies**

Run:
```bash
pnpm -C web/apps/admin add @xyflow/react @dagrejs/dagre ajv yaml diff "@openforms/react@workspace:*"
```
Expected: `dependencies` in `web/apps/admin/package.json` now lists all six packages, and `pnpm-lock.yaml` is updated.

- [ ] **Step 3: Add the `@schemas` alias so the admin app can import `schemas/*.schema.json` from the repo root**

In `web/apps/admin/vite.config.ts`, add these imports at the top of the file:
```ts
import { fileURLToPath } from "node:url";

const repoRoot = fileURLToPath(new URL("../../..", import.meta.url));
const schemasDir = fileURLToPath(new URL("../../../schemas", import.meta.url));
```
Then merge these keys into the existing `defineConfig({...})` object. Keep the existing `server.proxy` and `test` blocks. Only add `resolve.alias["@schemas"]` and `server.fs.allow`:
```ts
  resolve: {
    alias: {
      "@schemas": schemasDir,
    },
  },
  server: {
    // keep the existing proxy entries here
    fs: { allow: [repoRoot] },
  },
```
If a separate `vitest.config.ts` exists and does **not** `mergeConfig` the Vite config, add the same `resolve.alias` block there too.

In the tsconfig that includes `src` (`tsconfig.app.json` if present, otherwise `tsconfig.json`), add these to `compilerOptions`:
```json
    "resolveJsonModule": true,
    "paths": { "@schemas/*": ["../../../schemas/*"] }
```

- [ ] **Step 4: Write the editor types**

Create `web/apps/admin/src/editors/shared/types.ts`:
```ts
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
```

- [ ] **Step 5: Write the sample definitions used by every editor test**

Create `web/apps/admin/src/editors/test/samples.ts`:
```ts
import type { FormDef, WorkflowDef } from "../shared/types";

// Fresh objects on every call so tests can mutate freely.
export function sampleForm(): FormDef {
  return {
    slug: "job-application",
    title: "Job application",
    settings: { public: true },
    fields: [
      { key: "name", type: "text", label: "Full name", required: true, validation: { minLength: 2 } },
      { key: "email", type: "email", label: "Email", required: true },
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
}

export function sampleWorkflow(): WorkflowDef {
  return {
    slug: "hiring",
    title: "Hiring pipeline",
    initial: "new",
    states: [
      { key: "new", label: "New", color: "gray" },
      { key: "screening", label: "Screening", color: "blue" },
      { key: "hired", label: "Hired", color: "green", terminal: true },
      { key: "rejected", label: "Rejected", color: "red", terminal: true },
    ],
    fields: [
      { key: "score", type: "number", label: "Score" },
      { key: "rejectionReason", type: "textarea", label: "Rejection reason" },
    ],
    transitions: [
      { key: "screen", label: "Start screening", from: ["new"], to: "screening", guard: { roles: ["reviewer"] } },
      {
        key: "hire",
        label: "Hire",
        from: ["screening"],
        to: "hired",
        guard: { roles: ["hiring-manager"], requireFields: ["score"] },
        actions: [{ type: "webhook", url: "https://example.com/hooks/hired" }],
      },
      {
        key: "reject",
        label: "Reject",
        from: ["new", "screening"],
        to: "rejected",
        guard: { requireFields: ["rejectionReason"] },
        actions: [
          {
            type: "email",
            to: "{{submission.data.email}}",
            subject: "Your application",
            body: "{{submission.fields.rejectionReason}}",
          },
        ],
      },
    ],
  };
}
```

- [ ] **Step 6: Write the failing tests for object utilities and YAML**

Create `web/apps/admin/src/editors/shared/objects.test.ts`:
```ts
import { describe, expect, it } from "vitest";
import { applyPatch, clone, isObject, moveItem, nextNumbered, objects, str, unique, uniqueName } from "./objects";

describe("applyPatch", () => {
  it("sets values and deletes undefined and empty-string keys by default", () => {
    const out = applyPatch<{ a?: string; b?: string; c?: number; d?: boolean }>(
      { a: "x", b: "y", c: 1 },
      { a: "", b: undefined, c: 0, d: false },
    );
    expect(out).toEqual({ c: 0, d: false });
  });

  it("keeps empty strings when asked", () => {
    expect(applyPatch<{ a?: string }>({ a: "x" }, { a: "" }, { dropEmptyStrings: false })).toEqual({ a: "" });
  });

  it("never mutates the target", () => {
    const target = { a: "x" };
    applyPatch(target, { a: "y" });
    expect(target).toEqual({ a: "x" });
  });
});

describe("naming helpers", () => {
  it("nextNumbered returns the first free numbered name", () => {
    expect(nextNumbered("field", [])).toBe("field1");
    expect(nextNumbered("field", ["field1", "field2", "field4"])).toBe("field3");
  });

  it("uniqueName tries the bare name first", () => {
    expect(uniqueName("name_copy", ["name"])).toBe("name_copy");
    expect(uniqueName("name_copy", ["name_copy", "name_copy2"])).toBe("name_copy3");
  });
});

describe("collections", () => {
  it("moveItem returns a new array with the item moved", () => {
    const items = ["a", "b", "c"];
    expect(moveItem(items, 0, 2)).toEqual(["b", "c", "a"]);
    expect(items).toEqual(["a", "b", "c"]);
  });

  it("unique keeps first occurrences in order", () => {
    expect(unique(["b", "a", "b", "c", "a"])).toEqual(["b", "a", "c"]);
  });

  it("objects keeps only plain objects from arrays", () => {
    expect(objects([1, "x", null, { a: 1 }, [2]])).toEqual([{ a: 1 }]);
    expect(objects("nope")).toEqual([]);
  });

  it("str and isObject coerce safely", () => {
    expect(str(5)).toBe("");
    expect(str("a")).toBe("a");
    expect(isObject({})).toBe(true);
    expect(isObject([])).toBe(false);
    expect(isObject(null)).toBe(false);
  });

  it("clone is deep", () => {
    const a = { x: { y: 1 } };
    const b = clone(a);
    b.x.y = 2;
    expect(a.x.y).toBe(1);
  });
});
```

Create `web/apps/admin/src/editors/shared/yaml.test.ts`:
```ts
import { describe, expect, it } from "vitest";
import { parseYaml, toYaml } from "./yaml";
import { sampleForm, sampleWorkflow } from "../test/samples";

describe("toYaml / parseYaml", () => {
  it("round-trips a form definition", () => {
    const form = sampleForm();
    expect(parseYaml(toYaml(form))).toEqual({ ok: true, value: form });
  });

  it("round-trips a workflow definition including template braces", () => {
    const wf = sampleWorkflow();
    const text = toYaml(wf);
    expect(text).toContain("{{submission.data.email}}");
    expect(parseYaml(text)).toEqual({ ok: true, value: wf });
  });

  it("keeps the definition's key order", () => {
    const text = toYaml(sampleForm());
    expect(text.indexOf("slug:")).toBeLessThan(text.indexOf("title:"));
    expect(text.indexOf("title:")).toBeLessThan(text.indexOf("fields:"));
  });

  it("omits undefined properties", () => {
    expect(toYaml({ slug: "a", description: undefined })).toBe("slug: a\n");
  });

  it("reports syntax errors without throwing", () => {
    const result = parseYaml("fields: [\n");
    expect(result.ok).toBe(false);
    if (!result.ok) expect(result.error.length).toBeGreaterThan(0);
  });

  it("rejects documents that are not a top-level mapping", () => {
    const message = "The document must be a mapping (key: value pairs) at the top level.";
    expect(parseYaml("- a\n- b\n")).toEqual({ ok: false, error: message });
    expect(parseYaml("")).toEqual({ ok: false, error: message });
    expect(parseYaml("just text")).toEqual({ ok: false, error: message });
  });
});
```

- [ ] **Step 7: Run the tests to verify they fail**

Run: `pnpm -C web/apps/admin exec vitest run src/editors/shared/objects.test.ts src/editors/shared/yaml.test.ts`
Expected: FAIL with `Failed to resolve import "./objects"` and `Failed to resolve import "./yaml"`.

- [ ] **Step 8: Implement `objects.ts` and `yaml.ts`**

Create `web/apps/admin/src/editors/shared/objects.ts`:
```ts
export function applyPatch<T extends object>(
  target: T,
  patch: Partial<T>,
  opts: { dropEmptyStrings?: boolean } = {},
): T {
  const dropEmpty = opts.dropEmptyStrings ?? true;
  const out: Record<string, unknown> = { ...(target as Record<string, unknown>) };
  for (const [key, value] of Object.entries(patch)) {
    if (value === undefined || (dropEmpty && value === "")) delete out[key];
    else out[key] = value;
  }
  return out as T;
}

export function nextNumbered(prefix: string, taken: Iterable<string>): string {
  const set = new Set(taken);
  for (let n = 1; ; n++) {
    const candidate = `${prefix}${n}`;
    if (!set.has(candidate)) return candidate;
  }
}

export function uniqueName(base: string, taken: Iterable<string>): string {
  const set = new Set(taken);
  if (!set.has(base)) return base;
  for (let n = 2; ; n++) {
    const candidate = `${base}${n}`;
    if (!set.has(candidate)) return candidate;
  }
}

export function moveItem<T>(items: T[], from: number, to: number): T[] {
  const next = items.slice();
  const [item] = next.splice(from, 1);
  next.splice(to, 0, item);
  return next;
}

export function clone<T>(value: T): T {
  return structuredClone(value);
}

export function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

export function str(value: unknown): string {
  return typeof value === "string" ? value : "";
}

export function objects<T>(value: unknown): T[] {
  return Array.isArray(value) ? (value.filter(isObject) as T[]) : [];
}

export function unique<T>(items: T[]): T[] {
  return Array.from(new Set(items));
}
```

Create `web/apps/admin/src/editors/shared/yaml.ts`:
```ts
import { parse, stringify } from "yaml";

export type ParseResult<T> = { ok: true; value: T } | { ok: false; error: string };

const NOT_A_MAPPING = "The document must be a mapping (key: value pairs) at the top level.";

export function toYaml(value: unknown): string {
  // lineWidth 0 disables folding so long strings stay on one line and diffs stay line-stable.
  return stringify(value, { lineWidth: 0 });
}

export function parseYaml<T = Record<string, unknown>>(text: string): ParseResult<T> {
  let value: unknown;
  try {
    value = parse(text);
  } catch (err) {
    return { ok: false, error: err instanceof Error ? err.message : String(err) };
  }
  if (value === null || typeof value !== "object" || Array.isArray(value)) {
    return { ok: false, error: NOT_A_MAPPING };
  }
  return { ok: true, value: value as T };
}
```

- [ ] **Step 9: Run the tests to verify they pass**

Run: `pnpm -C web/apps/admin exec vitest run src/editors/shared/objects.test.ts src/editors/shared/yaml.test.ts`
Expected: PASS, `Test Files  2 passed`, `Tests  15 passed`.

- [ ] **Step 10: Commit**

```bash
git add web/apps/admin/package.json web/pnpm-lock.yaml web/apps/admin/vite.config.ts web/apps/admin/tsconfig*.json web/apps/admin/src/editors/shared/types.ts web/apps/admin/src/editors/shared/objects.ts web/apps/admin/src/editors/shared/objects.test.ts web/apps/admin/src/editors/shared/yaml.ts web/apps/admin/src/editors/shared/yaml.test.ts web/apps/admin/src/editors/test/samples.ts
git commit -m "chore(admin): editor foundation - deps, schema alias, types, yaml utils"
```
(If the lockfile lives at the repo root instead of `web/`, add `pnpm-lock.yaml` instead.)

---

### Task 2: Client-side validation (ajv + semantic rules + problem helpers)

**Files:**
- Create: `web/apps/admin/src/editors/shared/problems.ts`
- Create: `web/apps/admin/src/editors/shared/validation.ts`
- Test: `web/apps/admin/src/editors/shared/problems.test.ts`, `web/apps/admin/src/editors/shared/validation.test.ts`

**Interfaces:**
- Consumes: `types.ts`, `objects.ts` (Task 1), and `schemas/form.schema.json` / `schemas/workflow.schema.json` (Plan 02) via the `@schemas` alias.
- Produces (`problems.ts`):
  - `dedupeProblems(p: Problem[]): Problem[]`
  - `mergeProblems(...lists: Problem[][]): Problem[]`
  - `problemsAt(p, path): Problem[]` (exact match)
  - `problemsUnder(p, prefix): Problem[]` (the path itself, or any path under it via `.x` or `[n]`)
  - `indexFromPath(path, collection): number | null`
- Produces (`validation.ts`):
  - `validateForm(def: FormDef): Problem[]`
  - `validateWorkflow(def: WorkflowDef): Problem[]`
  - `formSemanticProblems(def): Problem[]`
  - `workflowSemanticProblems(def): Problem[]`
  - `ajvProblems(errors: ErrorObject[]): Problem[]`
  - `pointerToPath(pointer: string): string`

- [ ] **Step 1: Write the failing tests**

Create `web/apps/admin/src/editors/shared/problems.test.ts`:
```ts
import { describe, expect, it } from "vitest";
import { dedupeProblems, indexFromPath, mergeProblems, problemsAt, problemsUnder } from "./problems";

const problems = [
  { path: "fields[1]", message: "a" },
  { path: "fields[1].key", message: "b" },
  { path: "fields[10].key", message: "c" },
  { path: "fields[1].options[0].value", message: "d" },
  { path: "slug", message: "e" },
];

describe("problem helpers", () => {
  it("problemsAt matches exactly", () => {
    expect(problemsAt(problems, "fields[1].key")).toEqual([{ path: "fields[1].key", message: "b" }]);
  });

  it("problemsUnder matches the path and its descendants but not siblings with a longer index", () => {
    expect(problemsUnder(problems, "fields[1]").map((p) => p.message)).toEqual(["a", "b", "d"]);
  });

  it("indexFromPath extracts the collection index", () => {
    expect(indexFromPath("fields[10].key", "fields")).toBe(10);
    expect(indexFromPath("transitions[2].to", "fields")).toBeNull();
    expect(indexFromPath("slug", "fields")).toBeNull();
  });

  it("dedupe and merge remove identical problems", () => {
    expect(dedupeProblems([problems[4], problems[4]])).toEqual([problems[4]]);
    expect(mergeProblems([problems[0]], [problems[0], problems[4]])).toEqual([problems[0], problems[4]]);
  });
});
```

Create `web/apps/admin/src/editors/shared/validation.test.ts`:
```ts
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `pnpm -C web/apps/admin exec vitest run src/editors/shared/problems.test.ts src/editors/shared/validation.test.ts`
Expected: FAIL with `Failed to resolve import "./problems"` / `"./validation"`.

- [ ] **Step 3: Implement `problems.ts`**

Create `web/apps/admin/src/editors/shared/problems.ts`:
```ts
import type { Problem } from "./types";

export function dedupeProblems(problems: Problem[]): Problem[] {
  const seen = new Set<string>();
  return problems.filter((p) => {
    const key = `${p.path}\u0000${p.message}`;
    if (seen.has(key)) return false;
    seen.add(key);
    return true;
  });
}

export function mergeProblems(...lists: Problem[][]): Problem[] {
  return dedupeProblems(lists.flat());
}

export function problemsAt(problems: Problem[], path: string): Problem[] {
  return problems.filter((p) => p.path === path);
}

export function problemsUnder(problems: Problem[], prefix: string): Problem[] {
  return problems.filter(
    (p) => p.path === prefix || p.path.startsWith(`${prefix}.`) || p.path.startsWith(`${prefix}[`),
  );
}

export function indexFromPath(path: string, collection: string): number | null {
  const match = new RegExp(`^${collection}\\[(\\d+)\\]`).exec(path);
  return match ? Number(match[1]) : null;
}
```

- [ ] **Step 4: Implement `validation.ts`**

Create `web/apps/admin/src/editors/shared/validation.ts`:
```ts
import Ajv2020 from "ajv/dist/2020";
import type { ErrorObject } from "ajv";
import formSchema from "@schemas/form.schema.json";
import workflowSchema from "@schemas/workflow.schema.json";
import { dedupeProblems } from "./problems";
import {
  OPTION_FIELD_TYPES,
  STRING_FIELD_TYPES,
  WORKFLOW_FIELD_TYPES,
  type Action,
  type Field,
  type FormDef,
  type Problem,
  type State,
  type Transition,
  type WorkflowDef,
  type WorkflowField,
} from "./types";

// The Go server is authoritative (spec §2). This module mirrors spec §5.3 so the
// editors can show problems inline, using the same path format as the server.

const ajv = new Ajv2020({ allErrors: true, strict: false, validateFormats: false });
// Compile the form schema first so a workflow schema $ref to it resolves.
const validateFormSchema = ajv.compile(formSchema);
const validateWorkflowSchema = ajv.compile(workflowSchema);

const SLUG_RE = /^[a-z0-9][a-z0-9-]{0,62}$/;
const KEY_RE = /^[a-zA-Z][a-zA-Z0-9_]{0,63}$/;
const SLUG_MESSAGE = "must be 1-63 lowercase letters, digits or dashes, starting with a letter or digit";
const KEY_MESSAGE = "must start with a letter and contain only letters, digits and underscores (max 64)";

type Add = (path: string, message: string) => void;

function list<T>(value: unknown): T[] {
  return Array.isArray(value) ? (value as T[]) : [];
}

export function pointerToPath(pointer: string): string {
  if (!pointer) return "";
  return pointer
    .split("/")
    .slice(1)
    .map((seg) => seg.replace(/~1/g, "/").replace(/~0/g, "~"))
    .reduce((acc, seg) => (/^\d+$/.test(seg) ? `${acc}[${seg}]` : acc ? `${acc}.${seg}` : seg), "");
}

function joinPath(base: string, key: string): string {
  return base ? `${base}.${key}` : key;
}

export function ajvProblems(errors: ErrorObject[]): Problem[] {
  const out: Problem[] = [];
  for (const e of errors) {
    if (e.keyword === "if") continue; // always accompanied by the more specific "then" error
    const base = pointerToPath(e.instancePath);
    const params = e.params as Record<string, unknown>;
    if (e.keyword === "required") {
      out.push({ path: joinPath(base, String(params.missingProperty)), message: "is required" });
    } else if (e.keyword === "additionalProperties") {
      out.push({ path: joinPath(base, String(params.additionalProperty)), message: "is not a recognised property" });
    } else {
      out.push({ path: base, message: e.message ?? "is invalid" });
    }
  }
  return dedupeProblems(out);
}

function combine(schemaProblems: Problem[], semantic: Problem[]): Problem[] {
  // When the schema already complains about a path, the semantic message there is noise.
  const schemaPaths = new Set(schemaProblems.map((p) => p.path));
  return dedupeProblems([...schemaProblems, ...semantic.filter((p) => !schemaPaths.has(p.path))]);
}

export function validateForm(def: FormDef): Problem[] {
  const schema = validateFormSchema(def) ? [] : ajvProblems(validateFormSchema.errors ?? []);
  return combine(schema, formSemanticProblems(def));
}

export function validateWorkflow(def: WorkflowDef): Problem[] {
  const schema = validateWorkflowSchema(def) ? [] : ajvProblems(validateWorkflowSchema.errors ?? []);
  return combine(schema, workflowSemanticProblems(def));
}

export function formSemanticProblems(def: FormDef): Problem[] {
  const out: Problem[] = [];
  const add: Add = (path, message) => out.push({ path, message });
  if (!SLUG_RE.test(String(def?.slug ?? ""))) add("slug", SLUG_MESSAGE);

  const fields = list<Field>(def?.fields);
  const firstIndex = new Map<string, number>();
  fields.forEach((field, i) => {
    if (!field || typeof field !== "object") return;
    const base = `fields[${i}]`;
    const key = String(field.key ?? "");
    if (!KEY_RE.test(key)) add(`${base}.key`, KEY_MESSAGE);
    else if (firstIndex.has(key)) add(`${base}.key`, `duplicates the key of fields[${firstIndex.get(key)}]`);
    else firstIndex.set(key, i);

    const options = list<{ value: string }>(field.options);
    if (OPTION_FIELD_TYPES.has(field.type)) {
      if (options.length === 0) add(`${base}.options`, "needs at least one option");
      const seen = new Set<string>();
      options.forEach((o, j) => {
        if (seen.has(o?.value)) add(`${base}.options[${j}].value`, "duplicates another option value");
        seen.add(o?.value);
      });
    } else if (options.length > 0) {
      add(`${base}.options`, "only dropdown and multiple choice fields have options");
    }

    const v = field.validation;
    if (v && typeof v === "object") {
      const isString = STRING_FIELD_TYPES.has(field.type);
      (["minLength", "maxLength", "pattern"] as const).forEach((k) => {
        if (v[k] !== undefined && !isString) add(`${base}.validation.${k}`, "only applies to short text, long text, email and URL fields");
      });
      (["min", "max"] as const).forEach((k) => {
        if (v[k] !== undefined && field.type !== "number") add(`${base}.validation.${k}`, "only applies to number fields");
      });
      if (v.minLength !== undefined && v.maxLength !== undefined && v.minLength > v.maxLength) {
        add(`${base}.validation.minLength`, "must not be greater than maxLength");
      }
      if (v.min !== undefined && v.max !== undefined && v.min > v.max) {
        add(`${base}.validation.min`, "must not be greater than max");
      }
      if (v.pattern) {
        try {
          new RegExp(v.pattern);
        } catch {
          add(`${base}.validation.pattern`, "is not a valid regular expression");
        }
      }
    }

    const c = field.showIf;
    if (c && typeof c === "object") {
      const earlier = fields.slice(0, i).map((f) => f?.key);
      if (!earlier.includes(c.field)) add(`${base}.showIf.field`, "must refer to a field declared earlier in the form");
      const operators = [c.equals, c.notEquals, c.in].filter((x) => x !== undefined).length;
      if (operators !== 1) add(`${base}.showIf`, "needs exactly one of equals, notEquals or in");
    }
  });
  return out;
}

export function workflowSemanticProblems(def: WorkflowDef): Problem[] {
  const out: Problem[] = [];
  const add: Add = (path, message) => out.push({ path, message });
  if (!SLUG_RE.test(String(def?.slug ?? ""))) add("slug", SLUG_MESSAGE);

  const states = list<State>(def?.states);
  const stateIndex = new Map<string, number>();
  states.forEach((s, i) => {
    const key = String(s?.key ?? "");
    if (!KEY_RE.test(key)) add(`states[${i}].key`, KEY_MESSAGE);
    else if (stateIndex.has(key)) add(`states[${i}].key`, `duplicates the key of states[${stateIndex.get(key)}]`);
    else stateIndex.set(key, i);
  });
  const terminal = new Set(states.filter((s) => s?.terminal).map((s) => s.key));
  if (!stateIndex.has(def?.initial)) add("initial", "must be one of the declared states");

  const fieldKeys = new Map<string, number>();
  list<WorkflowField>(def?.fields).forEach((f, i) => {
    const key = String(f?.key ?? "");
    if (!KEY_RE.test(key)) add(`fields[${i}].key`, KEY_MESSAGE);
    else if (fieldKeys.has(key)) add(`fields[${i}].key`, `duplicates the key of fields[${fieldKeys.get(key)}]`);
    else fieldKeys.set(key, i);
    if (!WORKFLOW_FIELD_TYPES.includes(f?.type)) add(`fields[${i}].type`, "is not available for workflow fields");
    if (f?.type === "select" && list(f.options).length === 0) add(`fields[${i}].options`, "needs at least one option");
  });

  const transitionKeys = new Map<string, number>();
  list<Transition>(def?.transitions).forEach((t, i) => {
    const base = `transitions[${i}]`;
    const key = String(t?.key ?? "");
    if (!KEY_RE.test(key)) add(`${base}.key`, KEY_MESSAGE);
    else if (transitionKeys.has(key)) add(`${base}.key`, `duplicates the key of transitions[${transitionKeys.get(key)}]`);
    else transitionKeys.set(key, i);

    const from = list<string>(t?.from);
    if (from.length === 0) add(`${base}.from`, "needs at least one source state");
    from.forEach((k, j) => {
      if (!stateIndex.has(k)) add(`${base}.from[${j}]`, `refers to unknown state "${k}"`);
      else if (terminal.has(k)) add(`${base}.from[${j}]`, `cannot leave terminal state "${k}"`);
    });
    if (!stateIndex.has(t?.to)) add(`${base}.to`, `refers to unknown state "${t?.to ?? ""}"`);
    list<string>(t?.guard?.requireFields).forEach((k, j) => {
      if (!fieldKeys.has(k)) add(`${base}.guard.requireFields[${j}]`, `refers to unknown workflow field "${k}"`);
    });
    list<Action>(t?.actions).forEach((a, j) => actionProblems(a, `${base}.actions[${j}]`, add));
  });
  list<Action>(def?.onSubmit).forEach((a, j) => actionProblems(a, `onSubmit[${j}]`, add));
  return out;
}

function actionProblems(a: Action, base: string, add: Add): void {
  switch (a?.type) {
    case "webhook":
      if (!/^https?:\/\/[^\s/]+/i.test(a.url ?? "")) add(`${base}.url`, "must be an absolute http(s) URL");
      break;
    case "email":
      if (!a.to?.trim()) add(`${base}.to`, "is required");
      if (!a.subject?.trim()) add(`${base}.subject`, "is required");
      break;
    case "assign": {
      const count = (a.user ? 1 : 0) + (a.role ? 1 : 0);
      if (count !== 1) add(base, "needs exactly one of user or role");
      break;
    }
    default:
      add(`${base}.type`, "must be webhook, email or assign");
  }
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `pnpm -C web/apps/admin exec vitest run src/editors/shared/problems.test.ts src/editors/shared/validation.test.ts`
Expected: PASS, `Tests  17 passed`. If "accepts the sample form" or "accepts the sample workflow" fails, print the problems (`console.log(validateForm(sampleForm()))`). A schema path error there means the sample and Plan 02's schema disagree. Fix the **sample** only if it violates spec §5; otherwise report the schema bug to the Plan 02 owner.

- [ ] **Step 6: Commit**

```bash
git add web/apps/admin/src/editors/shared/problems.ts web/apps/admin/src/editors/shared/problems.test.ts web/apps/admin/src/editors/shared/validation.ts web/apps/admin/src/editors/shared/validation.test.ts
git commit -m "feat(admin): client-side definition validation with Go-style problem paths"
```

---

### Task 3: Shared editor controls, YAML pane, banner and unsaved-changes guard

**Files:**
- Create: `web/apps/admin/src/editors/shared/controls.tsx`
- Create: `web/apps/admin/src/editors/shared/OptionsEditor.tsx`
- Create: `web/apps/admin/src/editors/shared/TagInput.tsx`
- Create: `web/apps/admin/src/editors/shared/ProblemList.tsx`
- Create: `web/apps/admin/src/editors/shared/CodeManagedBanner.tsx`
- Create: `web/apps/admin/src/editors/shared/YamlPane.tsx`
- Create: `web/apps/admin/src/editors/shared/useUnsavedChangesGuard.ts`
- Create: `web/apps/admin/src/editors/shared/editors.css`
- Test: `web/apps/admin/src/editors/shared/shared-components.test.tsx`

**Interfaces:**
- Consumes: `types.ts`, `objects.ts`, `yaml.ts`, `problems.ts`.
- Produces:
  - `FieldErrors({ id?, problems })`
  - `TextControl({ label, value, onChange, problems?, hint?, multiline?, readOnly?, placeholder?, monospace? })`
  - `NumberControl({ label, value, onChange(n | undefined), problems?, hint?, min? })`
  - `CheckboxControl({ label, checked, onChange(bool), disabled?, hint? })`
  - `SelectControl({ label, value, options: {value,label}[], onChange(string), problems?, hint? })`
  - `OptionsEditor({ options, basePath, problems, onChange(next) })`
  - `TagInput({ label, values, onChange(next), placeholder?, hint? })`
  - `ProblemList({ problems, onSelect? })`, rendered as a region named "Problems"
  - `CodeManagedBanner({ kind, source? })`
  - `YamlPane({ value, onApply(next: Record<string, unknown>) })`
  - `useUnsavedChangesGuard(active: boolean)`
  - `UNSAVED_MESSAGE`

- [ ] **Step 1: Write the failing tests**

Create `web/apps/admin/src/editors/shared/shared-components.test.tsx`:
```tsx
import { useState } from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { CodeManagedBanner } from "./CodeManagedBanner";
import { OptionsEditor } from "./OptionsEditor";
import { ProblemList } from "./ProblemList";
import { TagInput } from "./TagInput";
import { TextControl } from "./controls";
import { YamlPane } from "./YamlPane";
import { UNSAVED_MESSAGE, useUnsavedChangesGuard } from "./useUnsavedChangesGuard";
import type { Option } from "./types";

afterEach(() => vi.restoreAllMocks());

describe("TextControl", () => {
  it("links errors to the input for assistive tech", () => {
    render(<TextControl label="Key" value="1x" onChange={() => {}} problems={[{ path: "fields[0].key", message: "bad key" }]} />);
    const input = screen.getByLabelText("Key");
    expect(input).toHaveAttribute("aria-invalid", "true");
    const describedBy = input.getAttribute("aria-describedby")!;
    expect(document.getElementById(describedBy)).toHaveTextContent("bad key");
  });
});

describe("OptionsEditor", () => {
  function Harness() {
    const [options, setOptions] = useState<Option[]>([{ value: "option1", label: "Option 1" }]);
    return (
      <>
        <OptionsEditor options={options} basePath="fields[0].options" problems={[{ path: "fields[0].options[0].value", message: "dup" }]} onChange={setOptions} />
        <output data-testid="options">{JSON.stringify(options)}</output>
      </>
    );
  }

  it("adds, edits and removes options and shows per-option problems", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    expect(screen.getByText("dup")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Add option" }));
    await user.clear(screen.getByLabelText("Option 2 label"));
    await user.type(screen.getByLabelText("Option 2 label"), "Second");
    await user.click(screen.getByRole("button", { name: "Remove option 1" }));
    expect(JSON.parse(screen.getByTestId("options").textContent!)).toEqual([{ value: "option2", label: "Second" }]);
  });
});

describe("TagInput", () => {
  function Harness() {
    const [values, setValues] = useState<string[]>(["reviewer"]);
    return (
      <>
        <TagInput label="Roles" values={values} onChange={setValues} />
        <output data-testid="values">{values.join("|")}</output>
      </>
    );
  }

  it("adds on Enter and comma, ignores duplicates, removes on click", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    const input = screen.getByLabelText("Roles");
    await user.type(input, "lead{Enter}");
    await user.type(input, "ops,reviewer,");
    expect(screen.getByTestId("values")).toHaveTextContent("reviewer|lead|ops");
    await user.click(screen.getByRole("button", { name: "Remove lead" }));
    expect(screen.getByTestId("values")).toHaveTextContent("reviewer|ops");
  });
});

describe("ProblemList", () => {
  it("renders nothing without problems and selectable items otherwise", async () => {
    const onSelect = vi.fn();
    const { rerender } = render(<ProblemList problems={[]} onSelect={onSelect} />);
    expect(screen.queryByRole("region", { name: "Problems" })).toBeNull();
    rerender(<ProblemList problems={[{ path: "fields[1].key", message: "bad" }, { path: "", message: "whole thing" }]} onSelect={onSelect} />);
    expect(screen.getByRole("heading", { name: "2 problems" })).toBeInTheDocument();
    await userEvent.setup().click(screen.getByRole("button", { name: /fields\[1\]\.key/ }));
    expect(onSelect).toHaveBeenCalledWith({ path: "fields[1].key", message: "bad" });
    expect(screen.getByText("(definition)")).toBeInTheDocument();
  });
});

describe("CodeManagedBanner", () => {
  it("shows only for CLI-sourced definitions with the spec wording", () => {
    const { rerender, container } = render(<CodeManagedBanner kind="form" source="ui" />);
    expect(container).toBeEmptyDOMElement();
    rerender(<CodeManagedBanner kind="form" source="cli" />);
    expect(screen.getByRole("note")).toHaveTextContent(
      "This form is managed in code. Changes made here will be overwritten by the next openforms push unless you run openforms pull.",
    );
    rerender(<CodeManagedBanner kind="workflow" source="cli" />);
    expect(screen.getByRole("note")).toHaveTextContent("This workflow is managed in code.");
  });
});

describe("YamlPane", () => {
  it("applies valid YAML and reports invalid YAML without applying it", () => {
    const onApply = vi.fn();
    render(<YamlPane value={{ slug: "a", title: "A" }} onApply={onApply} />);
    const textarea = screen.getByLabelText("YAML") as HTMLTextAreaElement;
    expect(textarea.value).toBe("slug: a\ntitle: A\n");

    fireEvent.change(textarea, { target: { value: "slug: a\ntitle: [\n" } });
    expect(screen.getByRole("alert")).toBeInTheDocument();
    expect(onApply).not.toHaveBeenCalled();
    expect(textarea.value).toBe("slug: a\ntitle: [\n");

    fireEvent.change(textarea, { target: { value: "slug: a\ntitle: B\n" } });
    expect(screen.queryByRole("alert")).toBeNull();
    expect(onApply).toHaveBeenCalledWith({ slug: "a", title: "B" });
  });
});

describe("useUnsavedChangesGuard", () => {
  function Guarded({ active }: { active: boolean }) {
    useUnsavedChangesGuard(active);
    return <a href="/elsewhere">Leave</a>;
  }

  it("asks before following links while active and blocks when declined", () => {
    const confirm = vi.spyOn(window, "confirm").mockReturnValue(false);
    render(<Guarded active />);
    const notCancelled = fireEvent.click(screen.getByText("Leave"));
    expect(confirm).toHaveBeenCalledWith(UNSAVED_MESSAGE);
    expect(notCancelled).toBe(false);
  });

  it("lets navigation through when confirmed", () => {
    vi.spyOn(window, "confirm").mockReturnValue(true);
    render(<Guarded active />);
    expect(fireEvent.click(screen.getByText("Leave"))).toBe(true);
  });

  it("does nothing while inactive", () => {
    const confirm = vi.spyOn(window, "confirm");
    render(<Guarded active={false} />);
    fireEvent.click(screen.getByText("Leave"));
    expect(confirm).not.toHaveBeenCalled();
  });

  it("cancels beforeunload while active", () => {
    render(<Guarded active />);
    const event = new Event("beforeunload", { cancelable: true });
    window.dispatchEvent(event);
    expect(event.defaultPrevented).toBe(true);
  });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `pnpm -C web/apps/admin exec vitest run src/editors/shared/shared-components.test.tsx`
Expected: FAIL with `Failed to resolve import "./CodeManagedBanner"`.

- [ ] **Step 3: Implement the controls**

Create `web/apps/admin/src/editors/shared/controls.tsx`:
```tsx
import { useId, type ChangeEvent, type ReactNode } from "react";
import type { Problem } from "./types";

export function FieldErrors({ id, problems }: { id?: string; problems: Problem[] }) {
  if (problems.length === 0) return null;
  return (
    <ul id={id} className="of-ed-errors">
      {problems.map((p, i) => (
        <li key={`${i}-${p.message}`}>{p.message}</li>
      ))}
    </ul>
  );
}

interface ControlBase {
  label: string;
  problems?: Problem[];
  hint?: ReactNode;
}

function useDescription(problems: Problem[], hint: ReactNode | undefined) {
  const id = useId();
  const hintId = `${id}-hint`;
  const errId = `${id}-err`;
  const describedBy = [hint ? hintId : "", problems.length ? errId : ""].filter(Boolean).join(" ") || undefined;
  return { id, hintId, errId, describedBy, invalid: problems.length > 0 ? true : undefined };
}

function ControlFooter(props: { hint?: ReactNode; hintId: string; errId: string; problems: Problem[] }) {
  return (
    <>
      {props.hint ? (
        <p id={props.hintId} className="of-ed-hint">
          {props.hint}
        </p>
      ) : null}
      <FieldErrors id={props.errId} problems={props.problems} />
    </>
  );
}

export function TextControl({
  label,
  value,
  onChange,
  problems = [],
  hint,
  multiline = false,
  readOnly = false,
  placeholder,
  monospace = false,
}: ControlBase & {
  value: string | undefined;
  onChange: (value: string) => void;
  multiline?: boolean;
  readOnly?: boolean;
  placeholder?: string;
  monospace?: boolean;
}) {
  const d = useDescription(problems, hint);
  const shared = {
    id: d.id,
    value: value ?? "",
    readOnly,
    placeholder,
    "aria-invalid": d.invalid,
    "aria-describedby": d.describedBy,
    className: monospace ? "of-ed-input of-ed-input--mono" : "of-ed-input",
  };
  return (
    <div className="of-ed-control">
      <label htmlFor={d.id}>{label}</label>
      {multiline ? (
        <textarea rows={4} {...shared} onChange={(e: ChangeEvent<HTMLTextAreaElement>) => onChange(e.target.value)} />
      ) : (
        <input type="text" {...shared} onChange={(e: ChangeEvent<HTMLInputElement>) => onChange(e.target.value)} />
      )}
      <ControlFooter hint={hint} hintId={d.hintId} errId={d.errId} problems={problems} />
    </div>
  );
}

export function NumberControl({
  label,
  value,
  onChange,
  problems = [],
  hint,
  min,
}: ControlBase & { value: number | undefined; onChange: (value: number | undefined) => void; min?: number }) {
  const d = useDescription(problems, hint);
  return (
    <div className="of-ed-control">
      <label htmlFor={d.id}>{label}</label>
      <input
        id={d.id}
        type="number"
        className="of-ed-input"
        min={min}
        value={value ?? ""}
        aria-invalid={d.invalid}
        aria-describedby={d.describedBy}
        onChange={(e) => {
          const raw = e.target.value;
          const n = Number(raw);
          onChange(raw === "" || !Number.isFinite(n) ? undefined : n);
        }}
      />
      <ControlFooter hint={hint} hintId={d.hintId} errId={d.errId} problems={problems} />
    </div>
  );
}

export function CheckboxControl({
  label,
  checked,
  onChange,
  disabled = false,
  hint,
}: { label: string; checked: boolean; onChange: (checked: boolean) => void; disabled?: boolean; hint?: ReactNode }) {
  const d = useDescription([], hint);
  return (
    <div className="of-ed-control of-ed-control--checkbox">
      <input
        id={d.id}
        type="checkbox"
        checked={checked}
        disabled={disabled}
        aria-describedby={d.describedBy}
        onChange={(e) => onChange(e.target.checked)}
      />
      <label htmlFor={d.id}>{label}</label>
      {hint ? (
        <p id={d.hintId} className="of-ed-hint">
          {hint}
        </p>
      ) : null}
    </div>
  );
}

export function SelectControl({
  label,
  value,
  options,
  onChange,
  problems = [],
  hint,
}: ControlBase & { value: string; options: { value: string; label: string }[]; onChange: (value: string) => void }) {
  const d = useDescription(problems, hint);
  return (
    <div className="of-ed-control">
      <label htmlFor={d.id}>{label}</label>
      <select
        id={d.id}
        className="of-ed-input"
        value={value}
        aria-invalid={d.invalid}
        aria-describedby={d.describedBy}
        onChange={(e) => onChange(e.target.value)}
      >
        {options.map((o) => (
          <option key={o.value} value={o.value}>
            {o.label}
          </option>
        ))}
      </select>
      <ControlFooter hint={hint} hintId={d.hintId} errId={d.errId} problems={problems} />
    </div>
  );
}
```

- [ ] **Step 4: Implement OptionsEditor, TagInput, ProblemList, CodeManagedBanner, YamlPane and the guard**

Create `web/apps/admin/src/editors/shared/OptionsEditor.tsx`:
```tsx
import { FieldErrors, TextControl } from "./controls";
import { nextNumbered } from "./objects";
import { problemsAt } from "./problems";
import type { Option, Problem } from "./types";

export function OptionsEditor({
  options,
  basePath,
  problems,
  onChange,
}: {
  options: Option[];
  basePath: string;
  problems: Problem[];
  onChange: (next: Option[]) => void;
}) {
  const update = (index: number, patch: Partial<Option>) =>
    onChange(options.map((o, i) => (i === index ? { ...o, ...patch } : o)));
  const add = () => {
    const value = nextNumbered("option", options.map((o) => o.value));
    onChange([...options, { value, label: `Option ${value.slice("option".length)}` }]);
  };
  return (
    <fieldset className="of-ed-fieldset">
      <legend>Options</legend>
      <FieldErrors problems={problemsAt(problems, basePath)} />
      {options.map((o, i) => (
        <div className="of-ed-option" key={i}>
          <TextControl
            label={`Option ${i + 1} value`}
            value={o.value}
            monospace
            onChange={(v) => update(i, { value: v })}
            problems={problemsAt(problems, `${basePath}[${i}].value`)}
          />
          <TextControl
            label={`Option ${i + 1} label`}
            value={o.label}
            onChange={(v) => update(i, { label: v })}
            problems={problemsAt(problems, `${basePath}[${i}].label`)}
          />
          <button
            type="button"
            className="of-ed-icon-btn"
            aria-label={`Remove option ${i + 1}`}
            onClick={() => onChange(options.filter((_, j) => j !== i))}
          >
            ✕
          </button>
        </div>
      ))}
      <button type="button" className="of-ed-btn" onClick={add}>
        Add option
      </button>
    </fieldset>
  );
}
```

Create `web/apps/admin/src/editors/shared/TagInput.tsx`:
```tsx
import { useId, useState, type ReactNode } from "react";

export function TagInput({
  label,
  values,
  onChange,
  placeholder,
  hint,
}: {
  label: string;
  values: string[];
  onChange: (next: string[]) => void;
  placeholder?: string;
  hint?: ReactNode;
}) {
  const id = useId();
  const [draft, setDraft] = useState("");

  const commit = (text: string) => {
    const parts = text.split(",").map((s) => s.trim()).filter(Boolean);
    setDraft("");
    if (parts.length === 0) return;
    const next = [...values];
    for (const part of parts) if (!next.includes(part)) next.push(part);
    if (next.length !== values.length) onChange(next);
  };

  return (
    <div className="of-ed-control">
      <label htmlFor={id}>{label}</label>
      <div className="of-ed-tags">
        <ul aria-label={`${label} values`}>
          {values.map((v) => (
            <li key={v} className="of-ed-tag">
              {v}
              <button type="button" aria-label={`Remove ${v}`} onClick={() => onChange(values.filter((x) => x !== v))}>
                ✕
              </button>
            </li>
          ))}
        </ul>
        <input
          id={id}
          className="of-ed-input"
          value={draft}
          placeholder={placeholder}
          aria-describedby={hint ? `${id}-hint` : undefined}
          onChange={(e) => {
            const next = e.target.value;
            if (next.includes(",")) commit(next);
            else setDraft(next);
          }}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              commit(draft);
            } else if (e.key === "Backspace" && draft === "" && values.length > 0) {
              onChange(values.slice(0, -1));
            }
          }}
          onBlur={() => commit(draft)}
        />
      </div>
      {hint ? (
        <p id={`${id}-hint`} className="of-ed-hint">
          {hint}
        </p>
      ) : null}
    </div>
  );
}
```

Create `web/apps/admin/src/editors/shared/ProblemList.tsx`:
```tsx
import type { Problem } from "./types";

export function ProblemList({ problems, onSelect }: { problems: Problem[]; onSelect?: (problem: Problem) => void }) {
  if (problems.length === 0) return null;
  return (
    <section className="of-ed-problems" aria-label="Problems">
      <h2>{problems.length === 1 ? "1 problem" : `${problems.length} problems`}</h2>
      <ul>
        {problems.map((p, i) => {
          const content = (
            <>
              <code>{p.path || "(definition)"}</code> {p.message}
            </>
          );
          return (
            <li key={`${p.path}-${i}`}>
              {onSelect ? (
                <button type="button" className="of-ed-link" onClick={() => onSelect(p)}>
                  {content}
                </button>
              ) : (
                content
              )}
            </li>
          );
        })}
      </ul>
    </section>
  );
}
```

Create `web/apps/admin/src/editors/shared/CodeManagedBanner.tsx`:
```tsx
import type { DefinitionKind, Source } from "./types";

export function CodeManagedBanner({ kind, source }: { kind: DefinitionKind; source?: Source }) {
  if (source !== "cli") return null;
  return (
    <div role="note" className="of-ed-banner">
      This {kind} is managed in code. Changes made here will be overwritten by the next <code>openforms push</code>{" "}
      unless you run <code>openforms pull</code>.
    </div>
  );
}
```

Create `web/apps/admin/src/editors/shared/YamlPane.tsx`:
```tsx
import { useId, useState } from "react";
import { parseYaml, toYaml } from "./yaml";

// Mounted fresh each time the YAML tab opens, so it always starts from the
// current definition. While the text is invalid, nothing is applied and the
// visual editor keeps the last valid definition.
export function YamlPane({ value, onApply }: { value: unknown; onApply: (next: Record<string, unknown>) => void }) {
  const id = useId();
  const [text, setText] = useState(() => toYaml(value));
  const [error, setError] = useState<string | null>(null);

  return (
    <div className="of-ed-yaml">
      <label htmlFor={id}>YAML</label>
      <textarea
        id={id}
        className="of-ed-input of-ed-input--mono"
        spellCheck={false}
        rows={30}
        value={text}
        aria-invalid={error ? true : undefined}
        aria-describedby={error ? `${id}-err` : `${id}-hint`}
        onChange={(e) => {
          const next = e.target.value;
          setText(next);
          const parsed = parseYaml<Record<string, unknown>>(next);
          if (parsed.ok) {
            setError(null);
            onApply(parsed.value);
          } else {
            setError(parsed.error);
          }
        }}
      />
      {error ? (
        <p id={`${id}-err`} role="alert" className="of-ed-errors">
          {error}
        </p>
      ) : (
        <p id={`${id}-hint`} className="of-ed-hint">
          Changes apply to the visual editor as you type.
        </p>
      )}
    </div>
  );
}
```

Create `web/apps/admin/src/editors/shared/useUnsavedChangesGuard.ts`:
```ts
import { useEffect } from "react";

export const UNSAVED_MESSAGE = "You have unsaved changes. Leave this page and discard them?";

// Works with any router type: intercepts link clicks in the capture phase
// (before React Router's Link handler) and the browser's beforeunload.
export function useUnsavedChangesGuard(active: boolean): void {
  useEffect(() => {
    if (!active) return;

    const onBeforeUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };

    const onClick = (event: MouseEvent) => {
      if (event.defaultPrevented || event.button !== 0) return;
      if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
      const target = event.target as Element | null;
      const anchor = target?.closest?.("a[href]") as HTMLAnchorElement | null;
      if (!anchor || anchor.target === "_blank" || anchor.hasAttribute("download")) return;
      if (!window.confirm(UNSAVED_MESSAGE)) {
        event.preventDefault();
        event.stopPropagation();
      }
    };

    window.addEventListener("beforeunload", onBeforeUnload);
    document.addEventListener("click", onClick, true);
    return () => {
      window.removeEventListener("beforeunload", onBeforeUnload);
      document.removeEventListener("click", onClick, true);
    };
  }, [active]);
}
```

Create `web/apps/admin/src/editors/shared/editors.css`:
```css
/* Editor styles. Uses the admin design tokens when present, with neutral fallbacks. */
.of-ed { display: flex; flex-direction: column; gap: 12px; min-width: 0; }
.of-ed-header { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; }
.of-ed-header h1 { font-size: 1.25rem; margin: 0; flex: 1 1 auto; }
.of-ed-dirty { font-size: 0.8125rem; color: var(--color-warning-text, #92400e); }
.of-ed-tabs { display: inline-flex; border: 1px solid var(--color-border, #d0d5dd); border-radius: 6px; overflow: hidden; }
.of-ed-tabs button { padding: 6px 12px; background: transparent; border: 0; color: inherit; font: inherit; cursor: pointer; }
.of-ed-tabs button[aria-selected="true"] { background: var(--color-accent-subtle, #e0e7ff); font-weight: 600; }
.of-ed-btn { padding: 6px 12px; border-radius: 6px; border: 1px solid var(--color-border, #d0d5dd); background: var(--color-surface, #fff); color: inherit; font: inherit; cursor: pointer; }
.of-ed-btn:disabled { opacity: 0.6; cursor: not-allowed; }
.of-ed-btn--primary { background: var(--color-accent, #4f46e5); border-color: var(--color-accent, #4f46e5); color: #fff; }
.of-ed-btn--danger { color: var(--color-danger, #d92d20); border-color: currentColor; }
.of-ed-icon-btn { border: 0; background: transparent; color: inherit; cursor: pointer; padding: 4px 6px; border-radius: 4px; }
.of-ed-icon-btn:hover { background: var(--color-hover, rgba(0, 0, 0, 0.06)); }
.of-ed-link { border: 0; background: none; padding: 0; color: inherit; font: inherit; text-align: left; cursor: pointer; }
.of-ed-link:hover { text-decoration: underline; }
.of-ed-banner { padding: 10px 12px; border-radius: 6px; background: var(--color-warning-bg, #fffaeb); border: 1px solid var(--color-warning-border, #fedf89); color: var(--color-warning-text, #93370d); }
.of-ed-alert { padding: 10px 12px; border-radius: 6px; background: var(--color-danger-bg, #fef3f2); border: 1px solid var(--color-danger-border, #fecdca); color: var(--color-danger, #b42318); margin: 0; }
.of-ed-problems { border: 1px solid var(--color-danger-border, #fecdca); border-radius: 6px; padding: 8px 12px; }
.of-ed-problems h2 { font-size: 0.875rem; margin: 0 0 4px; color: var(--color-danger, #b42318); }
.of-ed-problems ul { margin: 0; padding-left: 18px; font-size: 0.8125rem; }
.of-ed-problems code { font-family: var(--font-mono, ui-monospace, SFMono-Regular, Menlo, monospace); }
.of-ed-control { display: flex; flex-direction: column; gap: 4px; margin-bottom: 12px; }
.of-ed-control > label { font-size: 0.8125rem; font-weight: 600; }
.of-ed-control--checkbox { flex-direction: row; flex-wrap: wrap; align-items: center; gap: 8px; }
.of-ed-control--checkbox > label { font-weight: 400; }
.of-ed-control--checkbox > .of-ed-hint { flex-basis: 100%; }
.of-ed-input { padding: 6px 8px; border: 1px solid var(--color-border, #d0d5dd); border-radius: 6px; font: inherit; background: var(--color-surface, #fff); color: inherit; width: 100%; box-sizing: border-box; }
.of-ed-input--mono { font-family: var(--font-mono, ui-monospace, SFMono-Regular, Menlo, monospace); font-size: 0.8125rem; }
.of-ed-input[aria-invalid="true"] { border-color: var(--color-danger, #d92d20); }
.of-ed-input[readonly] { background: var(--color-muted-bg, #f2f4f7); }
.of-ed-hint { margin: 0; font-size: 0.75rem; color: var(--color-muted, #667085); }
.of-ed-errors { margin: 0; padding-left: 16px; font-size: 0.75rem; color: var(--color-danger, #b42318); }
.of-ed-fieldset { border: 1px solid var(--color-border, #d0d5dd); border-radius: 6px; padding: 8px 12px; margin: 0 0 12px; }
.of-ed-fieldset > legend { font-size: 0.8125rem; font-weight: 600; padding: 0 4px; }
.of-ed-option { display: grid; grid-template-columns: 1fr 1fr auto; gap: 8px; align-items: end; }
.of-ed-card { border: 1px solid var(--color-border, #d0d5dd); border-radius: 6px; padding: 8px; margin-bottom: 8px; }
.of-ed-card__head { display: flex; justify-content: space-between; align-items: center; margin-bottom: 6px; }
.of-ed-tags { display: flex; flex-wrap: wrap; gap: 6px; align-items: center; }
.of-ed-tags ul { display: contents; list-style: none; margin: 0; padding: 0; }
.of-ed-tag { display: inline-flex; align-items: center; gap: 4px; padding: 2px 4px 2px 8px; border-radius: 999px; background: var(--color-accent-subtle, #e0e7ff); font-size: 0.8125rem; }
.of-ed-tag button { border: 0; background: none; cursor: pointer; color: inherit; }
.of-ed-tags .of-ed-input { flex: 1 1 120px; width: auto; }
.of-ed-badge { display: inline-block; min-width: 18px; padding: 0 5px; border-radius: 999px; background: var(--color-danger, #d92d20); color: #fff; font-size: 0.6875rem; text-align: center; margin-left: 6px; }
.of-ed-list { display: flex; flex-direction: column; gap: 4px; }
.of-ed-list ol, .of-ed-list ul { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 2px; }
.of-ed-list li { display: flex; align-items: center; gap: 2px; }
.of-ed-list__item { flex: 1 1 auto; display: flex; flex-direction: column; align-items: flex-start; text-align: left; padding: 6px 8px; border-radius: 6px; border: 1px solid transparent; background: none; color: inherit; font: inherit; cursor: pointer; min-width: 0; }
.of-ed-list__item small { color: var(--color-muted, #667085); font-size: 0.75rem; }
.of-ed-list__item[aria-current="true"] { border-color: var(--color-accent, #4f46e5); background: var(--color-accent-subtle, #eef2ff); }
.of-ed-list__actions { display: flex; }
.of-ed-list h3 { font-size: 0.75rem; text-transform: uppercase; letter-spacing: 0.04em; color: var(--color-muted, #667085); margin: 12px 0 4px; }
.of-ed-add { display: flex; gap: 8px; align-items: end; margin-top: 8px; }
.of-ed-add .of-ed-control { margin-bottom: 0; flex: 1 1 auto; }
.of-ed-inspector h2 { font-size: 1rem; margin: 0 0 12px; }
.of-ed-yaml textarea { min-height: 60vh; }
.sr-only { position: absolute; width: 1px; height: 1px; padding: 0; margin: -1px; overflow: hidden; clip: rect(0, 0, 0, 0); white-space: nowrap; border: 0; }
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `pnpm -C web/apps/admin exec vitest run src/editors/shared/shared-components.test.tsx`
Expected: PASS, `Tests  11 passed`.

- [ ] **Step 6: Commit**

```bash
git add web/apps/admin/src/editors/shared/controls.tsx web/apps/admin/src/editors/shared/OptionsEditor.tsx web/apps/admin/src/editors/shared/TagInput.tsx web/apps/admin/src/editors/shared/ProblemList.tsx web/apps/admin/src/editors/shared/CodeManagedBanner.tsx web/apps/admin/src/editors/shared/YamlPane.tsx web/apps/admin/src/editors/shared/useUnsavedChangesGuard.ts web/apps/admin/src/editors/shared/editors.css web/apps/admin/src/editors/shared/shared-components.test.tsx
git commit -m "feat(admin): shared editor controls, YAML pane, code-managed banner, unsaved guard"
```

---

### Task 4: SDK adapter, save flow and editor layout

**Files:**
- Create: `web/apps/admin/src/editors/api.ts`
- Create: `web/apps/admin/src/editors/shared/useDefinitionSave.ts`
- Create: `web/apps/admin/src/editors/shared/EditorLayout.tsx`
- Create: `web/apps/admin/src/editors/test/records.ts`
- Test: `web/apps/admin/src/editors/api.test.ts`, `web/apps/admin/src/editors/shared/useDefinitionSave.test.tsx`

**Interfaces:**
- Consumes:
  - Plan 07: `client` (`src/api.ts`), `qk` (`src/queryKeys.ts`), MSW `server` (`src/test/server.ts`), `makeFormRecord`, `makeWorkflowRecord` (`src/test/fixtures.ts`).
  - Plan 06: `OpenFormsError`, `FormDefinition`, `WorkflowDefinition` from `@openforms/sdk`.
- Produces (`api.ts`):
  - `editorKeys.definition(kind, slug)`, `editorKeys.versions(kind, slug)`, `editorKeys.version(kind, slug, n)`, `editorKeys.workflowSlugs`
  - `LoadedDefinition<T> = { definition: T; source: Source; version: number }`
  - `loadForm(slug): Promise<LoadedDefinition<FormDef>>`
  - `loadWorkflow(slug): Promise<LoadedDefinition<WorkflowDef>>`
  - `saveDefinition(kind, def): Promise<{ version: number; changed: boolean }>`
  - `definitionExists(kind, slug): Promise<boolean>`
  - `listVersions(kind, slug): Promise<VersionSummary[]>`
  - `loadVersion(kind, slug, n): Promise<FormDef | WorkflowDef>`
  - `listWorkflowSlugs(): Promise<string[]>`
  - `useIsAdmin(): boolean`
  - `invalidateDefinition(qc, kind, slug): Promise<void>`
- Produces (`useDefinitionSave.ts`):
  - `useDefinitionSave(kind)` → `{ save(def, isNew, onSaved(result)), saving, serverProblems, error, clearServerProblems }`
  - `SlugTakenError`
- Produces (`EditorLayout.tsx`):
  - `EditorTab = "visual" | "yaml"`
  - `EditorLayout({ title, tab, onTabChange, onSave, saving, dirty, banner, problems, onSelectProblem, error, saveBlockedMessage, children })`
- Produces (`test/records.ts`):
  - `formRecordJson(def?, { source?, version? })`
  - `workflowRecordJson(def?, { source?, version? })`
  - `versionList(versions: number[], source?)`
  - `applyItem(kind, slug, version, changed?)`

- [ ] **Step 1: Write the test record helpers**

Create `web/apps/admin/src/editors/test/records.ts`:
```ts
import { makeFormRecord, makeWorkflowRecord } from "../../test/fixtures";
import type { DefinitionKind, FormDef, WorkflowDef } from "../shared/types";
import { sampleForm, sampleWorkflow } from "./samples";

// Records exactly as the API returns them (spec §7.2), built on Plan 07's factories.
export function formRecordJson(def: FormDef = sampleForm(), extra: { source?: string; version?: number } = {}) {
  return makeFormRecord({
    slug: def.slug,
    version: extra.version ?? 3,
    source: (extra.source ?? "ui") as never,
    definition: def as never,
  });
}

export function workflowRecordJson(def: WorkflowDef = sampleWorkflow(), extra: { source?: string; version?: number } = {}) {
  return makeWorkflowRecord({
    slug: def.slug,
    version: extra.version ?? 3,
    source: (extra.source ?? "ui") as never,
    definition: def as never,
  });
}

export function versionList(versions: number[], source = "ui") {
  return versions.map((v) => ({
    version: v,
    hash: `hash${v}`,
    source,
    createdBy: "admin@demo.local",
    createdAt: `2026-09-${String(10 + v).padStart(2, "0")}T10:00:00Z`,
  }));
}

export function applyItem(kind: DefinitionKind, slug: string, version: number, changed = true) {
  return { kind, slug, version, changed, created: false };
}
```

- [ ] **Step 2: Write the failing tests**

Create `web/apps/admin/src/editors/api.test.ts`:
```ts
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import { server } from "../test/server";
import { definitionExists, listVersions, listWorkflowSlugs, loadForm, loadVersion, loadWorkflow, saveDefinition } from "./api";
import { formRecordJson, versionList, workflowRecordJson } from "./test/records";
import { sampleForm, sampleWorkflow } from "./test/samples";

describe("editor SDK adapter", () => {
  it("loads a form record and unwraps definition, source and version", async () => {
    server.use(http.get("/api/v1/forms/job-application", () => HttpResponse.json({ form: formRecordJson(sampleForm(), { source: "cli", version: 7 }) })));
    await expect(loadForm("job-application")).resolves.toEqual({ definition: sampleForm(), source: "cli", version: 7 });
  });

  it("loads a workflow record", async () => {
    server.use(http.get("/api/v1/workflows/hiring", () => HttpResponse.json({ workflow: workflowRecordJson() })));
    const loaded = await loadWorkflow("hiring");
    expect(loaded.definition).toEqual(sampleWorkflow());
  });

  it("saves with source=ui and returns the new version", async () => {
    let url = "";
    let body: unknown;
    server.use(
      http.put("/api/v1/forms/:slug", async ({ request }) => {
        url = request.url;
        body = await request.json();
        return HttpResponse.json({ item: { kind: "form", slug: "job-application", version: 8, changed: true, created: false } });
      }),
    );
    await expect(saveDefinition("form", sampleForm())).resolves.toEqual({ version: 8, changed: true });
    expect(new URL(url).pathname).toBe("/api/v1/forms/job-application");
    expect(new URL(url).searchParams.get("source")).toBe("ui");
    expect(body).toEqual(sampleForm());
  });

  it("saves workflows to the workflow endpoint", async () => {
    let path = "";
    server.use(
      http.put("/api/v1/workflows/:slug", ({ request }) => {
        path = new URL(request.url).pathname;
        return HttpResponse.json({ item: { kind: "workflow", slug: "hiring", version: 2, changed: false, created: false } });
      }),
    );
    await expect(saveDefinition("workflow", sampleWorkflow())).resolves.toEqual({ version: 2, changed: false });
    expect(path).toBe("/api/v1/workflows/hiring");
  });

  it("definitionExists maps 404 to false, 200 to true and rethrows other errors", async () => {
    server.use(
      http.get("/api/v1/forms/missing", () => HttpResponse.json({ error: { code: "not_found", message: "definition not found" } }, { status: 404 })),
      http.get("/api/v1/forms/job-application", () => HttpResponse.json({ form: formRecordJson() })),
      http.get("/api/v1/forms/broken", () => HttpResponse.json({ error: { code: "internal", message: "boom" } }, { status: 500 })),
    );
    await expect(definitionExists("form", "missing")).resolves.toBe(false);
    await expect(definitionExists("form", "job-application")).resolves.toBe(true);
    await expect(definitionExists("form", "broken")).rejects.toBeTruthy();
  });

  it("lists versions and loads a specific version", async () => {
    server.use(
      http.get("/api/v1/forms/job-application/versions", () => HttpResponse.json({ items: versionList([2, 1], "cli") })),
      http.get("/api/v1/forms/job-application/versions/1", () =>
        HttpResponse.json({ form: formRecordJson({ ...sampleForm(), title: "Old" }, { version: 1 }) }),
      ),
    );
    await expect(listVersions("form", "job-application")).resolves.toEqual([
      { version: 2, source: "cli", createdBy: "admin@demo.local", createdAt: "2026-09-12T10:00:00Z" },
      { version: 1, source: "cli", createdBy: "admin@demo.local", createdAt: "2026-09-11T10:00:00Z" },
    ]);
    await expect(loadVersion("form", "job-application", 1)).resolves.toMatchObject({ title: "Old" });
  });

  it("lists workflow slugs sorted", async () => {
    server.use(
      http.get("/api/v1/workflows", () =>
        HttpResponse.json({
          items: [
            { slug: "triage", title: "Triage", version: 1, source: "ui", updatedAt: "2026-09-01T00:00:00Z", stateCount: 3 },
            { slug: "hiring", title: "Hiring", version: 2, source: "cli", updatedAt: "2026-09-01T00:00:00Z", stateCount: 5 },
          ],
        }),
      ),
    );
    await expect(listWorkflowSlugs()).resolves.toEqual(["hiring", "triage"]);
  });
});
```

Create `web/apps/admin/src/editors/shared/useDefinitionSave.test.tsx`:
```tsx
import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { describe, expect, it, vi } from "vitest";
import { server } from "../../test/server";
import { useDefinitionSave } from "./useDefinitionSave";
import { formRecordJson } from "../test/records";
import { sampleForm } from "../test/samples";

function makeWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return ({ children }: { children: ReactNode }) => <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
}

function countPuts(response: () => Response) {
  const calls = { count: 0 };
  server.use(
    http.put("/api/v1/forms/:slug", () => {
      calls.count++;
      return response();
    }),
  );
  return calls;
}

describe("useDefinitionSave", () => {
  it("saves and reports the new version", async () => {
    countPuts(() => HttpResponse.json({ item: { kind: "form", slug: "job-application", version: 4, changed: true, created: false } }));
    const onSaved = vi.fn();
    const { result } = renderHook(() => useDefinitionSave("form"), { wrapper: makeWrapper() });
    act(() => result.current.save(sampleForm(), false, onSaved));
    await waitFor(() => expect(onSaved).toHaveBeenCalledWith({ version: 4, changed: true }));
    expect(result.current.serverProblems).toEqual([]);
    expect(result.current.error).toBeNull();
  });

  it("exposes 422 details as server problems", async () => {
    const details = [{ path: "fields[0].label", message: "server says no" }];
    countPuts(() => HttpResponse.json({ error: { code: "validation_failed", message: "invalid", details } }, { status: 422 }));
    const { result } = renderHook(() => useDefinitionSave("form"), { wrapper: makeWrapper() });
    act(() => result.current.save(sampleForm(), false, () => {}));
    await waitFor(() => expect(result.current.serverProblems).toEqual(details));
    act(() => result.current.clearServerProblems());
    expect(result.current.serverProblems).toEqual([]);
  });

  it("keeps a 422 without details visible as a whole-definition problem", async () => {
    countPuts(() => HttpResponse.json({ error: { code: "validation_failed", message: "slug must match path" } }, { status: 422 }));
    const { result } = renderHook(() => useDefinitionSave("form"), { wrapper: makeWrapper() });
    act(() => result.current.save(sampleForm(), false, () => {}));
    await waitFor(() => expect(result.current.serverProblems).toHaveLength(1));
    expect(result.current.serverProblems[0].path).toBe("");
  });

  it("refuses to create a definition whose slug already exists", async () => {
    server.use(http.get("/api/v1/forms/job-application", () => HttpResponse.json({ form: formRecordJson() })));
    const puts = countPuts(() => HttpResponse.json({ item: {} }));
    const onSaved = vi.fn();
    const { result } = renderHook(() => useDefinitionSave("form"), { wrapper: makeWrapper() });
    act(() => result.current.save(sampleForm(), true, onSaved));
    await waitFor(() => expect(result.current.serverProblems).toHaveLength(1));
    expect(result.current.serverProblems[0]).toEqual({
      path: "slug",
      message: 'A form with the slug "job-application" already exists. Choose another slug.',
    });
    expect(puts.count).toBe(0);
    expect(onSaved).not.toHaveBeenCalled();
  });

  it("creates a new definition when the slug is free", async () => {
    server.use(http.get("/api/v1/forms/job-application", () => HttpResponse.json({ error: { code: "not_found", message: "nope" } }, { status: 404 })));
    const puts = countPuts(() => HttpResponse.json({ item: { kind: "form", slug: "job-application", version: 1, changed: true, created: true } }));
    const onSaved = vi.fn();
    const { result } = renderHook(() => useDefinitionSave("form"), { wrapper: makeWrapper() });
    act(() => result.current.save(sampleForm(), true, onSaved));
    await waitFor(() => expect(onSaved).toHaveBeenCalled());
    expect(puts.count).toBe(1);
  });

  it("reports other failures as an error message", async () => {
    countPuts(() => HttpResponse.json({ error: { code: "internal", message: "database unavailable" } }, { status: 500 }));
    const { result } = renderHook(() => useDefinitionSave("form"), { wrapper: makeWrapper() });
    act(() => result.current.save(sampleForm(), false, () => {}));
    await waitFor(() => expect(result.current.error).toBeTruthy());
    expect(result.current.serverProblems).toEqual([]);
  });
});
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `pnpm -C web/apps/admin exec vitest run src/editors/api.test.ts src/editors/shared/useDefinitionSave.test.tsx`
Expected: FAIL with `Failed to resolve import "./api"` / `"./useDefinitionSave"`.

- [ ] **Step 4: Implement the adapter**

Create `web/apps/admin/src/editors/api.ts`:
```ts
// The ONLY place the editors touch @openforms/sdk. If Plan 06's method names or
// return shapes differ from the assumptions below, adjust this file only.
import { OpenFormsError, type FormDefinition, type WorkflowDefinition } from "@openforms/sdk";
import { useQuery, type QueryClient } from "@tanstack/react-query";
import { client } from "../api";
import { qk } from "../queryKeys";
import type { DefinitionKind, FormDef, Source, VersionSummary, WorkflowDef } from "./shared/types";

export interface LoadedDefinition<T> {
  definition: T;
  source: Source;
  version: number;
}

export const editorKeys = {
  definition: (kind: DefinitionKind, slug: string) => ["editor", kind, slug] as const,
  versions: (kind: DefinitionKind, slug: string) => ["editor", kind, slug, "versions"] as const,
  version: (kind: DefinitionKind, slug: string, n: number) => ["editor", kind, slug, "version", n] as const,
  workflowSlugs: ["editor", "workflow-slugs"] as const,
};

export async function loadForm(slug: string): Promise<LoadedDefinition<FormDef>> {
  const record = await client.getForm(slug);
  return { definition: record.definition as unknown as FormDef, source: record.source as Source, version: record.version };
}

export async function loadWorkflow(slug: string): Promise<LoadedDefinition<WorkflowDef>> {
  const record = await client.getWorkflow(slug);
  return { definition: record.definition as unknown as WorkflowDef, source: record.source as Source, version: record.version };
}

export async function saveDefinition(
  kind: DefinitionKind,
  def: FormDef | WorkflowDef,
): Promise<{ version: number; changed: boolean }> {
  const item =
    kind === "form"
      ? await client.putForm(def.slug, def as unknown as FormDefinition, { source: "ui" })
      : await client.putWorkflow(def.slug, def as unknown as WorkflowDefinition, { source: "ui" });
  return { version: item.version, changed: item.changed };
}

export async function definitionExists(kind: DefinitionKind, slug: string): Promise<boolean> {
  try {
    if (kind === "form") await client.getForm(slug);
    else await client.getWorkflow(slug);
    return true;
  } catch (err) {
    if (err instanceof OpenFormsError && err.status === 404) return false;
    throw err;
  }
}

export async function listVersions(kind: DefinitionKind, slug: string): Promise<VersionSummary[]> {
  const items = kind === "form" ? await client.formVersions(slug) : await client.workflowVersions(slug);
  return items.map((v) => ({ version: v.version, source: v.source as Source, createdBy: v.createdBy, createdAt: v.createdAt }));
}

export async function loadVersion(kind: DefinitionKind, slug: string, version: number): Promise<FormDef | WorkflowDef> {
  const record = kind === "form" ? await client.formVersion(slug, version) : await client.workflowVersion(slug, version);
  return record.definition as unknown as FormDef | WorkflowDef;
}

export async function listWorkflowSlugs(): Promise<string[]> {
  const items = await client.listWorkflows();
  return items.map((w) => w.slug).sort();
}

export function useIsAdmin(): boolean {
  const { data } = useQuery({ queryKey: qk.me, queryFn: () => client.me() });
  return Boolean(data?.roles?.includes("admin"));
}

export async function invalidateDefinition(qc: QueryClient, kind: DefinitionKind, slug: string): Promise<void> {
  const planKeys =
    kind === "form"
      ? [qk.form(slug), qk.formVersions(slug), qk.forms]
      : [qk.workflow(slug), qk.workflowVersions(slug), qk.workflows];
  await Promise.all([
    qc.invalidateQueries({ queryKey: ["editor", kind, slug] }),
    qc.invalidateQueries({ queryKey: editorKeys.workflowSlugs }),
    ...planKeys.map((queryKey) => qc.invalidateQueries({ queryKey })),
  ]);
}
```

- [ ] **Step 5: Implement the save hook and layout**

Create `web/apps/admin/src/editors/shared/useDefinitionSave.ts`:
```ts
import { useCallback, useState } from "react";
import { OpenFormsError } from "@openforms/sdk";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { definitionExists, invalidateDefinition, saveDefinition } from "../api";
import type { DefinitionKind, FormDef, Problem, WorkflowDef } from "./types";

export class SlugTakenError extends Error {}

type SaveResult = { version: number; changed: boolean };

export function useDefinitionSave(kind: DefinitionKind) {
  const qc = useQueryClient();
  const [serverProblems, setServerProblems] = useState<Problem[]>([]);
  const [error, setError] = useState<string | null>(null);

  const { mutate, isPending } = useMutation({
    mutationFn: async ({ def, isNew }: { def: FormDef | WorkflowDef; isNew: boolean }): Promise<SaveResult> => {
      // PUT is an upsert, so "new" must not silently overwrite an existing definition.
      if (isNew && (await definitionExists(kind, def.slug))) {
        throw new SlugTakenError(`A ${kind} with the slug "${def.slug}" already exists. Choose another slug.`);
      }
      return saveDefinition(kind, def);
    },
    onMutate: () => {
      setServerProblems([]);
      setError(null);
    },
    onSuccess: (_result, { def }) => invalidateDefinition(qc, kind, def.slug),
    onError: (err) => {
      if (err instanceof SlugTakenError) {
        setServerProblems([{ path: "slug", message: err.message }]);
      } else if (err instanceof OpenFormsError && err.status === 422) {
        setServerProblems(err.details?.length ? err.details : [{ path: "", message: err.message }]);
      } else {
        setError(err instanceof Error ? err.message : String(err));
      }
    },
  });

  const save = useCallback(
    (def: FormDef | WorkflowDef, isNew: boolean, onSaved: (result: SaveResult) => void) =>
      mutate({ def, isNew }, { onSuccess: (result) => onSaved(result) }),
    [mutate],
  );
  const clearServerProblems = useCallback(() => setServerProblems([]), []);

  return { save, saving: isPending, serverProblems, error, clearServerProblems };
}
```

Create `web/apps/admin/src/editors/shared/EditorLayout.tsx`:
```tsx
import type { ReactNode } from "react";
import { ProblemList } from "./ProblemList";
import type { Problem } from "./types";
import "./editors.css";

export type EditorTab = "visual" | "yaml";

export function EditorLayout(props: {
  title: string;
  tab: EditorTab;
  onTabChange: (tab: EditorTab) => void;
  onSave: () => void;
  saving: boolean;
  dirty: boolean;
  banner?: ReactNode;
  problems: Problem[];
  onSelectProblem?: (problem: Problem) => void;
  error?: string | null;
  saveBlockedMessage?: string | null;
  children: ReactNode;
}) {
  return (
    <div className="of-ed">
      <header className="of-ed-header">
        <h1>{props.title}</h1>
        {props.dirty ? <span className="of-ed-dirty">Unsaved changes</span> : null}
        <div className="of-ed-tabs" role="tablist" aria-label="Editor mode">
          <button type="button" role="tab" aria-selected={props.tab === "visual"} onClick={() => props.onTabChange("visual")}>
            Visual
          </button>
          <button type="button" role="tab" aria-selected={props.tab === "yaml"} onClick={() => props.onTabChange("yaml")}>
            YAML
          </button>
        </div>
        <button type="button" className="of-ed-btn of-ed-btn--primary" onClick={props.onSave} disabled={props.saving}>
          {props.saving ? "Saving…" : "Save"}
        </button>
      </header>
      {props.banner}
      {props.error ? (
        <p role="alert" className="of-ed-alert">
          Could not save: {props.error}
        </p>
      ) : null}
      {props.saveBlockedMessage ? (
        <p role="alert" className="of-ed-alert">
          {props.saveBlockedMessage}
        </p>
      ) : null}
      <ProblemList problems={props.problems} onSelect={props.onSelectProblem} />
      <div className="of-ed-body">{props.children}</div>
    </div>
  );
}
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `pnpm -C web/apps/admin exec vitest run src/editors/api.test.ts src/editors/shared/useDefinitionSave.test.tsx`
Expected: PASS, `Tests  13 passed`. A failure in `api.test.ts` means the SDK shape differs from the assumptions. Fix `api.ts` only, then rerun.

- [ ] **Step 7: Type-check and commit**

Run: `pnpm -C web/apps/admin exec tsc --noEmit -p .`
(If the app uses `tsconfig.app.json`, run it with `-p tsconfig.app.json`.)
Expected: no output, exit code 0.

```bash
git add web/apps/admin/src/editors/api.ts web/apps/admin/src/editors/api.test.ts web/apps/admin/src/editors/shared/useDefinitionSave.ts web/apps/admin/src/editors/shared/useDefinitionSave.test.tsx web/apps/admin/src/editors/shared/EditorLayout.tsx web/apps/admin/src/editors/test/records.ts
git commit -m "feat(admin): editor SDK adapter, save flow with 422 mapping and slug guard"
```

---

### Task 5: Form editor reducer

**Parallel group:** P8-A (form lane, step 1 of 3). Runs concurrently with P8-B and P8-C.

**Files:**
- Create: `web/apps/admin/src/editors/form/formReducer.ts`
- Test: `web/apps/admin/src/editors/form/formReducer.test.ts`

**Interfaces:**
- Consumes: `types.ts`, `objects.ts`.
- Produces:
  - `FormEditorState = { def: FormDef; selected: number | null; dirty: boolean }` (`selected: null` means form settings)
  - `FieldPatch`
  - `FormEditorAction`: `setMeta`, `setSettings`, `addField`, `updateField`, `setFieldType`, `renameField`, `removeField`, `duplicateField`, `moveField`, `select`, `replace`, `markSaved`
  - `formEditorReducer(state, action)` (returns the same object for no-ops)
  - `initFormEditor(def)`
  - `newFormDefinition()`
  - `newField(type, takenKeys)`
  - `normalizeForm(input: unknown): FormDef`

- [ ] **Step 1: Write the failing tests**

Create `web/apps/admin/src/editors/form/formReducer.test.ts`:
```ts
import { describe, expect, it } from "vitest";
import { formEditorReducer as reduce, initFormEditor, newField, newFormDefinition, normalizeForm } from "./formReducer";
import { sampleForm } from "../test/samples";

const init = () => initFormEditor(sampleForm());
const keys = (s: ReturnType<typeof init>) => s.def.fields.map((f) => f.key);

describe("initFormEditor", () => {
  it("starts clean with form settings selected", () => {
    const s = init();
    expect(s).toEqual({ def: sampleForm(), selected: null, dirty: false });
  });

  it("does not share structure with its input", () => {
    const def = sampleForm();
    const s = initFormEditor(def);
    s.def.fields[0].label = "changed";
    expect(def.fields[0].label).toBe("Full name");
  });
});

describe("meta and settings", () => {
  it("sets slug and title even when empty", () => {
    const s = reduce(init(), { type: "setMeta", patch: { slug: "", title: "" } });
    expect(s.def.slug).toBe("");
    expect(s.def.title).toBe("");
    expect(s.dirty).toBe(true);
  });

  it("removes optional meta when cleared", () => {
    let s = reduce(init(), { type: "setMeta", patch: { description: "Hello", workflow: "hiring" } });
    expect(s.def).toMatchObject({ description: "Hello", workflow: "hiring" });
    s = reduce(s, { type: "setMeta", patch: { description: "", workflow: "" } });
    expect("description" in s.def).toBe(false);
    expect("workflow" in s.def).toBe(false);
  });

  it("updates settings and keeps public=false", () => {
    const s = reduce(init(), { type: "setSettings", patch: { public: false, submitLabel: "Send" } });
    expect(s.def.settings).toEqual({ public: false, submitLabel: "Send" });
  });
});

describe("fields", () => {
  it("adds a field with a unique key and selects it", () => {
    const s = reduce(init(), { type: "addField", fieldType: "text" });
    expect(s.def.fields[4]).toEqual({ key: "field1", type: "text", label: "Untitled question" });
    expect(s.selected).toBe(4);
    expect(s.dirty).toBe(true);
  });

  it("adds dropdowns with two starter options", () => {
    const s = reduce(init(), { type: "addField", fieldType: "multiselect" });
    expect(s.def.fields[4].options).toEqual([
      { value: "option1", label: "Option 1" },
      { value: "option2", label: "Option 2" },
    ]);
  });

  it("updates a field and drops cleared optional properties", () => {
    let s = reduce(init(), { type: "updateField", index: 0, patch: { help: "As on your passport" } });
    expect(s.def.fields[0].help).toBe("As on your passport");
    s = reduce(s, { type: "updateField", index: 0, patch: { help: "", required: false } });
    expect("help" in s.def.fields[0]).toBe(false);
    expect("required" in s.def.fields[0]).toBe(false);
  });

  it("ignores out-of-range indexes", () => {
    const s0 = init();
    expect(reduce(s0, { type: "updateField", index: 9, patch: { label: "x" } })).toBe(s0);
    expect(reduce(s0, { type: "removeField", index: -1 })).toBe(s0);
    expect(reduce(s0, { type: "renameField", index: 9, key: "x" })).toBe(s0);
  });

  it("changing to a dropdown adds options and drops validation that no longer applies", () => {
    const s = reduce(init(), { type: "setFieldType", index: 0, fieldType: "select" });
    expect(s.def.fields[0].type).toBe("select");
    expect(s.def.fields[0].options).toHaveLength(2);
    expect(s.def.fields[0].validation).toBeUndefined();
  });

  it("changing away from a dropdown removes options; string validation survives string types", () => {
    expect("options" in reduce(init(), { type: "setFieldType", index: 2, fieldType: "text" }).def.fields[2]).toBe(false);
    expect(reduce(init(), { type: "setFieldType", index: 0, fieldType: "textarea" }).def.fields[0].validation).toEqual({ minLength: 2 });
  });

  it("renaming a key updates conditions that reference it", () => {
    const s = reduce(init(), { type: "renameField", index: 2, key: "position" });
    expect(s.def.fields[2].key).toBe("position");
    expect(s.def.fields[3].showIf).toEqual({ field: "position", equals: "designer" });
  });

  it("renaming one of two duplicate keys leaves conditions alone", () => {
    let s = reduce(init(), { type: "renameField", index: 1, key: "role" });
    s = reduce(s, { type: "renameField", index: 1, key: "email" });
    expect(s.def.fields[3].showIf?.field).toBe("role");
  });

  it("removing a field drops conditions that depended on it", () => {
    const s = reduce(init(), { type: "removeField", index: 2 });
    expect(keys(s)).toEqual(["name", "email", "portfolio"]);
    expect(s.def.fields[2].showIf).toBeUndefined();
  });

  it("removing keeps the selection sensible", () => {
    let s = reduce(init(), { type: "select", index: 3 });
    s = reduce(s, { type: "removeField", index: 3 });
    expect(s.selected).toBe(2);
    s = reduce(s, { type: "removeField", index: 0 });
    expect(s.selected).toBe(1);
    const single = initFormEditor({ ...sampleForm(), fields: [sampleForm().fields[0]] });
    expect(reduce(reduce(single, { type: "select", index: 0 }), { type: "removeField", index: 0 }).selected).toBeNull();
  });

  it("duplicates below the original with a unique key and selects the copy", () => {
    const s = reduce(init(), { type: "duplicateField", index: 0 });
    expect(s.def.fields[1]).toEqual({ ...sampleForm().fields[0], key: "name_copy", label: "Full name (copy)" });
    expect(s.selected).toBe(1);
    expect(reduce(s, { type: "duplicateField", index: 0 }).def.fields[1].key).toBe("name_copy2");
  });

  it("moves fields and keeps the moved field selected", () => {
    let s = reduce(init(), { type: "select", index: 1 });
    s = reduce(s, { type: "moveField", index: 1, direction: 1 });
    expect(keys(s)).toEqual(["name", "role", "email", "portfolio"]);
    expect(s.selected).toBe(2);
  });

  it("moving past either end is a no-op", () => {
    const s0 = init();
    expect(reduce(s0, { type: "moveField", index: 0, direction: -1 })).toBe(s0);
    expect(reduce(s0, { type: "moveField", index: 3, direction: 1 })).toBe(s0);
  });
});

describe("replace and save", () => {
  it("replaces from YAML, normalising missing collections", () => {
    const s = reduce(init(), { type: "replace", def: { slug: "x", title: "X" } as never });
    expect(s.def).toEqual({ slug: "x", title: "X", settings: { public: false }, fields: [] });
    expect(s.dirty).toBe(true);
  });

  it("clears a selection that no longer exists after replace", () => {
    let s = reduce(init(), { type: "select", index: 3 });
    s = reduce(s, { type: "replace", def: { ...sampleForm(), fields: sampleForm().fields.slice(0, 2) } });
    expect(s.selected).toBeNull();
  });

  it("markSaved clears the dirty flag", () => {
    const s = reduce(reduce(init(), { type: "setMeta", patch: { title: "T" } }), { type: "markSaved" });
    expect(s.dirty).toBe(false);
  });
});

describe("helpers", () => {
  it("newFormDefinition is public, titled and empty", () => {
    expect(newFormDefinition()).toEqual({ slug: "", title: "Untitled form", settings: { public: true }, fields: [] });
  });

  it("newField picks the first free numbered key", () => {
    expect(newField("text", ["field1"]).key).toBe("field2");
  });

  it("normalizeForm drops non-object fields and repairs settings", () => {
    const def = normalizeForm({ slug: "a", title: "A", settings: "x", fields: [1, "x", { key: "a", type: "text", label: "A" }] });
    expect(def).toEqual({ slug: "a", title: "A", settings: { public: false }, fields: [{ key: "a", type: "text", label: "A" }] });
  });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `pnpm -C web/apps/admin exec vitest run src/editors/form/formReducer.test.ts`
Expected: FAIL with `Failed to resolve import "./formReducer"`.

- [ ] **Step 3: Implement the reducer**

Create `web/apps/admin/src/editors/form/formReducer.ts`:
```ts
import { applyPatch, clone, isObject, moveItem, nextNumbered, objects, str, uniqueName } from "../shared/objects";
import {
  OPTION_FIELD_TYPES,
  STRING_FIELD_TYPES,
  type Field,
  type FieldType,
  type FormDef,
  type FormSettings,
  type Validation,
} from "../shared/types";

export interface FormEditorState {
  def: FormDef;
  /** Index of the selected field, or null when form settings are shown. */
  selected: number | null;
  dirty: boolean;
}

export type FieldPatch = Partial<Pick<Field, "label" | "help" | "placeholder" | "required" | "options" | "validation" | "showIf">>;

export type FormEditorAction =
  | { type: "setMeta"; patch: Partial<Pick<FormDef, "slug" | "title" | "description" | "workflow">> }
  | { type: "setSettings"; patch: Partial<FormSettings> }
  | { type: "addField"; fieldType: FieldType }
  | { type: "updateField"; index: number; patch: FieldPatch }
  | { type: "setFieldType"; index: number; fieldType: FieldType }
  | { type: "renameField"; index: number; key: string }
  | { type: "removeField"; index: number }
  | { type: "duplicateField"; index: number }
  | { type: "moveField"; index: number; direction: -1 | 1 }
  | { type: "select"; index: number | null }
  | { type: "replace"; def: FormDef }
  | { type: "markSaved" };

const STARTER_OPTIONS = () => [
  { value: "option1", label: "Option 1" },
  { value: "option2", label: "Option 2" },
];

export function newFormDefinition(): FormDef {
  return { slug: "", title: "Untitled form", settings: { public: true }, fields: [] };
}

export function newField(type: FieldType, takenKeys: string[]): Field {
  const field: Field = { key: nextNumbered("field", takenKeys), type, label: "Untitled question" };
  if (OPTION_FIELD_TYPES.has(type)) field.options = STARTER_OPTIONS();
  return field;
}

export function normalizeForm(input: unknown): FormDef {
  const d = isObject(input) ? input : {};
  return {
    ...d,
    slug: str(d.slug),
    title: str(d.title),
    settings: isObject(d.settings) ? (d.settings as unknown as FormSettings) : { public: false },
    fields: objects<Field>(d.fields),
  } as FormDef;
}

export function initFormEditor(def: FormDef): FormEditorState {
  return { def: normalizeForm(clone(def)), selected: null, dirty: false };
}

const inRange = (state: FormEditorState, index: number) => index >= 0 && index < state.def.fields.length;
const countKey = (fields: Field[], key: string) => fields.filter((f) => f.key === key).length;

function withFields(state: FormEditorState, fields: Field[], selected = state.selected): FormEditorState {
  return { ...state, def: { ...state.def, fields }, selected, dirty: true };
}

function withoutShowIf(field: Field): Field {
  const next = { ...field };
  delete next.showIf;
  return next;
}

function fitValidation(validation: Validation | undefined, type: FieldType): Validation | undefined {
  if (!validation) return undefined;
  const next = { ...validation };
  if (!STRING_FIELD_TYPES.has(type)) {
    delete next.minLength;
    delete next.maxLength;
    delete next.pattern;
  }
  if (type !== "number") {
    delete next.min;
    delete next.max;
  }
  return Object.keys(next).length > 0 ? next : undefined;
}

export function formEditorReducer(state: FormEditorState, action: FormEditorAction): FormEditorState {
  const fields = state.def.fields;
  switch (action.type) {
    case "setMeta": {
      const { slug, title, ...optional } = action.patch;
      let def = applyPatch(state.def, optional);
      if (slug !== undefined) def = { ...def, slug };
      if (title !== undefined) def = { ...def, title };
      return { ...state, def, dirty: true };
    }
    case "setSettings":
      return { ...state, def: { ...state.def, settings: applyPatch(state.def.settings, action.patch) }, dirty: true };
    case "addField": {
      const next = [...fields, newField(action.fieldType, fields.map((f) => f.key))];
      return withFields(state, next, next.length - 1);
    }
    case "updateField": {
      if (!inRange(state, action.index)) return state;
      const updated = applyPatch(fields[action.index], action.patch);
      if (!updated.required) delete updated.required;
      return withFields(state, fields.map((f, i) => (i === action.index ? updated : f)));
    }
    case "setFieldType": {
      if (!inRange(state, action.index)) return state;
      const updated: Field = { ...fields[action.index], type: action.fieldType };
      if (OPTION_FIELD_TYPES.has(action.fieldType)) {
        if (!updated.options?.length) updated.options = STARTER_OPTIONS();
      } else {
        delete updated.options;
      }
      const validation = fitValidation(updated.validation, action.fieldType);
      if (validation) updated.validation = validation;
      else delete updated.validation;
      return withFields(state, fields.map((f, i) => (i === action.index ? updated : f)));
    }
    case "renameField": {
      if (!inRange(state, action.index)) return state;
      const oldKey = fields[action.index].key;
      const followReferences = countKey(fields, oldKey) === 1;
      const next = fields.map((f, i) => {
        if (i === action.index) return { ...f, key: action.key };
        if (followReferences && f.showIf?.field === oldKey) return { ...f, showIf: { ...f.showIf, field: action.key } };
        return f;
      });
      return withFields(state, next);
    }
    case "removeField": {
      if (!inRange(state, action.index)) return state;
      const removedKey = fields[action.index].key;
      let next = fields.filter((_, i) => i !== action.index);
      if (countKey(next, removedKey) === 0) {
        next = next.map((f) => (f.showIf?.field === removedKey ? withoutShowIf(f) : f));
      }
      let selected = state.selected;
      if (selected !== null) {
        if (selected === action.index) selected = next.length === 0 ? null : Math.min(action.index, next.length - 1);
        else if (selected > action.index) selected -= 1;
      }
      return withFields(state, next, selected);
    }
    case "duplicateField": {
      if (!inRange(state, action.index)) return state;
      const original = fields[action.index];
      const copy: Field = {
        ...clone(original),
        key: uniqueName(`${original.key}_copy`, fields.map((f) => f.key)),
        label: `${original.label} (copy)`,
      };
      const next = [...fields.slice(0, action.index + 1), copy, ...fields.slice(action.index + 1)];
      return withFields(state, next, action.index + 1);
    }
    case "moveField": {
      const target = action.index + action.direction;
      if (!inRange(state, action.index) || target < 0 || target >= fields.length) return state;
      let selected = state.selected;
      if (selected === action.index) selected = target;
      else if (selected === target) selected = action.index;
      return withFields(state, moveItem(fields, action.index, target), selected);
    }
    case "select":
      return { ...state, selected: action.index };
    case "replace": {
      const def = normalizeForm(clone(action.def));
      const selected = state.selected !== null && state.selected < def.fields.length ? state.selected : null;
      return { def, selected, dirty: true };
    }
    case "markSaved":
      return { ...state, dirty: false };
  }
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `pnpm -C web/apps/admin exec vitest run src/editors/form/formReducer.test.ts`
Expected: PASS, `Tests  23 passed`.

- [ ] **Step 5: Commit**

```bash
git add web/apps/admin/src/editors/form/formReducer.ts web/apps/admin/src/editors/form/formReducer.test.ts
git commit -m "feat(admin): form editor reducer with reference-aware rename/delete"
```

---

### Task 6: Form editor panels (field list, inspector, condition builder, settings, preview)

**Parallel group:** P8-A (form lane, step 2 of 3).

**Files:**
- Create: `web/apps/admin/src/editors/form/FieldList.tsx`
- Create: `web/apps/admin/src/editors/form/FieldInspector.tsx`
- Create: `web/apps/admin/src/editors/form/ShowIfBuilder.tsx`
- Create: `web/apps/admin/src/editors/form/ValidationEditor.tsx`
- Create: `web/apps/admin/src/editors/form/FormMetaPanel.tsx`
- Create: `web/apps/admin/src/editors/form/FormPreview.tsx`
- Create: `web/apps/admin/src/editors/form/form-editor.css`
- Test: `web/apps/admin/src/editors/form/form-components.test.tsx`, `web/apps/admin/src/editors/form/FormPreview.test.tsx`

**Interfaces:**
- Consumes: `formEditorReducer`, `FormEditorAction` (Task 5); the shared controls (Task 3); `problemsAt`, `problemsUnder` (Task 2); `OpenForm` from `@openforms/react` (Plan 06); `FormDefinition` from `@openforms/sdk`.
- Produces:
  - `FieldList({ fields, selected, problems, dispatch })`: a navigation landmark named "Fields"
  - `FieldInspector({ def, index, problems, dispatch })`: a region named "Field settings"
  - `ShowIfBuilder({ fields, index, problems, onChange(cond | undefined) })`
  - `conditionOperator(c)`, `defaultValueFor(field)`
  - `ValidationEditor({ field, base, problems, onChange })`
  - `FormMetaPanel({ def, isNew, workflowSlugs, problems, dispatch })`: a region named "Form settings"
  - `FormPreview({ def })`: a region named "Preview"
  - `PreviewBoundary({ resetKey, children })`

- [ ] **Step 1: Write the failing tests**

Create `web/apps/admin/src/editors/form/form-components.test.tsx`:
```tsx
import { useReducer } from "react";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { formEditorReducer, initFormEditor } from "./formReducer";
import { FieldList } from "./FieldList";
import { FieldInspector } from "./FieldInspector";
import { FormMetaPanel } from "./FormMetaPanel";
import { sampleForm } from "../test/samples";
import type { FormDef, Problem } from "../shared/types";

function Harness({ def = sampleForm(), problems = [], isNew = false }: { def?: FormDef; problems?: Problem[]; isNew?: boolean }) {
  const [state, dispatch] = useReducer(formEditorReducer, def, initFormEditor);
  return (
    <>
      <FieldList fields={state.def.fields} selected={state.selected} problems={problems} dispatch={dispatch} />
      {state.selected === null ? (
        <FormMetaPanel def={state.def} isNew={isNew} workflowSlugs={["hiring"]} problems={problems} dispatch={dispatch} />
      ) : (
        <FieldInspector def={state.def} index={state.selected} problems={problems} dispatch={dispatch} />
      )}
      <output data-testid="def">{JSON.stringify(state.def)}</output>
    </>
  );
}

const current = (): FormDef => JSON.parse(screen.getByTestId("def").textContent!);
const nav = () => screen.getByRole("navigation", { name: "Fields" });
const selectField = (user: ReturnType<typeof userEvent.setup>, label: string) =>
  user.click(within(nav()).getAllByRole("button", { name: new RegExp(`^${label}`) })[0]);
const inspector = () => screen.getByRole("region", { name: "Field settings" });

describe("FieldList", () => {
  it("adds a field of the chosen type and selects it", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await user.selectOptions(screen.getByLabelText("New field type"), "select");
    await user.click(screen.getByRole("button", { name: "Add field" }));
    expect(current().fields[4]).toMatchObject({ key: "field1", type: "select" });
    expect(within(inspector()).getByRole("heading", { name: "Untitled question" })).toBeInTheDocument();
  });

  it("reorders and deletes fields", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await user.click(screen.getByRole("button", { name: "Move Email down" }));
    expect(current().fields.map((f) => f.key)).toEqual(["name", "role", "email", "portfolio"]);
    await user.click(screen.getByRole("button", { name: "Delete Portfolio URL" }));
    expect(current().fields).toHaveLength(3);
  });

  it("marks fields that have problems", () => {
    render(<Harness problems={[{ path: "fields[0].label", message: "label is bad" }]} />);
    expect(within(nav()).getByLabelText("1 problem")).toBeInTheDocument();
  });
});

describe("FieldInspector", () => {
  it("shows problems next to the matching control", async () => {
    const user = userEvent.setup();
    render(<Harness problems={[{ path: "fields[0].label", message: "label is bad" }]} />);
    await selectField(user, "Full name");
    expect(within(inspector()).getByText("label is bad")).toBeInTheDocument();
    expect(within(inspector()).getByLabelText("Label")).toHaveAttribute("aria-invalid", "true");
  });

  it("renaming a key updates conditions that use it", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await selectField(user, "Role");
    const key = within(inspector()).getByLabelText("Key");
    await user.clear(key);
    await user.type(key, "position");
    expect(current().fields[3].showIf).toEqual({ field: "position", equals: "designer" });
  });

  it("changing type to dropdown adds options and removes text validation", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await selectField(user, "Full name");
    await user.selectOptions(within(inspector()).getByLabelText("Type"), "select");
    expect(current().fields[0].options).toHaveLength(2);
    expect(current().fields[0].validation).toBeUndefined();
    expect(within(inspector()).getByRole("group", { name: "Options" })).toBeInTheDocument();
  });
});

describe("ShowIfBuilder", () => {
  it("only offers fields declared earlier", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await selectField(user, "Email");
    await user.click(within(inspector()).getByLabelText("Only show this field when a previous answer matches"));
    const options = within(within(inspector()).getByLabelText("Previous field")).getAllByRole("option");
    expect(options.map((o) => (o as HTMLOptionElement).value)).toEqual(["name"]);
    expect(current().fields[1].showIf).toEqual({ field: "name", equals: "" });

    await selectField(user, "Portfolio URL");
    const portfolioOptions = within(within(inspector()).getByLabelText("Previous field")).getAllByRole("option");
    expect(portfolioOptions.map((o) => (o as HTMLOptionElement).value)).toEqual(["name", "email", "role"]);
  });

  it("uses the controlling field's options for values and supports 'is one of'", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await selectField(user, "Portfolio URL");
    expect(within(inspector()).getByLabelText("Value")).toHaveValue("designer");
    await user.selectOptions(within(inspector()).getByLabelText("Condition"), "in");
    expect(current().fields[3].showIf).toEqual({ field: "role", in: ["designer"] });
    await user.click(within(inspector()).getByLabelText("Engineer"));
    expect(current().fields[3].showIf).toEqual({ field: "role", in: ["designer", "engineer"] });
  });

  it("removes the condition when unchecked", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await selectField(user, "Portfolio URL");
    await user.click(within(inspector()).getByLabelText("Only show this field when a previous answer matches"));
    expect(current().fields[3].showIf).toBeUndefined();
  });

  it("explains that the first field cannot be conditional", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await selectField(user, "Full name");
    expect(within(inspector()).getByText("Conditional display is available for fields after the first one.")).toBeInTheDocument();
  });
});

describe("FormMetaPanel", () => {
  it("locks the slug for existing forms", () => {
    render(<Harness />);
    expect(screen.getByLabelText("Slug")).toHaveAttribute("readonly");
  });

  it("edits slug, workflow and publishing for new forms", async () => {
    const user = userEvent.setup();
    render(<Harness isNew def={{ slug: "", title: "Untitled form", settings: { public: true }, fields: [] }} />);
    await user.type(screen.getByLabelText("Slug"), "contact");
    await user.selectOptions(screen.getByLabelText("Workflow"), "hiring");
    await user.click(screen.getByLabelText("Public (anyone with the link can submit)"));
    expect(current()).toMatchObject({ slug: "contact", workflow: "hiring", settings: { public: false } });
  });
});
```

Create `web/apps/admin/src/editors/form/FormPreview.test.tsx`:
```tsx
import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { FormPreview, PreviewBoundary } from "./FormPreview";
import { sampleForm } from "../test/samples";
import type { FormDef } from "../shared/types";

afterEach(() => vi.restoreAllMocks());

describe("FormPreview", () => {
  it("renders the definition with the real renderer", async () => {
    render(<FormPreview def={sampleForm()} />);
    expect(await screen.findByLabelText(/Full name/)).toBeInTheDocument();
  });

  it("does not take the editor down for semantically invalid definitions", () => {
    vi.spyOn(console, "error").mockImplementation(() => {});
    const broken: FormDef = {
      ...sampleForm(),
      fields: [
        { key: "a", type: "select", label: "No options" },
        { key: "b", type: "text", label: "Ghost condition", showIf: { field: "missing", equals: "x" } },
      ],
    };
    render(<FormPreview def={broken} />);
    expect(screen.getByRole("region", { name: "Preview" })).toBeInTheDocument();
  });
});

describe("PreviewBoundary", () => {
  it("shows a fallback when the preview throws", () => {
    vi.spyOn(console, "error").mockImplementation(() => {});
    function Boom(): JSX.Element {
      throw new Error("kaboom");
    }
    render(
      <PreviewBoundary resetKey="a">
        <Boom />
      </PreviewBoundary>,
    );
    expect(screen.getByText(/Preview unavailable/)).toHaveTextContent("kaboom");
  });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `pnpm -C web/apps/admin exec vitest run src/editors/form/form-components.test.tsx src/editors/form/FormPreview.test.tsx`
Expected: FAIL with `Failed to resolve import "./FieldList"` / `"./FormPreview"`.

- [ ] **Step 3: Implement FieldList**

Create `web/apps/admin/src/editors/form/FieldList.tsx`:
```tsx
import { useState, type Dispatch } from "react";
import { SelectControl } from "../shared/controls";
import { problemsUnder } from "../shared/problems";
import { FIELD_TYPES, FIELD_TYPE_LABELS, type Field, type FieldType, type Problem } from "../shared/types";
import type { FormEditorAction } from "./formReducer";

function ProblemBadge({ count }: { count: number }) {
  if (count === 0) return null;
  return (
    <span className="of-ed-badge" aria-label={count === 1 ? "1 problem" : `${count} problems`}>
      {count}
    </span>
  );
}

export function FieldList({
  fields,
  selected,
  problems,
  dispatch,
}: {
  fields: Field[];
  selected: number | null;
  problems: Problem[];
  dispatch: Dispatch<FormEditorAction>;
}) {
  const [newType, setNewType] = useState<FieldType>("text");
  const settingsProblems = problems.filter((p) => !p.path.startsWith("fields")).length;

  return (
    <nav className="of-ed-list" aria-label="Fields">
      <button
        type="button"
        className="of-ed-list__item"
        aria-current={selected === null ? "true" : undefined}
        onClick={() => dispatch({ type: "select", index: null })}
      >
        <span>
          Form settings
          <ProblemBadge count={settingsProblems} />
        </span>
      </button>
      <h3>Questions</h3>
      {fields.length === 0 ? <p className="of-ed-hint">No questions yet. Add the first one below.</p> : null}
      <ol>
        {fields.map((field, i) => {
          const name = field.label || field.key || `Question ${i + 1}`;
          return (
            <li key={i}>
              <button
                type="button"
                className="of-ed-list__item"
                aria-current={selected === i ? "true" : undefined}
                onClick={() => dispatch({ type: "select", index: i })}
              >
                <span>
                  {name}
                  <ProblemBadge count={problemsUnder(problems, `fields[${i}]`).length} />
                </span>
                <small>
                  {field.key} · {FIELD_TYPE_LABELS[field.type] ?? field.type}
                  {field.required ? " · required" : ""}
                  {field.showIf ? " · conditional" : ""}
                </small>
              </button>
              <div className="of-ed-list__actions">
                <button type="button" className="of-ed-icon-btn" aria-label={`Move ${name} up`} disabled={i === 0} onClick={() => dispatch({ type: "moveField", index: i, direction: -1 })}>
                  ↑
                </button>
                <button type="button" className="of-ed-icon-btn" aria-label={`Move ${name} down`} disabled={i === fields.length - 1} onClick={() => dispatch({ type: "moveField", index: i, direction: 1 })}>
                  ↓
                </button>
                <button type="button" className="of-ed-icon-btn" aria-label={`Duplicate ${name}`} onClick={() => dispatch({ type: "duplicateField", index: i })}>
                  ⧉
                </button>
                <button type="button" className="of-ed-icon-btn" aria-label={`Delete ${name}`} onClick={() => dispatch({ type: "removeField", index: i })}>
                  ✕
                </button>
              </div>
            </li>
          );
        })}
      </ol>
      <div className="of-ed-add">
        <SelectControl
          label="New field type"
          value={newType}
          options={FIELD_TYPES.map((t) => ({ value: t, label: FIELD_TYPE_LABELS[t] }))}
          onChange={(v) => setNewType(v as FieldType)}
        />
        <button type="button" className="of-ed-btn" onClick={() => dispatch({ type: "addField", fieldType: newType })}>
          Add field
        </button>
      </div>
    </nav>
  );
}
```

- [ ] **Step 4: Implement ShowIfBuilder and ValidationEditor**

Create `web/apps/admin/src/editors/form/ShowIfBuilder.tsx`:
```tsx
import { CheckboxControl, FieldErrors, NumberControl, SelectControl, TextControl } from "../shared/controls";
import { problemsAt } from "../shared/problems";
import type { Condition, Field, Problem } from "../shared/types";

type Operator = "equals" | "notEquals" | "in";

const TOGGLE_LABEL = "Only show this field when a previous answer matches";

export function conditionOperator(c: Condition): Operator {
  if (c.in !== undefined) return "in";
  if (c.notEquals !== undefined) return "notEquals";
  return "equals";
}

export function defaultValueFor(field: Field | undefined): string | number | boolean {
  if (!field) return "";
  if (field.type === "select" || field.type === "multiselect") return field.options?.[0]?.value ?? "";
  if (field.type === "checkbox") return true;
  if (field.type === "number") return 0;
  return "";
}

function makeCondition(field: string, op: Operator, value: unknown): Condition {
  if (op === "in") return { field, in: Array.isArray(value) ? value : [value] };
  const scalar = Array.isArray(value) ? value[0] : value;
  return op === "equals" ? { field, equals: scalar } : { field, notEquals: scalar };
}

export function ShowIfBuilder({
  fields,
  index,
  problems,
  onChange,
}: {
  fields: Field[];
  index: number;
  problems: Problem[];
  onChange: (next: Condition | undefined) => void;
}) {
  const earlier = fields.slice(0, index);
  const cond = fields[index]?.showIf;
  const base = `fields[${index}].showIf`;

  if (earlier.length === 0) {
    return <p className="of-ed-hint">Conditional display is available for fields after the first one.</p>;
  }
  if (!cond) {
    const last = earlier[earlier.length - 1];
    return (
      <CheckboxControl
        label={TOGGLE_LABEL}
        checked={false}
        onChange={(on) => on && onChange(makeCondition(last.key, "equals", defaultValueFor(last)))}
      />
    );
  }

  const controller = earlier.find((f) => f.key === cond.field);
  const op = conditionOperator(cond);
  const value = cond[op];
  const controllerOptions = earlier.map((f) => ({ value: f.key, label: `${f.label || f.key} (${f.key})` }));
  if (!controller) controllerOptions.unshift({ value: cond.field, label: `${cond.field} (not an earlier field)` });

  return (
    <fieldset className="of-ed-fieldset">
      <legend>Conditional display</legend>
      <CheckboxControl label={TOGGLE_LABEL} checked onChange={(on) => !on && onChange(undefined)} />
      <FieldErrors problems={problemsAt(problems, base)} />
      <SelectControl
        label="Previous field"
        value={cond.field}
        options={controllerOptions}
        problems={problemsAt(problems, `${base}.field`)}
        onChange={(key) => onChange(makeCondition(key, op, defaultValueFor(earlier.find((f) => f.key === key))))}
      />
      <SelectControl
        label="Condition"
        value={op}
        options={[
          { value: "equals", label: "is" },
          { value: "notEquals", label: "is not" },
          { value: "in", label: "is one of" },
        ]}
        onChange={(next) => onChange(makeCondition(cond.field, next as Operator, value))}
      />
      <ConditionValue
        controller={controller}
        op={op}
        value={value}
        problems={problemsAt(problems, `${base}.${op}`)}
        onChange={(v) => onChange(makeCondition(cond.field, op, v))}
      />
    </fieldset>
  );
}

function ConditionValue({
  controller,
  op,
  value,
  problems,
  onChange,
}: {
  controller: Field | undefined;
  op: Operator;
  value: unknown;
  problems: Problem[];
  onChange: (value: unknown) => void;
}) {
  const options = controller?.options ?? [];
  if (options.length > 0) {
    if (op === "in") {
      const selected = Array.isArray(value) ? value : [];
      return (
        <fieldset className="of-ed-fieldset">
          <legend>Values</legend>
          {options.map((o) => (
            <CheckboxControl
              key={o.value}
              label={o.label}
              checked={selected.includes(o.value)}
              onChange={(on) => onChange(on ? [...selected, o.value] : selected.filter((v) => v !== o.value))}
            />
          ))}
          <FieldErrors problems={problems} />
        </fieldset>
      );
    }
    return (
      <SelectControl
        label="Value"
        value={String(value ?? "")}
        options={options.map((o) => ({ value: o.value, label: o.label }))}
        problems={problems}
        onChange={onChange}
      />
    );
  }
  if (controller?.type === "checkbox") {
    const scalar = Array.isArray(value) ? value[0] : value;
    return (
      <SelectControl
        label="Value"
        value={scalar === false ? "false" : "true"}
        options={[
          { value: "true", label: "Checked" },
          { value: "false", label: "Not checked" },
        ]}
        problems={problems}
        onChange={(v) => onChange(v === "true")}
      />
    );
  }
  if (controller?.type === "number") {
    const scalar = Array.isArray(value) ? value[0] : value;
    return (
      <NumberControl
        label="Value"
        value={typeof scalar === "number" ? scalar : undefined}
        problems={problems}
        onChange={(n) => onChange(n ?? 0)}
      />
    );
  }
  const text = Array.isArray(value) ? value.join(", ") : String(value ?? "");
  return (
    <TextControl
      label={op === "in" ? "Values (comma separated)" : "Value"}
      value={text}
      problems={problems}
      onChange={(t) => onChange(op === "in" ? t.split(",").map((s) => s.trim()).filter(Boolean) : t)}
    />
  );
}
```

Create `web/apps/admin/src/editors/form/ValidationEditor.tsx`:
```tsx
import { NumberControl, TextControl } from "../shared/controls";
import { applyPatch } from "../shared/objects";
import { problemsAt } from "../shared/problems";
import { STRING_FIELD_TYPES, type Field, type Problem, type Validation } from "../shared/types";

export function ValidationEditor({
  field,
  base,
  problems,
  onChange,
}: {
  field: Field;
  base: string;
  problems: Problem[];
  onChange: (next: Validation | undefined) => void;
}) {
  const v = field.validation ?? {};
  const set = (patch: Partial<Validation>) => {
    const next = applyPatch(v, patch);
    onChange(Object.keys(next).length > 0 ? next : undefined);
  };
  const at = (key: keyof Validation) => problemsAt(problems, `${base}.${key}`);

  if (STRING_FIELD_TYPES.has(field.type)) {
    return (
      <fieldset className="of-ed-fieldset">
        <legend>Validation</legend>
        <NumberControl label="Minimum length" min={0} value={v.minLength} problems={at("minLength")} onChange={(n) => set({ minLength: n })} />
        <NumberControl label="Maximum length" min={0} value={v.maxLength} problems={at("maxLength")} onChange={(n) => set({ maxLength: n })} />
        <TextControl
          label="Pattern (regular expression)"
          monospace
          value={v.pattern}
          problems={at("pattern")}
          hint="The server uses RE2 syntax: lookarounds and backreferences are not supported."
          onChange={(p) => set({ pattern: p })}
        />
      </fieldset>
    );
  }
  if (field.type === "number") {
    return (
      <fieldset className="of-ed-fieldset">
        <legend>Validation</legend>
        <NumberControl label="Minimum" value={v.min} problems={at("min")} onChange={(n) => set({ min: n })} />
        <NumberControl label="Maximum" value={v.max} problems={at("max")} onChange={(n) => set({ max: n })} />
      </fieldset>
    );
  }
  return null;
}
```

- [ ] **Step 5: Implement FieldInspector and FormMetaPanel**

Create `web/apps/admin/src/editors/form/FieldInspector.tsx`:
```tsx
import type { Dispatch } from "react";
import { CheckboxControl, SelectControl, TextControl } from "../shared/controls";
import { OptionsEditor } from "../shared/OptionsEditor";
import { problemsAt } from "../shared/problems";
import { FIELD_TYPES, FIELD_TYPE_LABELS, OPTION_FIELD_TYPES, type FieldType, type FormDef, type Problem } from "../shared/types";
import type { FieldPatch, FormEditorAction } from "./formReducer";
import { ShowIfBuilder } from "./ShowIfBuilder";
import { ValidationEditor } from "./ValidationEditor";

export function FieldInspector({
  def,
  index,
  problems,
  dispatch,
}: {
  def: FormDef;
  index: number;
  problems: Problem[];
  dispatch: Dispatch<FormEditorAction>;
}) {
  const field = def.fields[index];
  if (!field) return null;
  const base = `fields[${index}]`;
  const at = (prop: string) => problemsAt(problems, `${base}.${prop}`);
  const update = (patch: FieldPatch) => dispatch({ type: "updateField", index, patch });

  return (
    <section className="of-ed-inspector" aria-label="Field settings">
      <h2>{field.label || field.key || `Question ${index + 1}`}</h2>
      <TextControl label="Label" value={field.label} problems={at("label")} onChange={(v) => update({ label: v })} />
      <TextControl
        label="Key"
        value={field.key}
        monospace
        problems={at("key")}
        hint="Used in the API, exports and conditions. Renaming updates conditions that refer to it."
        onChange={(v) => dispatch({ type: "renameField", index, key: v })}
      />
      <SelectControl
        label="Type"
        value={field.type}
        options={FIELD_TYPES.map((t) => ({ value: t, label: FIELD_TYPE_LABELS[t] }))}
        problems={at("type")}
        onChange={(t) => dispatch({ type: "setFieldType", index, fieldType: t as FieldType })}
      />
      <CheckboxControl
        label={field.type === "checkbox" ? "Required (must be checked)" : "Required"}
        checked={Boolean(field.required)}
        onChange={(checked) => update({ required: checked })}
      />
      {field.type !== "checkbox" ? (
        <TextControl label="Placeholder" value={field.placeholder} problems={at("placeholder")} onChange={(v) => update({ placeholder: v })} />
      ) : null}
      <TextControl label="Help text" value={field.help} multiline problems={at("help")} onChange={(v) => update({ help: v })} />
      {OPTION_FIELD_TYPES.has(field.type) ? (
        <OptionsEditor options={field.options ?? []} basePath={`${base}.options`} problems={problems} onChange={(options) => update({ options })} />
      ) : null}
      <ValidationEditor field={field} base={`${base}.validation`} problems={problems} onChange={(validation) => update({ validation })} />
      <ShowIfBuilder fields={def.fields} index={index} problems={problems} onChange={(showIf) => update({ showIf })} />
    </section>
  );
}
```

Create `web/apps/admin/src/editors/form/FormMetaPanel.tsx`:
```tsx
import type { Dispatch } from "react";
import { CheckboxControl, SelectControl, TextControl } from "../shared/controls";
import { unique } from "../shared/objects";
import { problemsAt } from "../shared/problems";
import type { FormDef, Problem } from "../shared/types";
import type { FormEditorAction } from "./formReducer";

export function FormMetaPanel({
  def,
  isNew,
  workflowSlugs,
  problems,
  dispatch,
}: {
  def: FormDef;
  isNew: boolean;
  workflowSlugs: string[];
  problems: Problem[];
  dispatch: Dispatch<FormEditorAction>;
}) {
  const at = (path: string) => problemsAt(problems, path);
  const workflows = unique([...workflowSlugs, ...(def.workflow ? [def.workflow] : [])]);
  return (
    <section className="of-ed-inspector" aria-label="Form settings">
      <h2>Form settings</h2>
      <TextControl
        label="Slug"
        value={def.slug}
        monospace
        readOnly={!isNew}
        problems={at("slug")}
        hint={isNew ? "Lowercase letters, digits and dashes. Used in URLs like /f/<slug>. It can't be changed later." : "The slug can't be changed after the form is created."}
        onChange={(v) => dispatch({ type: "setMeta", patch: { slug: v } })}
      />
      <TextControl label="Title" value={def.title} problems={at("title")} onChange={(v) => dispatch({ type: "setMeta", patch: { title: v } })} />
      <TextControl
        label="Description"
        value={def.description}
        multiline
        problems={at("description")}
        onChange={(v) => dispatch({ type: "setMeta", patch: { description: v } })}
      />
      <SelectControl
        label="Workflow"
        value={def.workflow ?? ""}
        options={[{ value: "", label: "No workflow (submissions are simply received)" }, ...workflows.map((s) => ({ value: s, label: s }))]}
        problems={at("workflow")}
        onChange={(v) => dispatch({ type: "setMeta", patch: { workflow: v } })}
      />
      <CheckboxControl
        label="Public (anyone with the link can submit)"
        checked={Boolean(def.settings.public)}
        onChange={(checked) => dispatch({ type: "setSettings", patch: { public: checked } })}
      />
      <TextControl
        label="Submit button label"
        value={def.settings.submitLabel}
        placeholder="Submit"
        problems={at("settings.submitLabel")}
        onChange={(v) => dispatch({ type: "setSettings", patch: { submitLabel: v } })}
      />
      <TextControl
        label="Confirmation message"
        value={def.settings.confirmationMessage}
        multiline
        problems={at("settings.confirmationMessage")}
        onChange={(v) => dispatch({ type: "setSettings", patch: { confirmationMessage: v } })}
      />
    </section>
  );
}
```

- [ ] **Step 6: Implement FormPreview and the form editor CSS**

Create `web/apps/admin/src/editors/form/FormPreview.tsx`:
```tsx
import { Component, useState, type ReactNode } from "react";
import { OpenForm } from "@openforms/react";
import "@openforms/react/styles.css";
import type { FormDefinition } from "@openforms/sdk";
import type { FormDef } from "../shared/types";

export class PreviewBoundary extends Component<{ resetKey: string; children: ReactNode }, { error: Error | null }> {
  state: { error: Error | null } = { error: null };

  static getDerivedStateFromError(error: Error) {
    return { error };
  }

  componentDidUpdate(prev: { resetKey: string }) {
    if (prev.resetKey !== this.props.resetKey && this.state.error) this.setState({ error: null });
  }

  render() {
    if (this.state.error) {
      return (
        <p role="status" className="of-ed-hint">
          Preview unavailable until the problems above are fixed ({this.state.error.message}).
        </p>
      );
    }
    return this.props.children;
  }
}

export function FormPreview({ def }: { def: FormDef }) {
  const key = JSON.stringify(def);
  const [submitted, setSubmitted] = useState<Record<string, unknown> | null>(null);
  const [nonce, setNonce] = useState(0);

  return (
    <section className="of-fe-preview" aria-label="Preview">
      <h2>Preview</h2>
      <p className="of-ed-hint">Submitting here only shows the data that would be sent; nothing is saved.</p>
      <PreviewBoundary resetKey={key}>
        <OpenForm
          key={`${key}-${nonce}`}
          definition={def as unknown as FormDefinition}
          onSubmit={async (data: Record<string, unknown>) => {
            setSubmitted(data);
          }}
        />
      </PreviewBoundary>
      {submitted ? (
        <div className="of-fe-preview__result">
          <h3>Preview submission (not saved)</h3>
          <pre>{JSON.stringify(submitted, null, 2)}</pre>
          <button
            type="button"
            className="of-ed-btn"
            onClick={() => {
              setSubmitted(null);
              setNonce((n) => n + 1);
            }}
          >
            Reset preview
          </button>
        </div>
      ) : null}
    </section>
  );
}
```
(If Plan 06's `OpenForm` names the definition-mode submit callback differently, rename the `onSubmit` prop here. This is the only place that uses it.)

Create `web/apps/admin/src/editors/form/form-editor.css`:
```css
.of-fe-grid { display: grid; grid-template-columns: minmax(220px, 280px) minmax(300px, 1fr) minmax(300px, 1fr); gap: 16px; align-items: start; }
.of-fe-grid > * { min-width: 0; }
.of-fe-preview { border: 1px solid var(--color-border, #d0d5dd); border-radius: 8px; padding: 12px 16px; position: sticky; top: 12px; max-height: calc(100vh - 24px); overflow: auto; }
.of-fe-preview h2 { font-size: 1rem; margin: 0 0 4px; }
.of-fe-preview__result pre { font-size: 0.75rem; background: var(--color-muted-bg, #f2f4f7); padding: 8px; border-radius: 6px; overflow: auto; }
@media (max-width: 1100px) {
  .of-fe-grid { grid-template-columns: 1fr; }
  .of-fe-preview { position: static; max-height: none; }
}
```

- [ ] **Step 7: Run the tests to verify they pass**

Run: `pnpm -C web/apps/admin exec vitest run src/editors/form/form-components.test.tsx src/editors/form/FormPreview.test.tsx`
Expected: PASS, `Tests  16 passed`.

- [ ] **Step 8: Commit**

```bash
git add web/apps/admin/src/editors/form/FieldList.tsx web/apps/admin/src/editors/form/FieldInspector.tsx web/apps/admin/src/editors/form/ShowIfBuilder.tsx web/apps/admin/src/editors/form/ValidationEditor.tsx web/apps/admin/src/editors/form/FormMetaPanel.tsx web/apps/admin/src/editors/form/FormPreview.tsx web/apps/admin/src/editors/form/form-editor.css web/apps/admin/src/editors/form/form-components.test.tsx web/apps/admin/src/editors/form/FormPreview.test.tsx
git commit -m "feat(admin): form editor panels - field list, inspector, condition builder, live preview"
```

---

### Task 7: Form editor page

**Parallel group:** P8-A (form lane, step 3 of 3).

**Files:**
- Create: `web/apps/admin/src/editors/form/FormEditorPage.tsx`
- Test: `web/apps/admin/src/editors/form/FormEditorPage.test.tsx`

**Interfaces:**
- Consumes: Tasks 2–6, plus `renderWithProviders`, `server` (Plan 07) and `formRecordJson`, `applyItem` (Task 4).
- Produces: `FormEditorPage()`. It reads `:slug` from the route; with no slug it acts as the new-form editor. After a successful save it navigates to `/forms/<slug>` (the form overview, relative to the router basename).

- [ ] **Step 1: Write the failing tests**

Create `web/apps/admin/src/editors/form/FormEditorPage.test.tsx`:
```tsx
import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import { server } from "../../test/server";
import { renderWithProviders } from "../../test/render";
import { FormEditorPage } from "./FormEditorPage";
import { applyItem, formRecordJson } from "../test/records";
import { sampleForm } from "../test/samples";
import type { FormDef } from "../shared/types";

function mockApi({ source = "ui", exists = true }: { source?: string; exists?: boolean } = {}) {
  const puts: { url: string; body: FormDef }[] = [];
  server.use(
    http.get("/api/v1/forms/job-application", () =>
      exists
        ? HttpResponse.json({ form: formRecordJson(sampleForm(), { source }) })
        : HttpResponse.json({ error: { code: "not_found", message: "definition not found" } }, { status: 404 }),
    ),
    http.get("/api/v1/workflows", () => HttpResponse.json({ items: [] })),
    http.put("/api/v1/forms/:slug", async ({ request }) => {
      puts.push({ url: request.url, body: (await request.json()) as FormDef });
      return HttpResponse.json({ item: applyItem("form", "job-application", 4) });
    }),
  );
  return puts;
}

const renderEdit = () => renderWithProviders(<FormEditorPage />, { route: "/forms/job-application/edit", path: "/forms/:slug/edit" });
const fieldButton = (label: string) =>
  within(screen.getByRole("navigation", { name: "Fields" })).getAllByRole("button", { name: new RegExp(`^${label}`) })[0];

describe("FormEditorPage", () => {
  it("loads the form into the editor", async () => {
    mockApi();
    renderEdit();
    expect(await screen.findByRole("heading", { name: "Edit form: Job application" })).toBeInTheDocument();
    expect(fieldButton("Full name")).toBeInTheDocument();
    expect(screen.queryByRole("note")).toBeNull();
  });

  it("warns when the form is managed by the CLI", async () => {
    mockApi({ source: "cli" });
    renderEdit();
    expect(await screen.findByRole("note")).toHaveTextContent("This form is managed in code.");
  });

  it("saves edits with source=ui", async () => {
    const puts = mockApi();
    const user = userEvent.setup();
    renderEdit();
    await user.click(await screen.findByRole("button", { name: /^Full name/ }));
    const label = within(screen.getByRole("region", { name: "Field settings" })).getByLabelText("Label");
    await user.clear(label);
    await user.type(label, "Your name");
    expect(screen.getByText("Unsaved changes")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(puts).toHaveLength(1));
    expect(new URL(puts[0].url).searchParams.get("source")).toBe("ui");
    expect(puts[0].body.fields[0].label).toBe("Your name");
  });

  it("shows server problems, including ones that match no control", async () => {
    mockApi();
    server.use(
      http.put("/api/v1/forms/:slug", () =>
        HttpResponse.json(
          {
            error: {
              code: "validation_failed",
              message: "invalid",
              details: [
                { path: "fields[0].label", message: "server says no" },
                { path: "settings.unknownThing", message: "odd server rule" },
              ],
            },
          },
          { status: 422 },
        ),
      ),
    );
    const user = userEvent.setup();
    renderEdit();
    await user.click(await screen.findByRole("button", { name: "Save" }));
    const problemList = await screen.findByRole("region", { name: "Problems" });
    expect(within(problemList).getByText("odd server rule")).toBeInTheDocument();
    await user.click(within(problemList).getByRole("button", { name: /server says no/ }));
    expect(within(screen.getByRole("region", { name: "Field settings" })).getByText("server says no")).toBeInTheDocument();
  });

  it("blocks saving while client-side problems exist", async () => {
    const puts = mockApi();
    const user = userEvent.setup();
    renderEdit();
    await user.click(await screen.findByRole("button", { name: /^Email/ }));
    const key = within(screen.getByRole("region", { name: "Field settings" })).getByLabelText("Key");
    await user.clear(key);
    await user.type(key, "name");
    await user.click(screen.getByRole("button", { name: "Save" }));
    expect(await screen.findByText(/Fix \d+ problems? before saving\./)).toBeInTheDocument();
    expect(puts).toHaveLength(0);
  });

  it("refuses to create a new form with a taken slug", async () => {
    const puts = mockApi();
    const user = userEvent.setup();
    renderWithProviders(<FormEditorPage />, { route: "/forms/new", path: "/forms/new" });
    expect(await screen.findByRole("heading", { name: "New form" })).toBeInTheDocument();
    await user.type(screen.getByLabelText("Slug"), "job-application");
    await user.click(screen.getByRole("button", { name: "Add field" }));
    await user.click(screen.getByRole("button", { name: "Save" }));
    expect(await screen.findByText(/already exists/)).toBeInTheDocument();
    expect(puts).toHaveLength(0);
  });

  it("keeps the visual state when YAML is invalid and applies valid YAML", async () => {
    mockApi();
    const user = userEvent.setup();
    renderEdit();
    await screen.findByRole("heading", { name: "Edit form: Job application" });
    await user.click(screen.getByRole("tab", { name: "YAML" }));
    const textarea = screen.getByLabelText("YAML") as HTMLTextAreaElement;
    expect(textarea.value).toContain("slug: job-application");
    fireEvent.change(textarea, { target: { value: "fields: [\n" } });
    expect(screen.getByRole("alert")).toBeInTheDocument();
    await user.click(screen.getByRole("tab", { name: "Visual" }));
    expect(fieldButton("Full name")).toBeInTheDocument();

    await user.click(screen.getByRole("tab", { name: "YAML" }));
    const fresh = screen.getByLabelText("YAML") as HTMLTextAreaElement;
    fireEvent.change(fresh, { target: { value: fresh.value.replace("title: Job application", "title: Renamed") } });
    expect(screen.getByRole("heading", { name: "Edit form: Renamed" })).toBeInTheDocument();
  });

  it("reports a missing form", async () => {
    mockApi({ exists: false });
    renderEdit();
    expect(await screen.findByRole("alert")).toHaveTextContent('Form "job-application" was not found.');
  });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `pnpm -C web/apps/admin exec vitest run src/editors/form/FormEditorPage.test.tsx`
Expected: FAIL with `Failed to resolve import "./FormEditorPage"`.

- [ ] **Step 3: Implement the page**

Create `web/apps/admin/src/editors/form/FormEditorPage.tsx`:
```tsx
import { useEffect, useMemo, useReducer, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { OpenFormsError } from "@openforms/sdk";
import { editorKeys, listWorkflowSlugs, loadForm } from "../api";
import { CodeManagedBanner } from "../shared/CodeManagedBanner";
import { EditorLayout, type EditorTab } from "../shared/EditorLayout";
import { indexFromPath, mergeProblems } from "../shared/problems";
import { useDefinitionSave } from "../shared/useDefinitionSave";
import { useUnsavedChangesGuard } from "../shared/useUnsavedChangesGuard";
import { validateForm } from "../shared/validation";
import { YamlPane } from "../shared/YamlPane";
import type { FormDef, Problem, Source } from "../shared/types";
import { FieldInspector } from "./FieldInspector";
import { FieldList } from "./FieldList";
import { FormMetaPanel } from "./FormMetaPanel";
import { FormPreview } from "./FormPreview";
import { formEditorReducer, initFormEditor, newFormDefinition } from "./formReducer";
import "./form-editor.css";

export function FormEditorPage() {
  const { slug } = useParams<{ slug: string }>();
  const loaded = useQuery({
    queryKey: editorKeys.definition("form", slug ?? ""),
    queryFn: () => loadForm(slug!),
    enabled: Boolean(slug),
  });
  const workflows = useQuery({ queryKey: editorKeys.workflowSlugs, queryFn: listWorkflowSlugs });

  if (slug && loaded.isPending) return <p className="of-ed-hint">Loading form…</p>;
  if (slug && loaded.isError) {
    const notFound = loaded.error instanceof OpenFormsError && loaded.error.status === 404;
    return (
      <p role="alert" className="of-ed-alert">
        {notFound ? `Form "${slug}" was not found.` : `Could not load the form: ${loaded.error.message}`}
      </p>
    );
  }
  return (
    <FormEditor
      key={slug ?? "new"}
      initial={loaded.data?.definition ?? newFormDefinition()}
      source={loaded.data?.source}
      isNew={!slug}
      workflowSlugs={workflows.data ?? []}
    />
  );
}

function FormEditor({
  initial,
  source,
  isNew,
  workflowSlugs,
}: {
  initial: FormDef;
  source?: Source;
  isNew: boolean;
  workflowSlugs: string[];
}) {
  const navigate = useNavigate();
  const [state, dispatch] = useReducer(formEditorReducer, initial, initFormEditor);
  const [tab, setTab] = useState<EditorTab>("visual");
  const [blocked, setBlocked] = useState<string | null>(null);
  const { save, saving, serverProblems, error, clearServerProblems } = useDefinitionSave("form");

  const clientProblems = useMemo(() => validateForm(state.def), [state.def]);
  const problems = useMemo(() => mergeProblems(clientProblems, serverProblems), [clientProblems, serverProblems]);

  // Server problems describe the last save attempt; drop them once the user edits again.
  useEffect(() => {
    clearServerProblems();
    setBlocked(null);
  }, [state.def, clearServerProblems]);

  useUnsavedChangesGuard(state.dirty);

  const onSave = () => {
    if (clientProblems.length > 0) {
      const n = clientProblems.length;
      setBlocked(`Fix ${n} ${n === 1 ? "problem" : "problems"} before saving.`);
      return;
    }
    const slug = state.def.slug;
    save(state.def, isNew, () => {
      dispatch({ type: "markSaved" });
      navigate(`/forms/${slug}`);
    });
  };

  const onSelectProblem = (problem: Problem) => {
    setTab("visual");
    dispatch({ type: "select", index: indexFromPath(problem.path, "fields") });
  };

  return (
    <EditorLayout
      title={isNew ? "New form" : `Edit form: ${state.def.title || state.def.slug}`}
      tab={tab}
      onTabChange={setTab}
      onSave={onSave}
      saving={saving}
      dirty={state.dirty}
      banner={<CodeManagedBanner kind="form" source={source} />}
      problems={problems}
      onSelectProblem={onSelectProblem}
      error={error}
      saveBlockedMessage={blocked}
    >
      {tab === "visual" ? (
        <div className="of-fe-grid">
          <FieldList fields={state.def.fields} selected={state.selected} problems={problems} dispatch={dispatch} />
          {state.selected === null ? (
            <FormMetaPanel def={state.def} isNew={isNew} workflowSlugs={workflowSlugs} problems={problems} dispatch={dispatch} />
          ) : (
            <FieldInspector def={state.def} index={state.selected} problems={problems} dispatch={dispatch} />
          )}
          <FormPreview def={state.def} />
        </div>
      ) : (
        <YamlPane value={state.def} onApply={(next) => dispatch({ type: "replace", def: next as unknown as FormDef })} />
      )}
    </EditorLayout>
  );
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `pnpm -C web/apps/admin exec vitest run src/editors/form`
Expected: PASS, `Test Files  4 passed`, `Tests  47 passed`.

- [ ] **Step 5: Commit**

```bash
git add web/apps/admin/src/editors/form/FormEditorPage.tsx web/apps/admin/src/editors/form/FormEditorPage.test.tsx
git commit -m "feat(admin): form editor page with visual/YAML modes, save flow and CLI banner"
```

---

### Task 8: Workflow editor reducer

**Parallel group:** P8-B (workflow lane, step 1 of 4). Runs concurrently with P8-A and P8-C.

**Files:**
- Create: `web/apps/admin/src/editors/workflow/workflowReducer.ts`
- Test: `web/apps/admin/src/editors/workflow/workflowReducer.test.ts`

**Interfaces:**
- Consumes: `types.ts`, `objects.ts`.
- Produces:
  - `WorkflowSelection = { kind: "workflow" } | { kind: "state" | "transition" | "field"; index: number }`
  - `ActionTarget = { kind: "transition"; index: number } | { kind: "onSubmit" }`
  - `WorkflowEditorState = { def; selection; dirty }`
  - `WorkflowEditorAction`: `setMeta`, `setInitial`, `addState`, `updateState`, `renameState`, `removeState`, `addTransition`, `updateTransition`, `setGuard`, `removeTransition`, `addField`, `updateField`, `renameField`, `removeField`, `addAction`, `updateAction`, `removeAction`, `select`, `replace`, `markSaved`
  - `workflowEditorReducer`, `initWorkflowEditor`, `newWorkflowDefinition()`, `newAction(type)`, `normalizeWorkflow(input)`

- [ ] **Step 1: Write the failing tests**

Create `web/apps/admin/src/editors/workflow/workflowReducer.test.ts`:
```ts
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `pnpm -C web/apps/admin exec vitest run src/editors/workflow/workflowReducer.test.ts`
Expected: FAIL with `Failed to resolve import "./workflowReducer"`.

- [ ] **Step 3: Implement the reducer**

Create `web/apps/admin/src/editors/workflow/workflowReducer.ts`:
```ts
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
    const next = { ...def, onSubmit: actions };
    if (actions.length === 0) delete next.onSubmit;
    return next;
  }
  return {
    ...def,
    transitions: def.transitions.map((t, i) => {
      if (i !== target.index) return t;
      const next = { ...t, actions };
      if (actions.length === 0) delete next.actions;
      return next;
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
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `pnpm -C web/apps/admin exec vitest run src/editors/workflow/workflowReducer.test.ts`
Expected: PASS, `Tests  26 passed`.

- [ ] **Step 5: Commit**

```bash
git add web/apps/admin/src/editors/workflow/workflowReducer.ts web/apps/admin/src/editors/workflow/workflowReducer.test.ts
git commit -m "feat(admin): workflow editor reducer with reference-aware state/field edits"
```

---

### Task 9: Workflow diagram (dagre layout + React Flow)

**Parallel group:** P8-B (workflow lane, step 2 of 4).

**Files:**
- Create: `web/apps/admin/src/editors/workflow/layout.ts`
- Create: `web/apps/admin/src/editors/workflow/WorkflowDiagram.tsx`
- Create: `web/apps/admin/src/editors/workflow/workflow-editor.css`
- Create: `web/apps/admin/src/editors/test/xyflowMock.tsx`
- Test: `web/apps/admin/src/editors/workflow/layout.test.ts`, `web/apps/admin/src/editors/workflow/WorkflowDiagram.test.tsx`

**Interfaces:**
- Consumes: `WorkflowDef` and `WorkflowSelection` (Task 8).
- Produces:
  - `NODE_WIDTH`, `NODE_HEIGHT`
  - `StateNodeData = { label; stateKey; color; terminal; initial }`
  - `TransitionEdgeData = { transitionIndex }`
  - `layoutWorkflow(def, selection?) → { nodes: Node<StateNodeData, "state">[]; edges: Edge<TransitionEdgeData>[] }`: one node per distinct state key, one edge per (transition, known source), left-to-right ranks
  - `WorkflowDiagram({ def, selection, onSelect(sel) })`
  - `StateNode`
  - the test double `editors/test/xyflowMock.tsx`

- [ ] **Step 1: Write the failing tests and the React Flow test double**

Create `web/apps/admin/src/editors/test/xyflowMock.tsx`:
```tsx
// Minimal stand-in for @xyflow/react in jsdom (no layout engine, no ResizeObserver).
// Usage in a test file: vi.mock("@xyflow/react", () => import("../test/xyflowMock"));
import type { ComponentType, MouseEvent, ReactNode } from "react";

type NodeLike = { id: string; type?: string; data: Record<string, unknown>; selected?: boolean };
type EdgeLike = { id: string; source: string; target: string; label?: ReactNode; data?: Record<string, unknown> };
type NodeComponent = ComponentType<{ id: string; data: Record<string, unknown>; selected: boolean }>;

export function ReactFlow(props: {
  nodes: NodeLike[];
  edges: EdgeLike[];
  nodeTypes?: Record<string, NodeComponent>;
  onNodeClick?: (event: MouseEvent, node: NodeLike) => void;
  onEdgeClick?: (event: MouseEvent, edge: EdgeLike) => void;
  children?: ReactNode;
}) {
  return (
    <div data-testid="reactflow">
      {props.nodes.map((n) => {
        const Custom = n.type ? props.nodeTypes?.[n.type] : undefined;
        return (
          <div key={n.id} role="button" tabIndex={0} aria-label={`node ${n.id}`} onClick={(e) => props.onNodeClick?.(e, n)}>
            {Custom ? <Custom id={n.id} data={n.data} selected={Boolean(n.selected)} /> : n.id}
          </div>
        );
      })}
      {props.edges.map((e) => (
        <button key={e.id} type="button" aria-label={`edge ${e.id}`} onClick={(ev) => props.onEdgeClick?.(ev, e)}>
          {e.label}
        </button>
      ))}
      {props.children}
    </div>
  );
}

export const Handle = () => null;
export const Background = () => null;
export const Controls = () => null;
export const Position = { Left: "left", Right: "right", Top: "top", Bottom: "bottom" } as const;
export const MarkerType = { Arrow: "arrow", ArrowClosed: "arrowclosed" } as const;
```

Create `web/apps/admin/src/editors/workflow/layout.test.ts`:
```ts
import { describe, expect, it } from "vitest";
import { layoutWorkflow } from "./layout";
import { sampleWorkflow } from "../test/samples";

describe("layoutWorkflow", () => {
  it("creates one node per state with display data", () => {
    const { nodes } = layoutWorkflow(sampleWorkflow());
    expect(nodes.map((n) => n.id)).toEqual(["new", "screening", "hired", "rejected"]);
    expect(nodes[0]).toMatchObject({ type: "state", data: { label: "New", stateKey: "new", color: "gray", initial: true, terminal: false } });
    expect(nodes[2].data).toMatchObject({ terminal: true, initial: false, color: "green" });
  });

  it("lays states out left to right in transition order", () => {
    const { nodes } = layoutWorkflow(sampleWorkflow());
    const x = Object.fromEntries(nodes.map((n) => [n.id, n.position.x]));
    expect(x.new).toBeLessThan(x.screening);
    expect(x.screening).toBeLessThan(x.hired);
  });

  it("creates one edge per transition source, labelled and indexed", () => {
    const { edges } = layoutWorkflow(sampleWorkflow());
    expect(edges).toHaveLength(4);
    const reject = edges.filter((e) => e.data?.transitionIndex === 2);
    expect(reject.map((e) => [e.source, e.target])).toEqual([
      ["new", "rejected"],
      ["screening", "rejected"],
    ]);
    expect(reject[0].label).toBe("Reject");
    expect(new Set(edges.map((e) => e.id)).size).toBe(4);
  });

  it("skips edges that reference unknown states and ignores duplicate state keys", () => {
    const wf = sampleWorkflow();
    wf.states.push({ key: "new", label: "Duplicate" });
    wf.transitions.push({ key: "ghost", label: "Ghost", from: ["nowhere"], to: "new", guard: {} });
    const { nodes, edges } = layoutWorkflow(wf);
    expect(nodes).toHaveLength(4);
    expect(edges).toHaveLength(4);
  });

  it("marks the selected state and transition", () => {
    expect(layoutWorkflow(sampleWorkflow(), { kind: "state", index: 1 }).nodes[1].selected).toBe(true);
    const { edges } = layoutWorkflow(sampleWorkflow(), { kind: "transition", index: 2 });
    expect(edges.filter((e) => e.selected)).toHaveLength(2);
  });

  it("handles an empty workflow", () => {
    expect(layoutWorkflow({ ...sampleWorkflow(), states: [], transitions: [] })).toEqual({ nodes: [], edges: [] });
  });
});
```

Create `web/apps/admin/src/editors/workflow/WorkflowDiagram.test.tsx`:
```tsx
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { WorkflowDiagram } from "./WorkflowDiagram";
import { sampleWorkflow } from "../test/samples";

vi.mock("@xyflow/react", () => import("../test/xyflowMock"));

describe("WorkflowDiagram", () => {
  it("renders state nodes with initial and terminal markers", () => {
    render(<WorkflowDiagram def={sampleWorkflow()} selection={{ kind: "workflow" }} onSelect={() => {}} />);
    const newNode = screen.getByRole("button", { name: "node new" });
    expect(within(newNode).getByText("New")).toBeInTheDocument();
    expect(within(newNode).getByLabelText("Initial state")).toBeInTheDocument();
    expect(within(screen.getByRole("button", { name: "node hired" })).getByText("Hired").closest(".of-wf-node")).toHaveClass("of-wf-node--terminal");
  });

  it("selects states and transitions on click", async () => {
    const user = userEvent.setup();
    const onSelect = vi.fn();
    render(<WorkflowDiagram def={sampleWorkflow()} selection={{ kind: "workflow" }} onSelect={onSelect} />);
    await user.click(screen.getByRole("button", { name: "node screening" }));
    expect(onSelect).toHaveBeenLastCalledWith({ kind: "state", index: 1 });
    await user.click(screen.getAllByRole("button", { name: /^edge reject:/ })[0]);
    expect(onSelect).toHaveBeenLastCalledWith({ kind: "transition", index: 2 });
  });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `pnpm -C web/apps/admin exec vitest run src/editors/workflow/layout.test.ts src/editors/workflow/WorkflowDiagram.test.tsx`
Expected: FAIL with `Failed to resolve import "./layout"` / `"./WorkflowDiagram"`.

- [ ] **Step 3: Implement the layout**

Create `web/apps/admin/src/editors/workflow/layout.ts`:
```ts
import dagre from "@dagrejs/dagre";
import { MarkerType, type Edge, type Node } from "@xyflow/react";
import type { WorkflowDef } from "../shared/types";
import type { WorkflowSelection } from "./workflowReducer";

export const NODE_WIDTH = 168;
export const NODE_HEIGHT = 56;

export type StateNodeData = { label: string; stateKey: string; color: string; terminal: boolean; initial: boolean };
export type TransitionEdgeData = { transitionIndex: number };

export function layoutWorkflow(
  def: WorkflowDef,
  selection: WorkflowSelection = { kind: "workflow" },
): { nodes: Node<StateNodeData, "state">[]; edges: Edge<TransitionEdgeData>[] } {
  const graph = new dagre.graphlib.Graph({ multigraph: true });
  graph.setGraph({ rankdir: "LR", nodesep: 40, ranksep: 90, marginx: 16, marginy: 16 });
  graph.setDefaultEdgeLabel(() => ({}));

  const seen = new Set<string>();
  const states = def.states
    .map((state, index) => ({ state, index }))
    .filter(({ state }) => {
      if (seen.has(state.key)) return false;
      seen.add(state.key);
      return true;
    });
  states.forEach(({ state }) => graph.setNode(state.key, { width: NODE_WIDTH, height: NODE_HEIGHT }));

  const edges: Edge<TransitionEdgeData>[] = [];
  def.transitions.forEach((t, transitionIndex) => {
    t.from.forEach((from) => {
      if (!seen.has(from) || !seen.has(t.to)) return;
      const id = `${t.key}:${from}->${t.to}:${transitionIndex}`;
      graph.setEdge(from, t.to, {}, id);
      edges.push({
        id,
        source: from,
        target: t.to,
        label: t.label || t.key,
        data: { transitionIndex },
        selected: selection.kind === "transition" && selection.index === transitionIndex,
        markerEnd: { type: MarkerType.ArrowClosed },
      });
    });
  });

  if (states.length > 0) dagre.layout(graph);

  const nodes = states.map(({ state, index }) => {
    const positioned = graph.node(state.key);
    return {
      id: state.key,
      type: "state" as const,
      position: { x: positioned.x - NODE_WIDTH / 2, y: positioned.y - NODE_HEIGHT / 2 },
      data: {
        label: state.label || state.key,
        stateKey: state.key,
        color: state.color ?? "gray",
        terminal: Boolean(state.terminal),
        initial: def.initial === state.key,
      },
      selected: selection.kind === "state" && selection.index === index,
    };
  });

  return { nodes, edges };
}
```

- [ ] **Step 4: Implement the diagram and workflow CSS**

Create `web/apps/admin/src/editors/workflow/WorkflowDiagram.tsx`:
```tsx
import { useMemo } from "react";
import { Background, Controls, Handle, Position, ReactFlow, type Node, type NodeProps } from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import type { WorkflowDef } from "../shared/types";
import { layoutWorkflow, type StateNodeData, type TransitionEdgeData } from "./layout";
import type { WorkflowSelection } from "./workflowReducer";

export function StateNode({ data, selected }: NodeProps<Node<StateNodeData, "state">>) {
  const className = [
    "of-wf-node",
    `of-wf-node--${data.color}`,
    data.terminal ? "of-wf-node--terminal" : "",
    selected ? "is-selected" : "",
  ]
    .filter(Boolean)
    .join(" ");
  return (
    <div className={className}>
      <Handle type="target" position={Position.Left} />
      {data.initial ? (
        <span className="of-wf-node__initial" aria-label="Initial state" title="Initial state">
          ▶
        </span>
      ) : null}
      <strong>{data.label}</strong>
      <small>{data.stateKey}</small>
      <Handle type="source" position={Position.Right} />
    </div>
  );
}

const nodeTypes = { state: StateNode };

export function WorkflowDiagram({
  def,
  selection,
  onSelect,
}: {
  def: WorkflowDef;
  selection: WorkflowSelection;
  onSelect: (selection: WorkflowSelection) => void;
}) {
  const { nodes, edges } = useMemo(() => layoutWorkflow(def, selection), [def, selection]);
  return (
    <div className="of-wf-diagram" aria-label="Workflow diagram">
      <ReactFlow
        nodes={nodes}
        edges={edges}
        nodeTypes={nodeTypes}
        fitView
        nodesDraggable={false}
        nodesConnectable={false}
        onNodeClick={(_event, node) => {
          const index = def.states.findIndex((s) => s.key === node.id);
          if (index >= 0) onSelect({ kind: "state", index });
        }}
        onEdgeClick={(_event, edge) => {
          const data = edge.data as TransitionEdgeData | undefined;
          if (data) onSelect({ kind: "transition", index: data.transitionIndex });
        }}
      >
        <Background />
        <Controls showInteractive={false} />
      </ReactFlow>
    </div>
  );
}
```

Create `web/apps/admin/src/editors/workflow/workflow-editor.css`:
```css
.of-we-grid { display: grid; grid-template-columns: minmax(220px, 260px) minmax(360px, 1fr) minmax(300px, 380px); gap: 16px; align-items: start; }
.of-we-grid > * { min-width: 0; }
.of-wf-diagram { height: 520px; border: 1px solid var(--color-border, #d0d5dd); border-radius: 8px; background: var(--color-surface, #fff); }
.of-wf-node {
  --of-node: #667085;
  position: relative; box-sizing: border-box; width: 168px; min-height: 56px; padding: 8px 10px 8px 22px;
  border: 2px solid var(--of-node); border-radius: 10px; background: var(--color-surface, #fff); color: var(--color-text, #101828);
  display: flex; flex-direction: column; justify-content: center; font-size: 0.8125rem;
}
.of-wf-node small { color: var(--color-muted, #667085); font-family: var(--font-mono, ui-monospace, monospace); font-size: 0.6875rem; }
.of-wf-node--terminal { border-style: double; border-width: 4px; }
.of-wf-node.is-selected { box-shadow: 0 0 0 3px var(--color-accent-subtle, #c7d2fe); }
.of-wf-node__initial { position: absolute; left: 6px; top: 50%; transform: translateY(-50%); font-size: 0.625rem; color: var(--of-node); }
.of-wf-node--gray { --of-node: var(--state-gray, #667085); }
.of-wf-node--blue { --of-node: var(--state-blue, #1570ef); }
.of-wf-node--green { --of-node: var(--state-green, #079455); }
.of-wf-node--yellow { --of-node: var(--state-yellow, #ca8504); }
.of-wf-node--red { --of-node: var(--state-red, #d92d20); }
.of-wf-node--purple { --of-node: var(--state-purple, #7a5af8); }
.of-wf-dot { display: inline-block; width: 8px; height: 8px; border-radius: 50%; margin-right: 6px; background: var(--of-dot, #667085); }
.of-wf-dot--gray { --of-dot: var(--state-gray, #667085); }
.of-wf-dot--blue { --of-dot: var(--state-blue, #1570ef); }
.of-wf-dot--green { --of-dot: var(--state-green, #079455); }
.of-wf-dot--yellow { --of-dot: var(--state-yellow, #ca8504); }
.of-wf-dot--red { --of-dot: var(--state-red, #d92d20); }
.of-wf-dot--purple { --of-dot: var(--state-purple, #7a5af8); }
@media (max-width: 1200px) {
  .of-we-grid { grid-template-columns: 1fr; }
  .of-wf-diagram { height: 380px; }
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `pnpm -C web/apps/admin exec vitest run src/editors/workflow/layout.test.ts src/editors/workflow/WorkflowDiagram.test.tsx`
Expected: PASS, `Tests  8 passed`.

- [ ] **Step 6: Commit**

```bash
git add web/apps/admin/src/editors/workflow/layout.ts web/apps/admin/src/editors/workflow/layout.test.ts web/apps/admin/src/editors/workflow/WorkflowDiagram.tsx web/apps/admin/src/editors/workflow/WorkflowDiagram.test.tsx web/apps/admin/src/editors/workflow/workflow-editor.css web/apps/admin/src/editors/test/xyflowMock.tsx
git commit -m "feat(admin): workflow state diagram with dagre left-to-right layout"
```

---

### Task 10: Workflow editor panels (outline, inspectors, actions)

**Parallel group:** P8-B (workflow lane, step 3 of 4).

**Files:**
- Create: `web/apps/admin/src/editors/workflow/ActionsEditor.tsx`
- Create: `web/apps/admin/src/editors/workflow/StateInspector.tsx`
- Create: `web/apps/admin/src/editors/workflow/TransitionInspector.tsx`
- Create: `web/apps/admin/src/editors/workflow/WorkflowFieldInspector.tsx`
- Create: `web/apps/admin/src/editors/workflow/WorkflowMetaPanel.tsx`
- Create: `web/apps/admin/src/editors/workflow/WorkflowOutline.tsx`
- Test: `web/apps/admin/src/editors/workflow/workflow-components.test.tsx`

**Interfaces:**
- Consumes: the Task 8 reducer, `ActionTarget`, the shared controls, `TagInput`, `OptionsEditor` and the problem helpers.
- Produces:
  - `ActionsEditor({ title, actions, basePath, problems, onAdd(type), onUpdate(i, patch), onRemove(i) })`
  - `TEMPLATE_HINT`
  - `StateInspector` (region "State settings")
  - `TransitionInspector` (region "Transition settings")
  - `WorkflowFieldInspector` (region "Workflow field settings")
  - `WorkflowMetaPanel` (region "Workflow settings")
  - `WorkflowOutline` (navigation "Workflow outline")
  - All share the props `({ def, index?, isNew?, selection?, problems, dispatch })` as shown in the code.

- [ ] **Step 1: Write the failing tests**

Create `web/apps/admin/src/editors/workflow/workflow-components.test.tsx`:
```tsx
import { useReducer } from "react";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { initWorkflowEditor, workflowEditorReducer } from "./workflowReducer";
import { WorkflowOutline } from "./WorkflowOutline";
import { WorkflowMetaPanel } from "./WorkflowMetaPanel";
import { StateInspector } from "./StateInspector";
import { TransitionInspector } from "./TransitionInspector";
import { WorkflowFieldInspector } from "./WorkflowFieldInspector";
import { sampleWorkflow } from "../test/samples";
import type { Problem, WorkflowDef } from "../shared/types";

function Harness({ problems = [], isNew = false }: { problems?: Problem[]; isNew?: boolean }) {
  const [state, dispatch] = useReducer(workflowEditorReducer, sampleWorkflow(), initWorkflowEditor);
  const sel = state.selection;
  return (
    <>
      <WorkflowOutline def={state.def} selection={sel} problems={problems} dispatch={dispatch} />
      {sel.kind === "workflow" ? <WorkflowMetaPanel def={state.def} isNew={isNew} problems={problems} dispatch={dispatch} /> : null}
      {sel.kind === "state" ? <StateInspector def={state.def} index={sel.index} problems={problems} dispatch={dispatch} /> : null}
      {sel.kind === "transition" ? <TransitionInspector def={state.def} index={sel.index} problems={problems} dispatch={dispatch} /> : null}
      {sel.kind === "field" ? <WorkflowFieldInspector def={state.def} index={sel.index} problems={problems} dispatch={dispatch} /> : null}
      <output data-testid="def">{JSON.stringify(state.def)}</output>
    </>
  );
}

const current = (): WorkflowDef => JSON.parse(screen.getByTestId("def").textContent!);
const outline = () => screen.getByRole("navigation", { name: "Workflow outline" });
const pick = (user: ReturnType<typeof userEvent.setup>, name: string) =>
  user.click(within(outline()).getAllByRole("button", { name: new RegExp(`^${name}`) })[0]);

describe("WorkflowOutline", () => {
  it("lists states, transitions and fields and marks problems", () => {
    render(<Harness problems={[{ path: "transitions[1].to", message: "bad" }]} />);
    expect(within(outline()).getByRole("button", { name: /^New/ })).toHaveTextContent("initial");
    expect(within(outline()).getByRole("button", { name: /^Hire/ })).toHaveTextContent("screening → hired");
    expect(within(outline()).getByLabelText("1 problem")).toBeInTheDocument();
  });

  it("adds a state and opens it", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await user.click(screen.getByRole("button", { name: "Add state" }));
    expect(screen.getByRole("heading", { name: "State: New state" })).toBeInTheDocument();
  });
});

describe("StateInspector", () => {
  it("renames a state and updates transitions", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await pick(user, "Screening");
    const key = within(screen.getByRole("region", { name: "State settings" })).getByLabelText("Key");
    await user.clear(key);
    await user.type(key, "review");
    expect(current().transitions[0].to).toBe("review");
  });

  it("deletes a state together with its dependent transitions", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await pick(user, "Screening");
    await user.click(screen.getByRole("button", { name: "Delete state" }));
    expect(current().transitions.map((t) => t.key)).toEqual(["reject"]);
  });

  it("sets the initial state", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await pick(user, "Screening");
    await user.click(screen.getByLabelText("Initial state (new submissions start here)"));
    expect(current().initial).toBe("screening");
  });
});

describe("TransitionInspector", () => {
  it("edits sources, disables terminal sources, and edits guards", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await pick(user, "Start screening");
    const region = screen.getByRole("region", { name: "Transition settings" });
    expect(within(region).getByLabelText("Hired (hired) · terminal")).toBeDisabled();
    await user.click(within(region).getByLabelText("Screening (screening)"));
    expect(current().transitions[0].from).toEqual(["new", "screening"]);
    await user.type(within(region).getByLabelText("Roles allowed"), "lead{Enter}");
    expect(current().transitions[0].guard.roles).toEqual(["reviewer", "lead"]);
    await user.click(within(region).getByLabelText("Score (score)"));
    expect(current().transitions[0].guard.requireFields).toEqual(["score"]);
  });

  it("edits, adds and switches actions", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await pick(user, "Reject");
    const region = screen.getByRole("region", { name: "Transition settings" });
    const email = within(region).getByRole("group", { name: "Action 1: Send email" });
    await user.clear(within(email).getByLabelText("Subject"));
    await user.type(within(email).getByLabelText("Subject"), "About your application");
    expect(current().transitions[2].actions?.[0].subject).toBe("About your application");

    await user.selectOptions(within(region).getByLabelText("New action type"), "assign");
    await user.click(within(region).getByRole("button", { name: "Add action" }));
    const assign = within(region).getByRole("group", { name: "Action 2: Assign reviewer" });
    await user.selectOptions(within(assign).getByLabelText("Assign to"), "user");
    await user.type(within(assign).getByLabelText("User email"), "lead@example.com");
    expect(current().transitions[2].actions?.[1]).toEqual({ type: "assign", user: "lead@example.com" });

    await user.click(within(region).getByRole("button", { name: "Remove action 1" }));
    expect(current().transitions[2].actions).toHaveLength(1);
  });
});

describe("WorkflowFieldInspector and WorkflowMetaPanel", () => {
  it("renames a workflow field and keeps requireFields in sync", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await pick(user, "Score");
    const key = within(screen.getByRole("region", { name: "Workflow field settings" })).getByLabelText("Key");
    await user.clear(key);
    await user.type(key, "rating");
    expect(current().transitions[1].guard.requireFields).toEqual(["rating"]);
  });

  it("adds onSubmit actions from workflow settings", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    const region = screen.getByRole("region", { name: "Workflow settings" });
    expect(within(region).getByLabelText("Slug")).toHaveAttribute("readonly");
    await user.selectOptions(within(region).getByLabelText("New action type"), "assign");
    await user.click(within(region).getByRole("button", { name: "Add action" }));
    await user.type(within(region).getByLabelText("Role"), "reviewer");
    expect(current().onSubmit).toEqual([{ type: "assign", role: "reviewer" }]);
  });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `pnpm -C web/apps/admin exec vitest run src/editors/workflow/workflow-components.test.tsx`
Expected: FAIL with `Failed to resolve import "./WorkflowOutline"`.

- [ ] **Step 3: Implement ActionsEditor**

Create `web/apps/admin/src/editors/workflow/ActionsEditor.tsx`:
```tsx
import { useState } from "react";
import { FieldErrors, SelectControl, TextControl } from "../shared/controls";
import { problemsAt } from "../shared/problems";
import { ACTION_LABELS, ACTION_TYPES, type Action, type ActionType, type Problem } from "../shared/types";

export const TEMPLATE_HINT =
  "Placeholders: {{submission.data.<key>}}, {{submission.fields.<key>}}, {{submission.id}}, {{submission.stateLabel}}, {{submission.url}}, {{form.title}}, {{transition.label}}.";

export function ActionsEditor({
  title,
  actions,
  basePath,
  problems,
  onAdd,
  onUpdate,
  onRemove,
}: {
  title: string;
  actions: Action[];
  basePath: string;
  problems: Problem[];
  onAdd: (type: ActionType) => void;
  onUpdate: (index: number, patch: Partial<Action>) => void;
  onRemove: (index: number) => void;
}) {
  const [newType, setNewType] = useState<ActionType>("email");
  return (
    <fieldset className="of-ed-fieldset">
      <legend>{title}</legend>
      {actions.length === 0 ? <p className="of-ed-hint">No actions.</p> : null}
      {actions.map((action, i) => {
        const base = `${basePath}[${i}]`;
        const at = (key: string) => problemsAt(problems, `${base}.${key}`);
        const name = ACTION_LABELS[action.type] ?? action.type;
        return (
          <div key={i} className="of-ed-card" role="group" aria-label={`Action ${i + 1}: ${name}`}>
            <div className="of-ed-card__head">
              <strong>{name}</strong>
              <button type="button" className="of-ed-icon-btn" aria-label={`Remove action ${i + 1}`} onClick={() => onRemove(i)}>
                ✕
              </button>
            </div>
            <FieldErrors problems={[...problemsAt(problems, base), ...at("type")]} />
            {action.type === "webhook" ? (
              <TextControl
                label="URL"
                monospace
                value={action.url}
                problems={at("url")}
                hint="Receives a signed JSON POST. Failed deliveries are retried with backoff."
                onChange={(v) => onUpdate(i, { url: v })}
              />
            ) : null}
            {action.type === "email" ? (
              <>
                <TextControl label="To" value={action.to} problems={at("to")} onChange={(v) => onUpdate(i, { to: v })} />
                <TextControl label="Subject" value={action.subject} problems={at("subject")} onChange={(v) => onUpdate(i, { subject: v })} />
                <TextControl label="Body" multiline value={action.body} problems={at("body")} hint={TEMPLATE_HINT} onChange={(v) => onUpdate(i, { body: v })} />
              </>
            ) : null}
            {action.type === "assign" ? (
              <>
                <SelectControl
                  label="Assign to"
                  value={action.user !== undefined ? "user" : "role"}
                  options={[
                    { value: "role", label: "Least busy person with a role" },
                    { value: "user", label: "A specific person (email)" },
                  ]}
                  onChange={(mode) => onUpdate(i, mode === "user" ? { user: "", role: undefined } : { role: "", user: undefined })}
                />
                {action.user !== undefined ? (
                  <TextControl label="User email" value={action.user} problems={at("user")} onChange={(v) => onUpdate(i, { user: v })} />
                ) : (
                  <TextControl label="Role" value={action.role} problems={at("role")} onChange={(v) => onUpdate(i, { role: v })} />
                )}
              </>
            ) : null}
          </div>
        );
      })}
      <div className="of-ed-add">
        <SelectControl
          label="New action type"
          value={newType}
          options={ACTION_TYPES.map((t) => ({ value: t, label: ACTION_LABELS[t] }))}
          onChange={(v) => setNewType(v as ActionType)}
        />
        <button type="button" className="of-ed-btn" onClick={() => onAdd(newType)}>
          Add action
        </button>
      </div>
    </fieldset>
  );
}
```

- [ ] **Step 4: Implement the inspectors, meta panel and outline**

Create `web/apps/admin/src/editors/workflow/StateInspector.tsx`:
```tsx
import type { Dispatch } from "react";
import { CheckboxControl, SelectControl, TextControl } from "../shared/controls";
import { problemsAt } from "../shared/problems";
import { STATE_COLORS, type Problem, type StateColor, type WorkflowDef } from "../shared/types";
import type { WorkflowEditorAction } from "./workflowReducer";

const capitalize = (s: string) => s.charAt(0).toUpperCase() + s.slice(1);

export function StateInspector({
  def,
  index,
  problems,
  dispatch,
}: {
  def: WorkflowDef;
  index: number;
  problems: Problem[];
  dispatch: Dispatch<WorkflowEditorAction>;
}) {
  const state = def.states[index];
  if (!state) return null;
  const at = (prop: string) => problemsAt(problems, `states[${index}].${prop}`);
  const isInitial = def.initial === state.key;
  return (
    <section className="of-ed-inspector" aria-label="State settings">
      <h2>State: {state.label || state.key}</h2>
      <TextControl label="Label" value={state.label} problems={at("label")} onChange={(v) => dispatch({ type: "updateState", index, patch: { label: v } })} />
      <TextControl
        label="Key"
        monospace
        value={state.key}
        problems={at("key")}
        hint="Stored on submissions. Renaming updates transitions and the initial state; existing submissions keep their old state key."
        onChange={(v) => dispatch({ type: "renameState", index, key: v })}
      />
      <SelectControl
        label="Color"
        value={state.color ?? "gray"}
        options={STATE_COLORS.map((c) => ({ value: c, label: capitalize(c) }))}
        onChange={(c) => dispatch({ type: "updateState", index, patch: { color: c as StateColor } })}
      />
      <CheckboxControl
        label="Terminal (no transitions can leave this state)"
        checked={Boolean(state.terminal)}
        onChange={(checked) => dispatch({ type: "updateState", index, patch: { terminal: checked } })}
      />
      <CheckboxControl
        label="Initial state (new submissions start here)"
        checked={isInitial}
        disabled={isInitial}
        hint={isInitial ? "To change it, mark another state as initial." : undefined}
        onChange={(checked) => checked && dispatch({ type: "setInitial", key: state.key })}
      />
      <button type="button" className="of-ed-btn of-ed-btn--danger" onClick={() => dispatch({ type: "removeState", index })}>
        Delete state
      </button>
      <p className="of-ed-hint">Deleting a state also deletes transitions into it and removes it from other transitions' sources.</p>
    </section>
  );
}
```

Create `web/apps/admin/src/editors/workflow/TransitionInspector.tsx`:
```tsx
import type { Dispatch } from "react";
import { CheckboxControl, FieldErrors, SelectControl, TextControl } from "../shared/controls";
import { problemsAt, problemsUnder } from "../shared/problems";
import { TagInput } from "../shared/TagInput";
import type { Problem, WorkflowDef } from "../shared/types";
import { ActionsEditor } from "./ActionsEditor";
import type { WorkflowEditorAction } from "./workflowReducer";

export function TransitionInspector({
  def,
  index,
  problems,
  dispatch,
}: {
  def: WorkflowDef;
  index: number;
  problems: Problem[];
  dispatch: Dispatch<WorkflowEditorAction>;
}) {
  const t = def.transitions[index];
  if (!t) return null;
  const base = `transitions[${index}]`;
  const at = (prop: string) => problemsAt(problems, `${base}.${prop}`);
  const target = { kind: "transition" as const, index };
  const fields = def.fields ?? [];
  const toOptions = def.states.map((s) => ({ value: s.key, label: `${s.label} (${s.key})` }));
  if (!def.states.some((s) => s.key === t.to)) toOptions.unshift({ value: t.to, label: `${t.to || "(none)"} (unknown state)` });

  return (
    <section className="of-ed-inspector" aria-label="Transition settings">
      <h2>Transition: {t.label || t.key}</h2>
      <TextControl label="Label" value={t.label} problems={at("label")} onChange={(v) => dispatch({ type: "updateTransition", index, patch: { label: v } })} />
      <TextControl
        label="Key"
        monospace
        value={t.key}
        problems={at("key")}
        hint="Used by the API: POST /submissions/{id}/transitions with this key."
        onChange={(v) => dispatch({ type: "updateTransition", index, patch: { key: v } })}
      />
      <fieldset className="of-ed-fieldset">
        <legend>From states</legend>
        <FieldErrors problems={problemsUnder(problems, `${base}.from`)} />
        {def.states.map((s) => {
          const checked = t.from.includes(s.key);
          return (
            <CheckboxControl
              key={s.key}
              label={`${s.label} (${s.key})${s.terminal ? " · terminal" : ""}`}
              checked={checked}
              disabled={Boolean(s.terminal) && !checked}
              onChange={(on) =>
                dispatch({ type: "updateTransition", index, patch: { from: on ? [...t.from, s.key] : t.from.filter((k) => k !== s.key) } })
              }
            />
          );
        })}
      </fieldset>
      <SelectControl label="To state" value={t.to} options={toOptions} problems={at("to")} onChange={(v) => dispatch({ type: "updateTransition", index, patch: { to: v } })} />
      <fieldset className="of-ed-fieldset">
        <legend>Guard</legend>
        <TagInput
          label="Roles allowed"
          values={t.guard.roles ?? []}
          placeholder="Type a role and press Enter"
          hint="Admins can always perform this transition. Leave empty to allow any signed-in user."
          onChange={(roles) => dispatch({ type: "setGuard", index, guard: { ...t.guard, roles } })}
        />
        {fields.length === 0 ? (
          <p className="of-ed-hint">Add workflow fields to require them before this transition.</p>
        ) : (
          <fieldset className="of-ed-fieldset">
            <legend>Required fields</legend>
            <FieldErrors problems={problemsUnder(problems, `${base}.guard.requireFields`)} />
            {fields.map((f) => {
              const required = t.guard.requireFields ?? [];
              return (
                <CheckboxControl
                  key={f.key}
                  label={`${f.label} (${f.key})`}
                  checked={required.includes(f.key)}
                  onChange={(on) =>
                    dispatch({
                      type: "setGuard",
                      index,
                      guard: { ...t.guard, requireFields: on ? [...required, f.key] : required.filter((k) => k !== f.key) },
                    })
                  }
                />
              );
            })}
          </fieldset>
        )}
      </fieldset>
      <ActionsEditor
        title="Actions when this transition happens"
        actions={t.actions ?? []}
        basePath={`${base}.actions`}
        problems={problems}
        onAdd={(actionType) => dispatch({ type: "addAction", target, actionType })}
        onUpdate={(actionIndex, patch) => dispatch({ type: "updateAction", target, actionIndex, patch })}
        onRemove={(actionIndex) => dispatch({ type: "removeAction", target, actionIndex })}
      />
      <button type="button" className="of-ed-btn of-ed-btn--danger" onClick={() => dispatch({ type: "removeTransition", index })}>
        Delete transition
      </button>
    </section>
  );
}
```

Create `web/apps/admin/src/editors/workflow/WorkflowFieldInspector.tsx`:
```tsx
import type { Dispatch } from "react";
import { SelectControl, TextControl } from "../shared/controls";
import { OptionsEditor } from "../shared/OptionsEditor";
import { problemsAt } from "../shared/problems";
import { FIELD_TYPE_LABELS, WORKFLOW_FIELD_TYPES, type FieldType, type Problem, type WorkflowDef } from "../shared/types";
import type { WorkflowEditorAction } from "./workflowReducer";

export function WorkflowFieldInspector({
  def,
  index,
  problems,
  dispatch,
}: {
  def: WorkflowDef;
  index: number;
  problems: Problem[];
  dispatch: Dispatch<WorkflowEditorAction>;
}) {
  const field = def.fields?.[index];
  if (!field) return null;
  const base = `fields[${index}]`;
  const at = (prop: string) => problemsAt(problems, `${base}.${prop}`);
  return (
    <section className="of-ed-inspector" aria-label="Workflow field settings">
      <h2>Field: {field.label || field.key}</h2>
      <p className="of-ed-hint">Workflow fields are filled in by reviewers, not respondents.</p>
      <TextControl label="Label" value={field.label} problems={at("label")} onChange={(v) => dispatch({ type: "updateField", index, patch: { label: v } })} />
      <TextControl
        label="Key"
        monospace
        value={field.key}
        problems={at("key")}
        hint="Renaming updates transitions that require this field."
        onChange={(v) => dispatch({ type: "renameField", index, key: v })}
      />
      <SelectControl
        label="Type"
        value={field.type}
        options={WORKFLOW_FIELD_TYPES.map((t) => ({ value: t, label: FIELD_TYPE_LABELS[t] }))}
        problems={at("type")}
        onChange={(t) => dispatch({ type: "updateField", index, patch: { type: t as FieldType } })}
      />
      {field.type === "select" ? (
        <OptionsEditor
          options={field.options ?? []}
          basePath={`${base}.options`}
          problems={problems}
          onChange={(options) => dispatch({ type: "updateField", index, patch: { options } })}
        />
      ) : null}
      <button type="button" className="of-ed-btn of-ed-btn--danger" onClick={() => dispatch({ type: "removeField", index })}>
        Delete field
      </button>
    </section>
  );
}
```

Create `web/apps/admin/src/editors/workflow/WorkflowMetaPanel.tsx`:
```tsx
import type { Dispatch } from "react";
import { SelectControl, TextControl } from "../shared/controls";
import { problemsAt } from "../shared/problems";
import type { Problem, WorkflowDef } from "../shared/types";
import { ActionsEditor } from "./ActionsEditor";
import type { WorkflowEditorAction } from "./workflowReducer";

export function WorkflowMetaPanel({
  def,
  isNew,
  problems,
  dispatch,
}: {
  def: WorkflowDef;
  isNew: boolean;
  problems: Problem[];
  dispatch: Dispatch<WorkflowEditorAction>;
}) {
  const at = (path: string) => problemsAt(problems, path);
  const target = { kind: "onSubmit" as const };
  const initialOptions = def.states.map((s) => ({ value: s.key, label: `${s.label} (${s.key})` }));
  if (!def.states.some((s) => s.key === def.initial)) initialOptions.unshift({ value: def.initial, label: `${def.initial || "(none)"} (unknown state)` });

  return (
    <section className="of-ed-inspector" aria-label="Workflow settings">
      <h2>Workflow settings</h2>
      <TextControl
        label="Slug"
        monospace
        value={def.slug}
        readOnly={!isNew}
        problems={at("slug")}
        hint={isNew ? "Forms reference the workflow by this slug. It can't be changed later." : "The slug can't be changed after the workflow is created."}
        onChange={(v) => dispatch({ type: "setMeta", patch: { slug: v } })}
      />
      <TextControl label="Title" value={def.title} problems={at("title")} onChange={(v) => dispatch({ type: "setMeta", patch: { title: v } })} />
      <SelectControl label="Initial state" value={def.initial} options={initialOptions} problems={at("initial")} onChange={(key) => dispatch({ type: "setInitial", key })} />
      <ActionsEditor
        title="Actions when a submission is created"
        actions={def.onSubmit ?? []}
        basePath="onSubmit"
        problems={problems}
        onAdd={(actionType) => dispatch({ type: "addAction", target, actionType })}
        onUpdate={(actionIndex, patch) => dispatch({ type: "updateAction", target, actionIndex, patch })}
        onRemove={(actionIndex) => dispatch({ type: "removeAction", target, actionIndex })}
      />
    </section>
  );
}
```

Create `web/apps/admin/src/editors/workflow/WorkflowOutline.tsx`:
```tsx
import type { Dispatch } from "react";
import { problemsUnder } from "../shared/problems";
import type { Problem, WorkflowDef } from "../shared/types";
import type { WorkflowEditorAction, WorkflowSelection } from "./workflowReducer";

function Badge({ problems, prefix }: { problems: Problem[]; prefix: string }) {
  const n = problemsUnder(problems, prefix).length;
  if (n === 0) return null;
  return (
    <span className="of-ed-badge" aria-label={n === 1 ? "1 problem" : `${n} problems`}>
      {n}
    </span>
  );
}

export function WorkflowOutline({
  def,
  selection,
  problems,
  dispatch,
}: {
  def: WorkflowDef;
  selection: WorkflowSelection;
  problems: Problem[];
  dispatch: Dispatch<WorkflowEditorAction>;
}) {
  const current = (kind: WorkflowSelection["kind"], index?: number) =>
    selection.kind === kind && (index === undefined || ("index" in selection && selection.index === index)) ? "true" : undefined;
  const select = (next: WorkflowSelection) => dispatch({ type: "select", selection: next });
  const settingsProblems = problems.filter((p) => !/^(states|transitions|fields)\[/.test(p.path)).length;

  return (
    <nav className="of-ed-list" aria-label="Workflow outline">
      <button type="button" className="of-ed-list__item" aria-current={current("workflow")} onClick={() => select({ kind: "workflow" })}>
        <span>
          Workflow settings
          {settingsProblems > 0 ? (
            <span className="of-ed-badge" aria-label={settingsProblems === 1 ? "1 problem" : `${settingsProblems} problems`}>
              {settingsProblems}
            </span>
          ) : null}
        </span>
      </button>

      <h3>States</h3>
      <ul>
        {def.states.map((s, i) => (
          <li key={i}>
            <button type="button" className="of-ed-list__item" aria-current={current("state", i)} onClick={() => select({ kind: "state", index: i })}>
              <span>
                <span className={`of-wf-dot of-wf-dot--${s.color ?? "gray"}`} aria-hidden="true" />
                {s.label || s.key}
                <Badge problems={problems} prefix={`states[${i}]`} />
              </span>
              <small>
                {s.key}
                {def.initial === s.key ? " · initial" : ""}
                {s.terminal ? " · terminal" : ""}
              </small>
            </button>
          </li>
        ))}
      </ul>
      <button type="button" className="of-ed-btn" onClick={() => dispatch({ type: "addState" })}>
        Add state
      </button>

      <h3>Transitions</h3>
      <ul>
        {def.transitions.map((t, i) => (
          <li key={i}>
            <button type="button" className="of-ed-list__item" aria-current={current("transition", i)} onClick={() => select({ kind: "transition", index: i })}>
              <span>
                {t.label || t.key}
                <Badge problems={problems} prefix={`transitions[${i}]`} />
              </span>
              <small>
                {t.from.join(", ")} → {t.to}
              </small>
            </button>
          </li>
        ))}
      </ul>
      <button type="button" className="of-ed-btn" onClick={() => dispatch({ type: "addTransition" })} disabled={def.states.length === 0}>
        Add transition
      </button>

      <h3>Workflow fields</h3>
      <ul>
        {(def.fields ?? []).map((f, i) => (
          <li key={i}>
            <button type="button" className="of-ed-list__item" aria-current={current("field", i)} onClick={() => select({ kind: "field", index: i })}>
              <span>
                {f.label || f.key}
                <Badge problems={problems} prefix={`fields[${i}]`} />
              </span>
              <small>{f.key}</small>
            </button>
          </li>
        ))}
      </ul>
      <button type="button" className="of-ed-btn" onClick={() => dispatch({ type: "addField" })}>
        Add field
      </button>
    </nav>
  );
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `pnpm -C web/apps/admin exec vitest run src/editors/workflow/workflow-components.test.tsx`
Expected: PASS, `Tests  10 passed`.

- [ ] **Step 6: Commit**

```bash
git add web/apps/admin/src/editors/workflow/ActionsEditor.tsx web/apps/admin/src/editors/workflow/StateInspector.tsx web/apps/admin/src/editors/workflow/TransitionInspector.tsx web/apps/admin/src/editors/workflow/WorkflowFieldInspector.tsx web/apps/admin/src/editors/workflow/WorkflowMetaPanel.tsx web/apps/admin/src/editors/workflow/WorkflowOutline.tsx web/apps/admin/src/editors/workflow/workflow-components.test.tsx
git commit -m "feat(admin): workflow outline, state/transition/field inspectors and action editor"
```

---

### Task 11: Workflow editor page

**Parallel group:** P8-B (workflow lane, step 4 of 4).

**Files:**
- Create: `web/apps/admin/src/editors/workflow/WorkflowEditorPage.tsx`
- Test: `web/apps/admin/src/editors/workflow/WorkflowEditorPage.test.tsx`

**Interfaces:**
- Consumes: Tasks 2–4 and 8–10, Plan 07's `renderWithProviders` / `server`, and `workflowRecordJson`, `applyItem`.
- Produces: `WorkflowEditorPage()`. It reads `:slug`; with no slug it creates a new workflow. On save it navigates to `/workflows/<slug>`.

- [ ] **Step 1: Write the failing tests**

Create `web/apps/admin/src/editors/workflow/WorkflowEditorPage.test.tsx`:
```tsx
import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { describe, expect, it, vi } from "vitest";
import { server } from "../../test/server";
import { renderWithProviders } from "../../test/render";
import { WorkflowEditorPage } from "./WorkflowEditorPage";
import { applyItem, workflowRecordJson } from "../test/records";
import { sampleWorkflow } from "../test/samples";
import type { WorkflowDef } from "../shared/types";

vi.mock("@xyflow/react", () => import("../test/xyflowMock"));

function mockApi({ source = "ui" }: { source?: string } = {}) {
  const puts: { url: string; body: WorkflowDef }[] = [];
  server.use(
    http.get("/api/v1/workflows/hiring", () => HttpResponse.json({ workflow: workflowRecordJson(sampleWorkflow(), { source }) })),
    http.put("/api/v1/workflows/:slug", async ({ request }) => {
      puts.push({ url: request.url, body: (await request.json()) as WorkflowDef });
      return HttpResponse.json({ item: applyItem("workflow", "hiring", 4) });
    }),
  );
  return puts;
}

const renderEdit = () => renderWithProviders(<WorkflowEditorPage />, { route: "/workflows/hiring/edit", path: "/workflows/:slug/edit" });
const outline = () => screen.getByRole("navigation", { name: "Workflow outline" });

describe("WorkflowEditorPage", () => {
  it("loads the workflow and shows the CLI banner when code-managed", async () => {
    mockApi({ source: "cli" });
    renderEdit();
    expect(await screen.findByRole("heading", { name: "Edit workflow: Hiring pipeline" })).toBeInTheDocument();
    expect(screen.getByRole("note")).toHaveTextContent("This workflow is managed in code.");
  });

  it("selects a state from the diagram", async () => {
    mockApi();
    const user = userEvent.setup();
    renderEdit();
    await user.click(await screen.findByRole("button", { name: "node hired" }));
    expect(screen.getByRole("heading", { name: "State: Hired" })).toBeInTheDocument();
  });

  it("renames a state and saves the updated transitions with source=ui", async () => {
    const puts = mockApi();
    const user = userEvent.setup();
    renderEdit();
    await screen.findByRole("heading", { name: "Edit workflow: Hiring pipeline" });
    await user.click(within(outline()).getByRole("button", { name: /^Screening/ }));
    const key = within(screen.getByRole("region", { name: "State settings" })).getByLabelText("Key");
    await user.clear(key);
    await user.type(key, "review");
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(puts).toHaveLength(1));
    expect(new URL(puts[0].url).searchParams.get("source")).toBe("ui");
    expect(puts[0].body.states[1].key).toBe("review");
    expect(puts[0].body.transitions[0].to).toBe("review");
  });

  it("maps server problems to the transition and lists unmapped ones", async () => {
    mockApi();
    server.use(
      http.put("/api/v1/workflows/:slug", () =>
        HttpResponse.json(
          {
            error: {
              code: "validation_failed",
              message: "invalid",
              details: [
                { path: "transitions[1].guard.requireFields[0]", message: "server rejects field" },
                { path: "something.else", message: "unmapped server rule" },
              ],
            },
          },
          { status: 422 },
        ),
      ),
    );
    const user = userEvent.setup();
    renderEdit();
    await user.click(await screen.findByRole("button", { name: "Save" }));
    const list = await screen.findByRole("region", { name: "Problems" });
    expect(within(list).getByText("unmapped server rule")).toBeInTheDocument();
    await user.click(within(list).getByRole("button", { name: /server rejects field/ }));
    expect(screen.getByRole("heading", { name: "Transition: Hire" })).toBeInTheDocument();
  });

  it("refuses to create a new workflow with a taken slug", async () => {
    const puts = mockApi();
    const user = userEvent.setup();
    renderWithProviders(<WorkflowEditorPage />, { route: "/workflows/new", path: "/workflows/new" });
    expect(await screen.findByRole("heading", { name: "New workflow" })).toBeInTheDocument();
    await user.type(within(screen.getByRole("region", { name: "Workflow settings" })).getByLabelText("Slug"), "hiring");
    await user.click(screen.getByRole("button", { name: "Save" }));
    expect(await screen.findByText(/already exists/)).toBeInTheDocument();
    expect(puts).toHaveLength(0);
  });

  it("survives invalid YAML", async () => {
    mockApi();
    const user = userEvent.setup();
    renderEdit();
    await screen.findByRole("heading", { name: "Edit workflow: Hiring pipeline" });
    await user.click(screen.getByRole("tab", { name: "YAML" }));
    fireEvent.change(screen.getByLabelText("YAML"), { target: { value: "states: [\n" } });
    expect(screen.getByRole("alert")).toBeInTheDocument();
    await user.click(screen.getByRole("tab", { name: "Visual" }));
    expect(within(outline()).getByRole("button", { name: /^Screening/ })).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `pnpm -C web/apps/admin exec vitest run src/editors/workflow/WorkflowEditorPage.test.tsx`
Expected: FAIL with `Failed to resolve import "./WorkflowEditorPage"`.

- [ ] **Step 3: Implement the page**

Create `web/apps/admin/src/editors/workflow/WorkflowEditorPage.tsx`:
```tsx
import { useEffect, useMemo, useReducer, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { OpenFormsError } from "@openforms/sdk";
import { editorKeys, loadWorkflow } from "../api";
import { CodeManagedBanner } from "../shared/CodeManagedBanner";
import { EditorLayout, type EditorTab } from "../shared/EditorLayout";
import { indexFromPath, mergeProblems } from "../shared/problems";
import { useDefinitionSave } from "../shared/useDefinitionSave";
import { useUnsavedChangesGuard } from "../shared/useUnsavedChangesGuard";
import { validateWorkflow } from "../shared/validation";
import { YamlPane } from "../shared/YamlPane";
import type { Problem, Source, WorkflowDef } from "../shared/types";
import { StateInspector } from "./StateInspector";
import { TransitionInspector } from "./TransitionInspector";
import { WorkflowDiagram } from "./WorkflowDiagram";
import { WorkflowFieldInspector } from "./WorkflowFieldInspector";
import { WorkflowMetaPanel } from "./WorkflowMetaPanel";
import { WorkflowOutline } from "./WorkflowOutline";
import { initWorkflowEditor, newWorkflowDefinition, workflowEditorReducer, type WorkflowSelection } from "./workflowReducer";
import "./workflow-editor.css";

export function WorkflowEditorPage() {
  const { slug } = useParams<{ slug: string }>();
  const loaded = useQuery({
    queryKey: editorKeys.definition("workflow", slug ?? ""),
    queryFn: () => loadWorkflow(slug!),
    enabled: Boolean(slug),
  });

  if (slug && loaded.isPending) return <p className="of-ed-hint">Loading workflow…</p>;
  if (slug && loaded.isError) {
    const notFound = loaded.error instanceof OpenFormsError && loaded.error.status === 404;
    return (
      <p role="alert" className="of-ed-alert">
        {notFound ? `Workflow "${slug}" was not found.` : `Could not load the workflow: ${loaded.error.message}`}
      </p>
    );
  }
  return (
    <WorkflowEditor
      key={slug ?? "new"}
      initial={loaded.data?.definition ?? newWorkflowDefinition()}
      source={loaded.data?.source}
      isNew={!slug}
    />
  );
}

function selectionForProblem(problem: Problem): WorkflowSelection {
  const state = indexFromPath(problem.path, "states");
  if (state !== null) return { kind: "state", index: state };
  const transition = indexFromPath(problem.path, "transitions");
  if (transition !== null) return { kind: "transition", index: transition };
  const field = indexFromPath(problem.path, "fields");
  if (field !== null) return { kind: "field", index: field };
  return { kind: "workflow" };
}

function WorkflowEditor({ initial, source, isNew }: { initial: WorkflowDef; source?: Source; isNew: boolean }) {
  const navigate = useNavigate();
  const [state, dispatch] = useReducer(workflowEditorReducer, initial, initWorkflowEditor);
  const [tab, setTab] = useState<EditorTab>("visual");
  const [blocked, setBlocked] = useState<string | null>(null);
  const { save, saving, serverProblems, error, clearServerProblems } = useDefinitionSave("workflow");

  const clientProblems = useMemo(() => validateWorkflow(state.def), [state.def]);
  const problems = useMemo(() => mergeProblems(clientProblems, serverProblems), [clientProblems, serverProblems]);

  useEffect(() => {
    clearServerProblems();
    setBlocked(null);
  }, [state.def, clearServerProblems]);

  useUnsavedChangesGuard(state.dirty);

  const onSave = () => {
    if (clientProblems.length > 0) {
      const n = clientProblems.length;
      setBlocked(`Fix ${n} ${n === 1 ? "problem" : "problems"} before saving.`);
      return;
    }
    const slug = state.def.slug;
    save(state.def, isNew, () => {
      dispatch({ type: "markSaved" });
      navigate(`/workflows/${slug}`);
    });
  };

  const select = (selection: WorkflowSelection) => dispatch({ type: "select", selection });
  const sel = state.selection;

  return (
    <EditorLayout
      title={isNew ? "New workflow" : `Edit workflow: ${state.def.title || state.def.slug}`}
      tab={tab}
      onTabChange={setTab}
      onSave={onSave}
      saving={saving}
      dirty={state.dirty}
      banner={<CodeManagedBanner kind="workflow" source={source} />}
      problems={problems}
      onSelectProblem={(p) => {
        setTab("visual");
        select(selectionForProblem(p));
      }}
      error={error}
      saveBlockedMessage={blocked}
    >
      {tab === "visual" ? (
        <div className="of-we-grid">
          <WorkflowOutline def={state.def} selection={sel} problems={problems} dispatch={dispatch} />
          <WorkflowDiagram def={state.def} selection={sel} onSelect={select} />
          <div>
            {sel.kind === "workflow" ? <WorkflowMetaPanel def={state.def} isNew={isNew} problems={problems} dispatch={dispatch} /> : null}
            {sel.kind === "state" ? <StateInspector def={state.def} index={sel.index} problems={problems} dispatch={dispatch} /> : null}
            {sel.kind === "transition" ? <TransitionInspector def={state.def} index={sel.index} problems={problems} dispatch={dispatch} /> : null}
            {sel.kind === "field" ? <WorkflowFieldInspector def={state.def} index={sel.index} problems={problems} dispatch={dispatch} /> : null}
          </div>
        </div>
      ) : (
        <YamlPane value={state.def} onApply={(next) => dispatch({ type: "replace", def: next as unknown as WorkflowDef })} />
      )}
    </EditorLayout>
  );
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `pnpm -C web/apps/admin exec vitest run src/editors/workflow`
Expected: PASS, `Test Files  5 passed`, `Tests  50 passed`.

- [ ] **Step 5: Commit**

```bash
git add web/apps/admin/src/editors/workflow/WorkflowEditorPage.tsx web/apps/admin/src/editors/workflow/WorkflowEditorPage.test.tsx
git commit -m "feat(admin): workflow editor page with diagram, inspectors, YAML and save flow"
```

---

### Task 12: Version history, side-by-side diff and restore (OverviewExtras)

**Parallel group:** P8-C (history lane, single task). Runs concurrently with P8-A and P8-B.

**Files:**
- Create: `web/apps/admin/src/editors/history/lineDiff.ts`
- Create: `web/apps/admin/src/editors/history/DiffView.tsx`
- Create: `web/apps/admin/src/editors/history/VersionHistory.tsx`
- Create: `web/apps/admin/src/editors/history/history.css`
- Modify (replace body): `web/apps/admin/src/extensions/OverviewExtras.tsx`
- Test: `web/apps/admin/src/editors/history/lineDiff.test.tsx`, `web/apps/admin/src/editors/history/VersionHistory.test.tsx`

**Interfaces:**
- Consumes:
  - Task 4: `editorKeys`, `listVersions`, `loadVersion`, `saveDefinition`, `invalidateDefinition`, `useIsAdmin`, `formRecordJson`, `versionList`, `applyItem`
  - Task 1: `toYaml`
  - Plan 07: `makePrincipal`, `renderWithProviders`, `server`
- Produces:
  - `DiffRow = { kind: "same" | "removed" | "added" | "changed"; left: {n,text} | null; right: {n,text} | null }`
  - `sideBySide(a, b): DiffRow[]`
  - `DiffView({ left, right, leftTitle, rightTitle })`: a table named "Differences between <leftTitle> and <rightTitle>"
  - `VersionHistory({ kind, slug })`
  - `OverviewExtras({ kind, slug })`, which now renders `VersionHistory`

- [ ] **Step 1: Write the failing tests**

Create `web/apps/admin/src/editors/history/lineDiff.test.tsx`:
```tsx
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { sideBySide } from "./lineDiff";
import { DiffView } from "./DiffView";

describe("sideBySide", () => {
  it("marks identical text as same with matching line numbers", () => {
    expect(sideBySide("a\nb\n", "a\nb\n")).toEqual([
      { kind: "same", left: { n: 1, text: "a" }, right: { n: 1, text: "a" } },
      { kind: "same", left: { n: 2, text: "b" }, right: { n: 2, text: "b" } },
    ]);
  });

  it("pairs a replaced line", () => {
    const rows = sideBySide("a\nb\nc\n", "a\nB\nc\n");
    expect(rows[1]).toEqual({ kind: "changed", left: { n: 2, text: "b" }, right: { n: 2, text: "B" } });
    expect(rows[2]).toEqual({ kind: "same", left: { n: 3, text: "c" }, right: { n: 3, text: "c" } });
  });

  it("shows added lines only on the right", () => {
    expect(sideBySide("a\n", "a\nb\n")).toEqual([
      { kind: "same", left: { n: 1, text: "a" }, right: { n: 1, text: "a" } },
      { kind: "added", left: null, right: { n: 2, text: "b" } },
    ]);
  });

  it("shows removed lines only on the left", () => {
    expect(sideBySide("a\nb\n", "a\n")).toEqual([
      { kind: "same", left: { n: 1, text: "a" }, right: { n: 1, text: "a" } },
      { kind: "removed", left: { n: 2, text: "b" }, right: null },
    ]);
  });

  it("pads uneven replacements", () => {
    expect(sideBySide("a\nx\n", "a\ny\nz\n").slice(1)).toEqual([
      { kind: "changed", left: { n: 2, text: "x" }, right: { n: 2, text: "y" } },
      { kind: "changed", left: null, right: { n: 3, text: "z" } },
    ]);
  });
});

describe("DiffView", () => {
  it("labels the table and notes identical content (e.g. a workflow re-pin)", () => {
    render(<DiffView left={"slug: a\n"} right={"slug: a\n"} leftTitle="v1" rightTitle="v2" />);
    expect(screen.getByRole("table", { name: "Differences between v1 and v2" })).toBeInTheDocument();
    expect(screen.getByText("These versions have identical content.")).toBeInTheDocument();
  });
});
```

Create `web/apps/admin/src/editors/history/VersionHistory.test.tsx`:
```tsx
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, describe, expect, it, vi } from "vitest";
import { server } from "../../test/server";
import { renderWithProviders } from "../../test/render";
import { makePrincipal } from "../../test/fixtures";
import { OverviewExtras } from "../../extensions/OverviewExtras";
import { applyItem, formRecordJson, versionList } from "../test/records";
import { sampleForm } from "../test/samples";

afterEach(() => vi.restoreAllMocks());

const defV = (n: number) => ({ ...sampleForm(), title: `Job application v${n}` });

function mockVersions(versions: number[], roles: string[] = ["admin"]) {
  const puts: { url: string; body: { title: string } }[] = [];
  server.use(
    http.get("/api/v1/auth/me", () => HttpResponse.json({ principal: makePrincipal({ roles }) })),
    http.get("/api/v1/forms/job-application/versions", () => HttpResponse.json({ items: versionList(versions) })),
    http.get("/api/v1/forms/job-application/versions/:n", ({ params }) =>
      HttpResponse.json({ form: formRecordJson(defV(Number(params.n)), { version: Number(params.n) }) }),
    ),
    http.put("/api/v1/forms/job-application", async ({ request }) => {
      puts.push({ url: request.url, body: (await request.json()) as { title: string } });
      return HttpResponse.json({ item: applyItem("form", "job-application", Math.max(...versions) + 1) });
    }),
  );
  return puts;
}

const renderHistory = () => renderWithProviders(<OverviewExtras kind="form" slug="job-application" />);

describe("VersionHistory via OverviewExtras", () => {
  it("compares the two latest versions by default", async () => {
    mockVersions([3, 2, 1]);
    renderHistory();
    const diff = await screen.findByRole("table", { name: "Differences between v2 and v3" });
    expect(within(diff).getByText("title: Job application v2")).toBeInTheDocument();
    expect(within(diff).getByText("title: Job application v3")).toBeInTheDocument();
  });

  it("switches the comparison when another version is ticked", async () => {
    mockVersions([3, 2, 1]);
    const user = userEvent.setup();
    renderHistory();
    await user.click(await screen.findByRole("checkbox", { name: "Compare version 1" }));
    expect(await screen.findByRole("table", { name: "Differences between v1 and v2" })).toBeInTheDocument();
  });

  it("restores an older version as a new UI version", async () => {
    const puts = mockVersions([3, 2, 1]);
    vi.spyOn(window, "confirm").mockReturnValue(true);
    const user = userEvent.setup();
    renderHistory();
    await user.click(await screen.findByRole("button", { name: "Restore version 1" }));
    await waitFor(() => expect(puts).toHaveLength(1));
    expect(puts[0].body.title).toBe("Job application v1");
    expect(new URL(puts[0].url).searchParams.get("source")).toBe("ui");
    expect(await screen.findByText("Restored as version 4.")).toBeInTheDocument();
  });

  it("does not restore when the confirmation is declined", async () => {
    const puts = mockVersions([2, 1]);
    vi.spyOn(window, "confirm").mockReturnValue(false);
    const user = userEvent.setup();
    renderHistory();
    await user.click(await screen.findByRole("button", { name: "Restore version 1" }));
    expect(puts).toHaveLength(0);
  });

  it("hides restore for non-admins", async () => {
    mockVersions([3, 2, 1], ["reviewer"]);
    renderHistory();
    await screen.findByRole("checkbox", { name: "Compare version 3" });
    expect(screen.queryByRole("button", { name: /Restore version/ })).toBeNull();
  });

  it("explains when there is only one version", async () => {
    mockVersions([1]);
    renderHistory();
    expect(await screen.findByText(/Only one version so far/)).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `pnpm -C web/apps/admin exec vitest run src/editors/history`
Expected: FAIL with `Failed to resolve import "./lineDiff"`. The VersionHistory tests fail because `OverviewExtras` still returns `null`.

- [ ] **Step 3: Implement the diff**

Create `web/apps/admin/src/editors/history/lineDiff.ts`:
```ts
import { diffLines } from "diff";

export type DiffCell = { n: number; text: string };
export type DiffRow = { kind: "same" | "removed" | "added" | "changed"; left: DiffCell | null; right: DiffCell | null };

function lines(value: string): string[] {
  const out = value.split("\n");
  if (out[out.length - 1] === "") out.pop();
  return out;
}

export function sideBySide(a: string, b: string): DiffRow[] {
  const parts = diffLines(a, b);
  const rows: DiffRow[] = [];
  let leftN = 1;
  let rightN = 1;
  for (let i = 0; i < parts.length; i++) {
    const part = parts[i];
    if (!part.added && !part.removed) {
      for (const text of lines(part.value)) rows.push({ kind: "same", left: { n: leftN++, text }, right: { n: rightN++, text } });
      continue;
    }
    if (part.removed && parts[i + 1]?.added) {
      const left = lines(part.value);
      const right = lines(parts[i + 1].value);
      const size = Math.max(left.length, right.length);
      for (let k = 0; k < size; k++) {
        rows.push({
          kind: "changed",
          left: k < left.length ? { n: leftN++, text: left[k] } : null,
          right: k < right.length ? { n: rightN++, text: right[k] } : null,
        });
      }
      i++;
      continue;
    }
    if (part.removed) {
      for (const text of lines(part.value)) rows.push({ kind: "removed", left: { n: leftN++, text }, right: null });
    } else {
      for (const text of lines(part.value)) rows.push({ kind: "added", left: null, right: { n: rightN++, text } });
    }
  }
  return rows;
}
```

Create `web/apps/admin/src/editors/history/DiffView.tsx`:
```tsx
import { useMemo } from "react";
import { sideBySide } from "./lineDiff";

export function DiffView({ left, right, leftTitle, rightTitle }: { left: string; right: string; leftTitle: string; rightTitle: string }) {
  const rows = useMemo(() => sideBySide(left, right), [left, right]);
  const identical = rows.every((r) => r.kind === "same");
  return (
    <div className="of-vh-diff">
      {identical ? <p className="of-ed-hint">These versions have identical content.</p> : null}
      <table aria-label={`Differences between ${leftTitle} and ${rightTitle}`}>
        <thead>
          <tr>
            <th colSpan={2}>{leftTitle}</th>
            <th colSpan={2}>{rightTitle}</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((row, i) => (
            <tr key={i} className={`of-vh-row of-vh-row--${row.kind}`}>
              <td className="of-vh-ln">{row.left?.n ?? ""}</td>
              <td className="of-vh-code of-vh-code--left">{row.left?.text ?? ""}</td>
              <td className="of-vh-ln">{row.right?.n ?? ""}</td>
              <td className="of-vh-code of-vh-code--right">{row.right?.text ?? ""}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
```

- [ ] **Step 4: Implement VersionHistory, its CSS and the OverviewExtras body**

Create `web/apps/admin/src/editors/history/VersionHistory.tsx`:
```tsx
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { editorKeys, invalidateDefinition, listVersions, loadVersion, saveDefinition, useIsAdmin } from "../api";
import { toYaml } from "../shared/yaml";
import { SOURCE_LABELS, type DefinitionKind } from "../shared/types";
import { DiffView } from "./DiffView";
import "../shared/editors.css";
import "./history.css";

export function VersionHistory({ kind, slug }: { kind: DefinitionKind; slug: string }) {
  const qc = useQueryClient();
  const isAdmin = useIsAdmin();
  const [picked, setPicked] = useState<number[] | null>(null);
  const [status, setStatus] = useState<string | null>(null);

  const versions = useQuery({ queryKey: editorKeys.versions(kind, slug), queryFn: () => listVersions(kind, slug) });
  const list = versions.data ?? [];
  const selection = picked ?? list.slice(0, 2).map((v) => v.version);
  const older = selection.length === 2 ? Math.min(...selection) : undefined;
  const newer = selection.length === 2 ? Math.max(...selection) : undefined;

  // Versions are immutable (spec §5.5), so they never go stale.
  const olderQuery = useQuery({
    queryKey: editorKeys.version(kind, slug, older ?? 0),
    queryFn: () => loadVersion(kind, slug, older!),
    enabled: older !== undefined,
    staleTime: Infinity,
  });
  const newerQuery = useQuery({
    queryKey: editorKeys.version(kind, slug, newer ?? 0),
    queryFn: () => loadVersion(kind, slug, newer!),
    enabled: newer !== undefined,
    staleTime: Infinity,
  });

  const restore = useMutation({
    mutationFn: async (version: number) => {
      const def = await qc.fetchQuery({
        queryKey: editorKeys.version(kind, slug, version),
        queryFn: () => loadVersion(kind, slug, version),
        staleTime: Infinity,
      });
      return saveDefinition(kind, def);
    },
    onSuccess: async (result) => {
      setStatus(result.changed ? `Restored as version ${result.version}.` : "That version matches the current one, so nothing changed.");
      setPicked(null);
      await invalidateDefinition(qc, kind, slug);
    },
    onError: (err) => setStatus(`Restore failed: ${err instanceof Error ? err.message : String(err)}`),
  });

  const toggle = (version: number, on: boolean) =>
    setPicked((prev) => {
      const current = prev ?? selection;
      if (!on) return current.filter((v) => v !== version);
      const next = [...current.filter((v) => v !== version), version];
      return next.length > 2 ? next.slice(next.length - 2) : next;
    });

  if (versions.isPending) return <p className="of-ed-hint">Loading version history…</p>;
  if (versions.isError) {
    return (
      <p role="alert" className="of-ed-alert">
        Could not load version history: {versions.error.message}
      </p>
    );
  }

  return (
    <section className="of-vh" aria-labelledby={`vh-${kind}-${slug}`}>
      <h2 id={`vh-${kind}-${slug}`}>Version history</h2>
      {status ? <p role="status">{status}</p> : null}
      <table className="of-vh-versions">
        <caption className="sr-only">Versions of {slug}</caption>
        <thead>
          <tr>
            <th>Compare</th>
            <th>Version</th>
            <th>Source</th>
            <th>By</th>
            <th>Created</th>
            {isAdmin ? (
              <th>
                <span className="sr-only">Actions</span>
              </th>
            ) : null}
          </tr>
        </thead>
        <tbody>
          {list.map((v, i) => (
            <tr key={v.version}>
              <td>
                <input
                  type="checkbox"
                  aria-label={`Compare version ${v.version}`}
                  checked={selection.includes(v.version)}
                  onChange={(e) => toggle(v.version, e.target.checked)}
                />
              </td>
              <td>
                v{v.version}
                {i === 0 ? <span className="of-vh-current"> current</span> : null}
              </td>
              <td>
                <span className={`of-vh-source of-vh-source--${v.source}`}>{SOURCE_LABELS[v.source] ?? v.source}</span>
              </td>
              <td>{v.createdBy || "—"}</td>
              <td>
                <time dateTime={v.createdAt}>{new Date(v.createdAt).toLocaleString()}</time>
              </td>
              {isAdmin ? (
                <td>
                  {i > 0 ? (
                    <button
                      type="button"
                      className="of-ed-btn"
                      disabled={restore.isPending}
                      onClick={() => {
                        if (window.confirm(`Restore version ${v.version}? Its content will be saved as a new version.`)) restore.mutate(v.version);
                      }}
                    >
                      Restore version {v.version}
                    </button>
                  ) : null}
                </td>
              ) : null}
            </tr>
          ))}
        </tbody>
      </table>
      {list.length < 2 ? (
        <p className="of-ed-hint">Only one version so far. A new version appears each time the {kind} changes.</p>
      ) : older === undefined || newer === undefined ? (
        <p className="of-ed-hint">Select two versions to compare.</p>
      ) : olderQuery.data && newerQuery.data ? (
        <DiffView leftTitle={`v${older}`} rightTitle={`v${newer}`} left={toYaml(olderQuery.data)} right={toYaml(newerQuery.data)} />
      ) : olderQuery.isError || newerQuery.isError ? (
        <p role="alert" className="of-ed-alert">
          Could not load one of the versions to compare.
        </p>
      ) : (
        <p className="of-ed-hint">Loading comparison…</p>
      )}
    </section>
  );
}
```

Create `web/apps/admin/src/editors/history/history.css`:
```css
.of-vh { margin-top: 24px; display: flex; flex-direction: column; gap: 12px; }
.of-vh h2 { font-size: 1.0625rem; margin: 0; }
.of-vh-versions { border-collapse: collapse; width: 100%; font-size: 0.8125rem; }
.of-vh-versions th, .of-vh-versions td { text-align: left; padding: 6px 8px; border-bottom: 1px solid var(--color-border, #eaecf0); }
.of-vh-current { color: var(--color-muted, #667085); font-size: 0.75rem; }
.of-vh-source { padding: 1px 8px; border-radius: 999px; background: var(--color-muted-bg, #f2f4f7); font-size: 0.75rem; }
.of-vh-source--cli { background: var(--color-warning-bg, #fffaeb); color: var(--color-warning-text, #93370d); }
.of-vh-diff { overflow: auto; border: 1px solid var(--color-border, #d0d5dd); border-radius: 8px; }
.of-vh-diff table { border-collapse: collapse; width: 100%; font-family: var(--font-mono, ui-monospace, SFMono-Regular, Menlo, monospace); font-size: 0.75rem; }
.of-vh-diff th { position: sticky; top: 0; background: var(--color-surface, #fff); text-align: left; padding: 6px 8px; border-bottom: 1px solid var(--color-border, #d0d5dd); }
.of-vh-ln { width: 1%; padding: 0 8px; text-align: right; color: var(--color-muted, #98a2b3); user-select: none; }
.of-vh-code { white-space: pre; padding: 0 8px; width: 49%; }
.of-vh-row--removed .of-vh-code--left, .of-vh-row--changed .of-vh-code--left { background: var(--diff-removed, #fef3f2); }
.of-vh-row--added .of-vh-code--right, .of-vh-row--changed .of-vh-code--right { background: var(--diff-added, #ecfdf3); }
```

Replace the entire contents of `web/apps/admin/src/extensions/OverviewExtras.tsx` with:
```tsx
import { VersionHistory } from "../editors/history/VersionHistory";

// Extension point defined by Plan 07; Plan 08 renders the version history here.
export function OverviewExtras({ kind, slug }: { kind: "form" | "workflow"; slug: string }): JSX.Element | null {
  return <VersionHistory kind={kind} slug={slug} />;
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `pnpm -C web/apps/admin exec vitest run src/editors/history`
Expected: PASS, `Test Files  2 passed`, `Tests  12 passed`.

Then run Plan 07's overview page tests to check the new extras don't break them (they now make version requests):
Run: `pnpm -C web/apps/admin exec vitest run src/pages`
Expected: PASS. If an overview test fails with an MSW "unhandled request" error for `/versions`, add a default handler to Plan 07's `src/test/server.ts` default handlers:
```ts
http.get("/api/v1/forms/:slug/versions", () => HttpResponse.json({ items: [] })),
http.get("/api/v1/workflows/:slug/versions", () => HttpResponse.json({ items: [] })),
```
Then rerun until the tests pass.

- [ ] **Step 6: Commit**

```bash
git add web/apps/admin/src/editors/history web/apps/admin/src/extensions/OverviewExtras.tsx web/apps/admin/src/test/server.ts
git commit -m "feat(admin): version history with side-by-side YAML diff and restore"
```

---

### Task 13: Register editor routes and verify the whole admin app

**Sequential.** Runs after lanes P8-A, P8-B and P8-C are merged.

**Files:**
- Modify: `web/apps/admin/src/routes.tsx`
- Test: `web/apps/admin/src/editors/routes.test.ts`

**Interfaces:**
- Consumes: `FormEditorPage` (Task 7), `WorkflowEditorPage` (Task 11), and Plan 07's `routes: AdminRoute[]`.
- Produces: the admin-only routes `forms/new`, `forms/:slug/edit`, `workflows/new` and `workflows/:slug/edit`.

- [ ] **Step 1: Write the failing test**

Create `web/apps/admin/src/editors/routes.test.ts`:
```ts
import { describe, expect, it, vi } from "vitest";
import { routes } from "../routes";

vi.mock("@xyflow/react", () => import("./test/xyflowMock"));

describe("editor routes", () => {
  it.each(["forms/new", "forms/:slug/edit", "workflows/new", "workflows/:slug/edit"])("registers %s as admin-only", (path) => {
    const route = routes.find((r) => r.path === path);
    expect(route).toBeDefined();
    expect(route?.adminOnly).toBe(true);
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `pnpm -C web/apps/admin exec vitest run src/editors/routes.test.ts`
Expected: FAIL with `expected undefined to be defined` for each of the four paths.

- [ ] **Step 3: Append the routes**

In `web/apps/admin/src/routes.tsx`, add these imports next to the existing page imports:
```tsx
import { FormEditorPage } from "./editors/form/FormEditorPage";
import { WorkflowEditorPage } from "./editors/workflow/WorkflowEditorPage";
```
Then append these four entries at the end of the exported `routes` array. React Router ranks static segments above dynamic ones, so `forms/new` wins over `forms/:slug`:
```tsx
  { path: "forms/new", element: <FormEditorPage />, adminOnly: true },
  { path: "forms/:slug/edit", element: <FormEditorPage />, adminOnly: true },
  { path: "workflows/new", element: <WorkflowEditorPage />, adminOnly: true },
  { path: "workflows/:slug/edit", element: <WorkflowEditorPage />, adminOnly: true },
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `pnpm -C web/apps/admin exec vitest run src/editors/routes.test.ts`
Expected: PASS, `Tests  4 passed`.

- [ ] **Step 5: Run the full admin suite, the type check and the production build**

Run: `pnpm -C web/apps/admin exec vitest run`
Expected: PASS for every test file (Plan 07's tests plus all `src/editors/**` tests), with 0 failures.

Run: `pnpm -C web/apps/admin exec tsc --noEmit -p .` (use `-p tsconfig.app.json` if that is the app config)
Expected: exit code 0 and no output. If it reports `Could not find a declaration file for module 'diff'`, run `pnpm -C web/apps/admin add -D @types/diff` and rerun.

Run: `pnpm -C web build`
Expected: the build succeeds, and `internal/webui/dist/admin/index.html` exists (`ls internal/webui/dist/admin/index.html` prints the path).

- [ ] **Step 6: Manual smoke test against a real server**

Run in three terminals:
```bash
docker compose up -d postgres mailpit
go run ./cmd/openforms admin create-user --email admin@local.test --name Admin --password password123 --roles admin
OPENFORMS_DATABASE_URL="postgres://openforms:openforms@localhost:54329/openforms?sslmode=disable" go run ./cmd/openforms serve
pnpm -C web/apps/admin dev
```
(Run the `create-user` command with the same `OPENFORMS_DATABASE_URL` exported.) Open the dev server URL Vite prints, go to `/admin/login`, and sign in. Then check each of these:
1. Forms → **New form**. Set the slug to `smoke`, add a Dropdown field, and see it appear in Preview. Save. You land on the `smoke` overview.
2. **Edit**. Rename the dropdown's key, add a second field with "Only show this field when…", and save. The overview's **Version history** lists v2 and v1, and the diff shows the changes.
3. **Restore version 1**. A v3 appears with v1's content.
4. Workflows → **New workflow**. Slug `smoke-flow`: add a state, click it in the diagram, mark it terminal, and add a transition with a required workflow field and an email action. Save.
5. Run `openforms push` against this server for a form, then open that form's editor. The yellow "managed in code" banner is shown.
6. Edit anything, then click a nav link. The browser asks to discard unsaved changes.

- [ ] **Step 7: Commit**

```bash
git add web/apps/admin/src/routes.tsx web/apps/admin/src/editors/routes.test.ts web/apps/admin/package.json web/pnpm-lock.yaml
git commit -m "feat(admin): register visual editor routes"
```

---

## Self-Review Notes (for the executor)

- **Spec coverage (§9.5):**

  | Spec requirement | Covered by |
  |---|---|
  | Three-pane form editor | Tasks 6–7 |
  | Add, reorder, duplicate, delete fields | Tasks 5, 6 |
  | Inspector with options, validation, and a showIf builder restricted to earlier fields | Task 6 |
  | Live `<OpenForm definition>` preview | Task 6 |
  | Visual/YAML tabs | Tasks 3, 7, 11 |
  | ajv + semantic validation, with 422 details mapped by path | Tasks 2, 4, 7, 11 |
  | `PUT ?source=ui` | Task 4 |
  | CLI banner | Tasks 3, 7, 11 |
  | Workflow diagram (dagre LR, colors, double-bordered terminal nodes, labelled edges, click-to-select) | Task 9 |
  | Inspectors for states, transitions, fields, actions and onSubmit | Task 10 |
  | Version history: list, pick two, side-by-side diff, restore with `source=ui` | Task 12 |
  | Unsaved-changes guard | Task 3 |
  | Editor routes | Task 13 |

- **Cross-plan risk:** Plan 02 may emit problem paths for the same rule that differ slightly from the client port (e.g. `fields[1].showIf` vs `fields[1].showIf.field`). Nothing breaks: server problems always appear in the Problems list, and clicking one selects the item found by the `fields[n]` / `states[n]` / `transitions[n]` prefix.
