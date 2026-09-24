# openforms Plan 06 — SDK, React Renderer, Hosted Pages & Embed — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Tasks marked with the same **Parallel group** touch disjoint files (except where a task says "append one line to `index.ts`": on merge keep both lines) and may be dispatched to concurrent subagents via superpowers:dispatching-parallel-agents.

**Goal:** Build the `web/` pnpm workspace containing `@openforms/sdk` (generated definition types, a typed API client and shared submission logic), `@openforms/react` (headless hook, accessible `<OpenForm>` renderer and `<StatusTracker>`), the hosted app that serves `/f/:slug` and `/s/:id`, and `embed.js`.

**Architecture:** The SDK is framework-free TypeScript. Its definition types are generated from `schemas/*.schema.json`, and its `visible`/`validateSubmission` logic has to pass the same conformance fixtures as the Go `definition` package. `@openforms/react` builds `useOpenForm` on top of the SDK, then `<OpenForm>` and `<StatusTracker>` on top of the hook. The hosted app is a small React SPA with no router library that renders those components. `embed.js` is a dependency-free IIFE that injects iframes pointing at the hosted app and listens for `postMessage` events restricted by origin. Every HTTP interaction is mocked in tests (mocked `fetch` for the SDK, MSW for React), so this plan never needs the Go server running.

**Tech Stack:** TypeScript 5.7, Node 22 LTS, pnpm 9, React 18.3, Vite 6, Vitest 3, jsdom 26, @testing-library/react 16 + user-event 14 + jest-dom 6, MSW 2, json-schema-to-typescript 15.

**Spec:** `docs/superpowers/specs/2026-09-23-openforms-design.md`. The binding sections are §3 (web build contract), §5.4 (submission semantics and fixture format), §7 (HTTP API), and §9.1–§9.4 (frontend names). Read §9.1–§9.4 before starting any task.

## Global Constraints

- Node ≥ 22, pnpm 9 (`"packageManager": "pnpm@9.15.4"`), TypeScript `^5.7.3`, React `^18.3.1`, Vite `^6.1.0`, Vitest `^3.0.5`.
- Workspace root is `web/`. Members are `web/packages/*` and `web/apps/*` (`pnpm-workspace.yaml`).
- `web/package.json` scripts are exactly: `build` = `pnpm -r --filter "./packages/*" build && pnpm -r --filter "./apps/*" build && node scripts/copy-dist.mjs`; `test` = `pnpm -r test`; `typecheck` = `pnpm -r typecheck`.
- `scripts/copy-dist.mjs` wipes `internal/webui/dist/*` (keeping `.gitkeep`), copies every existing `apps/<name>/dist` to `internal/webui/dist/<name>`, and copies `packages/embed/dist/embed.js` to `internal/webui/dist/embed/embed.js`.
- Every Vite app sets `base: "/_app/<name>/"`. Dev servers proxy `/api` and `/healthz` to `http://localhost:8080`.
- All JSON on the wire is camelCase. Errors use the envelope `{"error": {"code", "message", "details"?}}` from §7.1.
- The Go server is authoritative for validation. TS validation exists for UX only and **must pass `schemas/fixtures/visibility.json` and `schemas/fixtures/submission.json` unchanged**. If a fixture disagrees with this plan's reading of §5.4, the fixture wins: change the TS code, never the fixture.
- Package names and exports follow §9.1–9.2 exactly: `OpenFormsClient`, `OpenFormsError`, `visible`, `validateSubmission`, `useOpenForm`, `OpenForm`, `StatusTracker`, and the default field components `TextInput, TextArea, EmailInput, NumberInput, Select, MultiSelect, Checkbox, DateInput, UrlInput`.
- Shipped UI copy contains no "Lorem ipsum" or other placeholder text.
- Run all commands from the repo root `D:\Github\openforms` in Git Bash. Commit after every task with conventional prefixes.

## Review Focus

These are the input classes most likely to break for a real user. Each one is pinned by a test in the task that owns the code.

1. **Server-side 422 after client validation passes.** Example: the server rejects an email domain. The problem details (`data.<key>` paths) must appear as field errors next to the right input, and the form must stay editable. Tested in Task 5 (`maps server validation errors onto fields`) and Task 6 (`shows server-side field errors`).
2. **Non-JSON error responses.** A proxy can answer with an HTML 502, and the network can drop. These must surface as `OpenFormsError` with `code: "http_error"` or `"network_error"`, never as an uncaught `SyntaxError`. Tested in Task 3.
3. **Values of fields that became hidden.** If a respondent fills `portfolio`, then switches `role` so `portfolio` hides, the stale value must not be submitted. Tested in Task 4 (logic) and Task 5 (`drops values of fields that became hidden`).
4. **Double submit.** A double-click or Enter pressed twice must create exactly one submission. Tested in Task 5 (`ignores a second submit while one is in flight`) and Task 6 (`double-clicking submit sends one request`).
5. **Hostile or malformed `postMessage`.** Messages from a foreign origin, messages whose `source` is not one of our iframes, and non-numeric or absurd heights must be ignored by `embed.js`. Tested in Task 8.

## Contract resolutions

Spec §7.2 leaves some response envelopes implicit. This plan fixes them as follows, and Plans 01 and 03 must match:

| Endpoint | Response body assumed |
|---|---|
| `POST /auth/login` | `{"user": User}` |
| `GET /users` · `POST /users` · `PATCH /users/{id}` | `{"items": [User]}` · `{"user": User}` · `{"user": User}` |
| `DELETE /users/{id}` · `DELETE /api-keys/{id}` · `POST /auth/logout` · `POST /jobs/{id}/retry` | `204` no body |
| `GET /api-keys` | `{"items": [ApiKey]}`, where ApiKey = `{id,name,prefix,roles,createdAt,lastUsedAt,revokedAt}` |
| `GET /workflows/{slug}` and `/versions/{n}` | `{"workflow": WorkflowRecord}` (mirrors forms' `{"form": FormRecord}`) |

These choices also apply:

- **Visibility.** An `equals`, `notEquals` or `in` whose value is `null` counts as absent, mirroring Go `omitempty` on `any`. A controller value that is "not provided" matches nothing: `equals` → false, `notEquals` → true, `in` → false. For a `multiselect` controller, `in` means "any selected value is in the list".
- **`pattern`** is tested as an unanchored search (Go `regexp.MatchString` semantics) with the JS `u` flag.
- **Error reporting.** Each field reports at most one problem, the first failing check.

If Plan 02's fixtures contradict any of these choices, follow the fixtures.

---

## File map

```
web/
  package.json                       Task 1  workspace scripts (contract)
  pnpm-workspace.yaml                Task 1
  tsconfig.base.json                 Task 1
  .gitignore                         Task 1
  README.md                          Task 10
  scripts/copy-dist.mjs              Task 1  build-output copier (exported copyDist + CLI)
  scripts/copy-dist.test.mjs         Task 1  node:test
  packages/sdk/
    package.json, tsconfig.json, tsconfig.build.json, vitest.config.ts   Task 2
    scripts/gen-types.mjs            Task 2  schema → TS generator (+ --check)
    src/generated/form.ts            Task 2  GENERATED, committed
    src/generated/workflow.ts        Task 2  GENERATED, committed
    src/types.ts                     Task 2  stable aliases (Field, State, …)
    src/api-types.ts                 Task 2  §7.3 wire shapes
    src/types.test.ts                Task 2
    src/index.ts                     Task 2 (+1 line Task 3, +1 line Task 4)
    src/client.ts, client.test.ts    Task 3
    src/logic.ts, logic.test.ts      Task 4
  packages/react/
    package.json, tsconfig.json, tsconfig.build.json, vitest.config.ts   Task 5
    src/test-setup.ts, src/test-fixtures.ts                               Task 5
    src/useOpenForm.ts, useOpenForm.test.tsx                             Task 5
    src/index.ts                     Task 5 (+1 line Task 6, +1 line Task 7)
    src/fields.tsx                   Task 6  default field components
    src/OpenForm.tsx, OpenForm.test.tsx   Task 6
    src/styles.css                   Task 6  (includes StatusTracker styles)
    src/StatusTracker.tsx, StatusTracker.test.tsx   Task 7
  packages/embed/
    package.json, tsconfig.json, vite.config.ts, vitest.config.ts        Task 8
    src/embed.ts, src/embed.test.ts, src/index.ts                         Task 8
  apps/hosted/
    package.json, tsconfig.json, vite.config.ts, vitest.config.ts, index.html   Task 9
    src/route.ts, route.test.ts      Task 9
    src/embed-bridge.ts              Task 9
    src/App.tsx, App.test.tsx        Task 9
    src/main.tsx, src/hosted.css, src/test-setup.ts                        Task 9
.gitignore (repo root)               Task 10 (append)
```

## Task order & parallelism

1. Task 1 → Task 2 run sequentially.
2. **Parallel group A:** Tasks 3, 4 and 8 run after Task 2. Task 8 only needs Task 1.
3. Task 5 runs after Tasks 3 and 4.
4. **Parallel group B:** Tasks 6 and 7 run after Task 5.
5. Task 9 runs after Tasks 6 and 7.
6. Task 10 runs last.

---

### Task 1: Workspace scaffold and `copy-dist.mjs`

**Files:**
- Create: `web/package.json`
- Create: `web/pnpm-workspace.yaml`
- Create: `web/tsconfig.base.json`
- Create: `web/.gitignore`
- Create: `web/scripts/copy-dist.mjs`
- Test: `web/scripts/copy-dist.test.mjs`

**Interfaces:**
- Consumes: nothing. `internal/webui/dist/.gitkeep` comes from Plan 01. The script creates the directory if it is missing.
- Produces: `copyDist(webDir: string, outDir: string): string[]` (returns the copied names, e.g. `["admin","hosted","embed"]`); the CLI entry `node scripts/copy-dist.mjs`; the root scripts `build`, `test`, `typecheck` and `test:scripts`.

- [ ] **Step 1: Create the workspace files**

`web/package.json`:
```json
{
  "name": "openforms-web",
  "private": true,
  "packageManager": "pnpm@9.15.4",
  "engines": { "node": ">=22" },
  "scripts": {
    "build": "pnpm -r --filter \"./packages/*\" build && pnpm -r --filter \"./apps/*\" build && node scripts/copy-dist.mjs",
    "test": "pnpm -r test",
    "typecheck": "pnpm -r typecheck",
    "test:scripts": "node --test \"scripts/*.test.mjs\""
  }
}
```

`web/pnpm-workspace.yaml`:
```yaml
packages:
  - "packages/*"
  - "apps/*"
```

`web/tsconfig.base.json`:
```json
{
  "compilerOptions": {
    "target": "ES2022",
    "lib": ["ES2022", "DOM", "DOM.Iterable"],
    "module": "ESNext",
    "moduleResolution": "Bundler",
    "jsx": "react-jsx",
    "strict": true,
    "isolatedModules": true,
    "esModuleInterop": true,
    "resolveJsonModule": true,
    "skipLibCheck": true,
    "forceConsistentCasingInFileNames": true,
    "noEmit": true,
    "types": []
  }
}
```

`web/.gitignore`:
```
node_modules/
dist/
*.tsbuildinfo
coverage/
```

- [ ] **Step 2: Write the failing test**

`web/scripts/copy-dist.test.mjs`:
```js
import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, mkdirSync, writeFileSync, existsSync, readFileSync, readdirSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { copyDist } from "./copy-dist.mjs";

function fixture() {
  const root = mkdtempSync(join(tmpdir(), "of-copy-"));
  const web = join(root, "web");
  const out = join(root, "internal", "webui", "dist");
  mkdirSync(join(web, "apps", "hosted", "dist", "assets"), { recursive: true });
  writeFileSync(join(web, "apps", "hosted", "dist", "index.html"), "<html>hosted</html>");
  writeFileSync(join(web, "apps", "hosted", "dist", "assets", "app.js"), "console.log(1)");
  mkdirSync(join(web, "apps", "admin"), { recursive: true }); // app without a build
  mkdirSync(join(web, "packages", "embed", "dist"), { recursive: true });
  writeFileSync(join(web, "packages", "embed", "dist", "embed.js"), "/*embed*/");
  mkdirSync(join(out, "stale"), { recursive: true });
  writeFileSync(join(out, "stale", "old.html"), "old");
  writeFileSync(join(out, ".gitkeep"), "");
  return { web, out };
}

test("copies built apps and embed.js, keeps .gitkeep, removes stale output", () => {
  const { web, out } = fixture();
  const copied = copyDist(web, out);
  assert.deepEqual(copied, ["hosted", "embed"]);
  assert.equal(readFileSync(join(out, "hosted", "index.html"), "utf8"), "<html>hosted</html>");
  assert.ok(existsSync(join(out, "hosted", "assets", "app.js")));
  assert.equal(readFileSync(join(out, "embed", "embed.js"), "utf8"), "/*embed*/");
  assert.ok(existsSync(join(out, ".gitkeep")));
  assert.ok(!existsSync(join(out, "stale")));
  assert.ok(!existsSync(join(out, "admin")));
});

test("creates the output directory when missing", () => {
  const root = mkdtempSync(join(tmpdir(), "of-copy-empty-"));
  const out = join(root, "dist");
  assert.deepEqual(copyDist(join(root, "web"), out), []);
  assert.deepEqual(readdirSync(out), []);
});
```

- [ ] **Step 3: Install and run the test to verify it fails**

Run: `pnpm -C web install && pnpm -C web test:scripts`
Expected: FAIL with `Cannot find module '.../web/scripts/copy-dist.mjs'` (ERR_MODULE_NOT_FOUND).

- [ ] **Step 4: Write the implementation**

`web/scripts/copy-dist.mjs`:
```js
// Copies web build outputs into internal/webui/dist so the Go binary can embed them.
import { cpSync, existsSync, mkdirSync, readdirSync, rmSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

export function copyDist(webDir, outDir) {
  mkdirSync(outDir, { recursive: true });
  for (const entry of readdirSync(outDir)) {
    if (entry === ".gitkeep") continue;
    rmSync(join(outDir, entry), { recursive: true, force: true });
  }

  const copied = [];
  const appsDir = join(webDir, "apps");
  if (existsSync(appsDir)) {
    for (const name of readdirSync(appsDir).sort()) {
      const dist = join(appsDir, name, "dist");
      if (!existsSync(dist)) continue;
      cpSync(dist, join(outDir, name), { recursive: true });
      copied.push(name);
    }
  }

  const embed = join(webDir, "packages", "embed", "dist", "embed.js");
  if (existsSync(embed)) {
    mkdirSync(join(outDir, "embed"), { recursive: true });
    cpSync(embed, join(outDir, "embed", "embed.js"));
    copied.push("embed");
  }
  return copied;
}

const invokedDirectly =
  process.argv[1] !== undefined && pathToFileURL(resolve(process.argv[1])).href === import.meta.url;

if (invokedDirectly) {
  const webDir = resolve(dirname(fileURLToPath(import.meta.url)), "..");
  const outDir = resolve(webDir, "..", "internal", "webui", "dist");
  const copied = copyDist(webDir, outDir);
  console.log(`copy-dist: ${copied.length ? copied.join(", ") : "nothing to copy"} → ${outDir}`);
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `pnpm -C web test:scripts`
Expected: PASS, with `# pass 2` and `# fail 0`.

- [ ] **Step 6: Commit**

```bash
git add web/package.json web/pnpm-workspace.yaml web/tsconfig.base.json web/.gitignore web/scripts web/pnpm-lock.yaml
git commit -m "chore(web): scaffold pnpm workspace and copy-dist build step"
```

---

### Task 2: `@openforms/sdk` package, generated definition types, wire types

**Files:**
- Create: `web/packages/sdk/package.json`
- Create: `web/packages/sdk/tsconfig.json`
- Create: `web/packages/sdk/tsconfig.build.json`
- Create: `web/packages/sdk/vitest.config.ts`
- Create: `web/packages/sdk/scripts/gen-types.mjs`
- Create (generated, committed): `web/packages/sdk/src/generated/form.ts`, `web/packages/sdk/src/generated/workflow.ts`
- Create: `web/packages/sdk/src/types.ts`
- Create: `web/packages/sdk/src/api-types.ts`
- Create: `web/packages/sdk/src/index.ts`
- Test: `web/packages/sdk/src/types.test.ts`

**Interfaces:**
- Consumes: `schemas/form.schema.json` and `schemas/workflow.schema.json` from Plan 02.
- Produces:
  - Definition types from `types.ts`: `FormDefinition`, `WorkflowDefinition`, `Field`, `FieldType`, `Option`, `Validation`, `Condition`, `FormSettings`, `State`, `StateColor`, `WorkflowField`, `Transition`, `Guard`, `Action`, `ActionType`.
  - Wire types from `api-types.ts`: `Problem`, `Source`, `EventType`, `ActorType`, `Principal`, `User`, `ApiKey`, `Assignee`, `Submission`, `SubmissionEvent`, `AvailableTransition`, `SubmissionDetail`, `ApplyItem`, `JobStatus`, `Job`, `FormSummary`, `WorkflowSummary`, `FormRecord`, `WorkflowRecord`, `VersionInfo`, `PublicHistoryItem`, `PublicStatus`, `PublicSubmitResult`, `Page<T>`, `Bundle`, `SubmissionFilter`, `TransitionInput`, `CreateUserInput`, `UpdateUserInput`, `CreateApiKeyInput`, `CreatedApiKey`.

- [ ] **Step 1: Create the package config**

`web/packages/sdk/package.json`:
```json
{
  "name": "@openforms/sdk",
  "version": "0.1.0",
  "license": "MIT",
  "type": "module",
  "exports": { ".": "./src/index.ts" },
  "types": "./src/index.ts",
  "files": ["dist"],
  "publishConfig": {
    "exports": { ".": { "types": "./dist/index.d.ts", "import": "./dist/index.js" } },
    "types": "./dist/index.d.ts"
  },
  "scripts": {
    "gen": "node scripts/gen-types.mjs",
    "build": "tsc -p tsconfig.build.json",
    "test": "node scripts/gen-types.mjs --check && vitest run",
    "typecheck": "tsc -p tsconfig.json"
  },
  "devDependencies": {
    "@types/node": "^22.10.0",
    "json-schema-to-typescript": "^15.0.4",
    "typescript": "^5.7.3",
    "vitest": "^3.0.5"
  }
}
```

`web/packages/sdk/tsconfig.json`:
```json
{
  "extends": "../../tsconfig.base.json",
  "compilerOptions": { "types": ["node"] },
  "include": ["src", "vitest.config.ts"]
}
```

`web/packages/sdk/tsconfig.build.json`:
```json
{
  "extends": "../../tsconfig.base.json",
  "compilerOptions": {
    "noEmit": false,
    "declaration": true,
    "outDir": "dist",
    "rootDir": "src",
    "types": []
  },
  "include": ["src"],
  "exclude": ["src/**/*.test.ts"]
}
```

`web/packages/sdk/vitest.config.ts`:
```ts
import { defineConfig } from "vitest/config";

export default defineConfig({
  test: { environment: "node", include: ["src/**/*.test.ts"] },
});
```

Then run `pnpm -C web install`.

- [ ] **Step 2: Write the failing type test**

`web/packages/sdk/src/types.test.ts`:
```ts
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
```

- [ ] **Step 3: Run typecheck to verify it fails**

Run: `pnpm -C web/packages/sdk typecheck`
Expected: FAIL with `error TS2307: Cannot find module './index.js'`.

- [ ] **Step 4: Write the type generator**

`web/packages/sdk/scripts/gen-types.mjs`:
```js
// Generates src/generated/*.ts from ../../../schemas/*.schema.json.
// Usage: node scripts/gen-types.mjs          (write files)
//        node scripts/gen-types.mjs --check  (exit 1 if committed files are stale)
import { compile } from "json-schema-to-typescript";
import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const schemasDir = resolve(here, "../../../../schemas");
const outDir = resolve(here, "../src/generated");
const check = process.argv.includes("--check");

const targets = [
  { schema: "form.schema.json", typeName: "FormDefinition", out: "form.ts" },
  { schema: "workflow.schema.json", typeName: "WorkflowDefinition", out: "workflow.ts" },
];

let stale = false;
mkdirSync(outDir, { recursive: true });

for (const t of targets) {
  const schema = JSON.parse(readFileSync(join(schemasDir, t.schema), "utf8"));
  // The root type name must be stable regardless of the schema's own title/$id.
  delete schema.title;
  delete schema.$id;
  const ts = await compile(schema, t.typeName, {
    cwd: schemasDir,
    additionalProperties: false,
    unreachableDefinitions: false,
    bannerComment: `/* eslint-disable */\n/**\n * GENERATED from schemas/${t.schema} by web/packages/sdk/scripts/gen-types.mjs.\n * Do not edit by hand: run \`pnpm -C web/packages/sdk gen\`.\n */`,
    style: { semi: true, singleQuote: false, trailingComma: "all", printWidth: 100 },
  });
  const file = join(outDir, t.out);
  if (check) {
    const current = existsSync(file) ? readFileSync(file, "utf8") : "";
    if (current !== ts) {
      console.error(`stale: src/generated/${t.out} — run \`pnpm -C web/packages/sdk gen\``);
      stale = true;
    }
  } else {
    writeFileSync(file, ts);
    console.log(`wrote src/generated/${t.out}`);
  }
}

if (stale) process.exit(1);
```

Run: `pnpm -C web/packages/sdk gen`
Expected: `wrote src/generated/form.ts` and `wrote src/generated/workflow.ts`. Open both files. They must export `interface FormDefinition` and `interface WorkflowDefinition`. Commit them as generated and do not edit them by hand.

- [ ] **Step 5: Write the stable type aliases**

`web/packages/sdk/src/types.ts`:
```ts
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
```

- [ ] **Step 6: Write the wire types (spec §7.3)**

`web/packages/sdk/src/api-types.ts`:
```ts
import type { FormDefinition, State, WorkflowDefinition } from "./types.js";

export interface Problem {
  path: string;
  message: string;
}

export type Source = "cli" | "ui" | "api" | "seed";
export type EventType =
  | "created"
  | "transition"
  | "fields_updated"
  | "assigned"
  | "comment"
  | "action_succeeded"
  | "action_failed";
export type ActorType = "user" | "api_key" | "system" | "respondent";

export interface Principal {
  kind: "user" | "api_key";
  id: string;
  name: string;
  email: string;
  roles: string[];
}

export interface User {
  id: string;
  email: string;
  name: string;
  roles: string[];
  createdAt: string;
}

export interface ApiKey {
  id: string;
  name: string;
  prefix: string;
  roles: string[];
  createdAt: string;
  lastUsedAt: string | null;
  revokedAt: string | null;
}

export interface Assignee {
  id: string;
  name: string;
  email: string;
}

export interface Submission {
  id: string;
  form: string;
  formVersion: number;
  state: string;
  stateLabel: string;
  terminal: boolean;
  data: Record<string, unknown>;
  fields: Record<string, unknown>;
  assignee: Assignee | null;
  createdAt: string;
  updatedAt: string;
}

export interface SubmissionEvent {
  id: number;
  type: EventType;
  fromState: string | null;
  toState: string | null;
  transition: string | null;
  actor: { type: ActorType; id: string | null; name: string };
  payload: Record<string, unknown>;
  createdAt: string;
}

export interface AvailableTransition {
  key: string;
  label: string;
  to: string;
  toLabel: string;
  requireFields: string[];
  allowed: boolean;
  reason: "" | "role";
}

export interface SubmissionDetail {
  submission: Submission;
  form: FormDefinition;
  workflow: WorkflowDefinition | null;
  events: SubmissionEvent[];
  transitions: AvailableTransition[];
}

export interface ApplyItem {
  kind: "form" | "workflow";
  slug: string;
  version: number;
  changed: boolean;
  created: boolean;
}

export type JobStatus = "pending" | "running" | "done" | "failed";

export interface Job {
  id: number;
  kind: string;
  status: JobStatus;
  attempts: number;
  maxAttempts: number;
  runAt: string;
  lastError: string;
  payload: unknown;
  createdAt: string;
  updatedAt: string;
}

export interface FormSummary {
  slug: string;
  title: string;
  workflow?: string | null;
  public: boolean;
  version: number;
  source: Source;
  updatedAt: string;
  submissionCount: number;
}

export interface WorkflowSummary {
  slug: string;
  title: string;
  version: number;
  source: Source;
  updatedAt: string;
  stateCount: number;
}

export interface FormRecord {
  slug: string;
  version: number;
  source: Source;
  updatedAt: string;
  workflowVersion: number | null;
  definition: FormDefinition;
}

export interface WorkflowRecord {
  slug: string;
  version: number;
  source: Source;
  updatedAt: string;
  definition: WorkflowDefinition;
}

export interface VersionInfo {
  version: number;
  hash: string;
  source: Source;
  createdBy: string;
  createdAt: string;
}

export interface PublicHistoryItem {
  state: string;
  label: string;
  at: string;
}

export interface PublicStatus {
  id: string;
  formTitle: string;
  state: string;
  stateLabel: string;
  terminal: boolean;
  states: State[];
  history: PublicHistoryItem[];
  createdAt: string;
}

export interface PublicSubmitResult {
  id: string;
  state: string;
  stateLabel: string;
  receiptToken: string;
  confirmationMessage?: string;
}

export interface Page<T> {
  items: T[];
  nextCursor: string | null;
}

export interface Bundle {
  forms: FormDefinition[];
  workflows: WorkflowDefinition[];
}

export interface SubmissionFilter {
  form?: string;
  state?: string;
  /** A user id, `"me"` or `"none"`. */
  assignee?: string;
  cursor?: string;
  limit?: number;
}

export interface TransitionInput {
  transition: string;
  fields?: Record<string, unknown>;
  comment?: string;
  expectedState?: string;
}

export interface CreateUserInput {
  email: string;
  name: string;
  password: string;
  roles: string[];
}

export interface UpdateUserInput {
  name?: string;
  password?: string;
  roles?: string[];
}

export interface CreateApiKeyInput {
  name: string;
  roles: string[];
}

export interface CreatedApiKey {
  apiKey: ApiKey;
  key: string;
}
```

`web/packages/sdk/src/index.ts`:
```ts
export * from "./types.js";
export * from "./api-types.js";
```

- [ ] **Step 7: Run typecheck and tests to verify they pass**

Run: `pnpm -C web/packages/sdk typecheck && pnpm -C web/packages/sdk test`
Expected: typecheck exits 0. The test run first runs `gen --check` (no output means up to date), then Vitest reports `src/types.test.ts (3 tests)` passing.

If typecheck fails on the spec examples, check `schemas/*.schema.json` against spec §5.1/§5.2 and fix the schema in Plan 02's area. Do not loosen the types to get around it.

- [ ] **Step 8: Commit**

```bash
git add web/packages/sdk web/pnpm-lock.yaml
git commit -m "feat(sdk): generated definition types and API wire types"
```

---

### Task 3: `OpenFormsClient` and `OpenFormsError`

**Parallel group:** A (with Tasks 4 and 8). This task appends one export line to `src/index.ts`.

**Files:**
- Create: `web/packages/sdk/src/client.ts`
- Modify: `web/packages/sdk/src/index.ts` (append `export * from "./client.js";`)
- Test: `web/packages/sdk/src/client.test.ts`

**Interfaces:**
- Consumes: the Task 2 types.
- Produces:
```ts
class OpenFormsError extends Error { readonly status: number; readonly code: string; readonly details: Problem[] }
interface ClientOptions { baseUrl?: string; apiKey?: string; fetch?: typeof fetch }
class OpenFormsClient {
  constructor(opts?: ClientOptions);
  readonly baseUrl: string;
  url(path: string, query?: Record<string, string | number | boolean | null | undefined>): string; // `${baseUrl}/api/v1${path}?…`
  getPublicForm(slug): Promise<FormDefinition>;
  submitPublic(slug, data): Promise<PublicSubmitResult>;
  getPublicStatus(id, token): Promise<PublicStatus>;
  login(email, password): Promise<User>; logout(): Promise<void>; me(): Promise<Principal>;
  listForms(): Promise<FormSummary[]>; getForm(slug): Promise<FormRecord>;
  putForm(slug, def, opts?: { source?: Source }): Promise<ApplyItem>;
  formVersions(slug): Promise<VersionInfo[]>; formVersion(slug, n): Promise<FormRecord>;
  listWorkflows(): Promise<WorkflowSummary[]>; getWorkflow(slug): Promise<WorkflowRecord>;
  putWorkflow(slug, def, opts?: { source?: Source }): Promise<ApplyItem>;
  workflowVersions(slug): Promise<VersionInfo[]>; workflowVersion(slug, n): Promise<WorkflowRecord>;
  validateDefinitions(bundle): Promise<{ valid: true }>;
  applyDefinitions(bundle, opts?: { dryRun?: boolean; source?: Source }): Promise<ApplyItem[]>;
  exportDefinitions(): Promise<Bundle>;
  listSubmissions(filter?: SubmissionFilter): Promise<Page<Submission>>;
  getSubmission(id): Promise<SubmissionDetail>;
  createSubmission(form, data): Promise<{ submission: Submission; receiptToken: string }>;
  transition(id, input: TransitionInput): Promise<Submission>;
  updateFields(id, fields): Promise<Submission>;
  comment(id, body): Promise<SubmissionEvent>;
  assign(id, userId: string | null): Promise<Submission>;
  csvUrl(slug): string;
  listUsers(): Promise<User[]>; createUser(input): Promise<User>; updateUser(id, input): Promise<User>; deleteUser(id): Promise<void>;
  listApiKeys(): Promise<ApiKey[]>; createApiKey(input): Promise<CreatedApiKey>; revokeApiKey(id): Promise<void>;
  listJobs(status?: JobStatus): Promise<Job[]>; retryJob(id: number): Promise<void>;
}
```
  Error codes produced client-side: `network_error` (status 0), `http_error` (non-2xx without an envelope), `invalid_response` (2xx without JSON).

- [ ] **Step 1: Write the failing test**

`web/packages/sdk/src/client.test.ts`:
```ts
import { describe, expect, it, vi } from "vitest";
import { OpenFormsClient, OpenFormsError } from "./client.js";
import type {
  ApiKey,
  ApplyItem,
  Bundle,
  FormRecord,
  FormSummary,
  Job,
  Principal,
  PublicStatus,
  PublicSubmitResult,
  Submission,
  SubmissionDetail,
  SubmissionEvent,
  User,
  VersionInfo,
  WorkflowRecord,
  WorkflowSummary,
} from "./api-types.js";
import type { FormDefinition, WorkflowDefinition } from "./types.js";

function mockFetch(status: number, body?: unknown, contentType = "application/json") {
  return vi.fn<typeof fetch>(async () =>
    status === 204 || body === undefined
      ? new Response(null, { status })
      : new Response(typeof body === "string" ? body : JSON.stringify(body), {
          status,
          headers: { "Content-Type": contentType },
        }),
  );
}

const form: FormDefinition = {
  slug: "contact",
  title: "Contact us",
  settings: { public: true },
  fields: [{ key: "email", type: "email", label: "Email", required: true }],
};
const workflow: WorkflowDefinition = {
  slug: "contact-triage",
  title: "Contact triage",
  initial: "new",
  states: [
    { key: "new", label: "New" },
    { key: "done", label: "Done", terminal: true },
  ],
  transitions: [{ key: "close", label: "Close", from: ["new"], to: "done", guard: {} }],
};
const submission: Submission = {
  id: "s1",
  form: "contact",
  formVersion: 1,
  state: "new",
  stateLabel: "New",
  terminal: false,
  data: { email: "ada@example.com" },
  fields: {},
  assignee: null,
  createdAt: "2026-09-23T10:00:00Z",
  updatedAt: "2026-09-23T10:00:00Z",
};
const event: SubmissionEvent = {
  id: 7,
  type: "comment",
  fromState: null,
  toState: null,
  transition: null,
  actor: { type: "user", id: "u1", name: "Rita Reviewer" },
  payload: { body: "hi" },
  createdAt: "2026-09-23T10:05:00Z",
};
const user: User = { id: "u1", email: "rita@example.com", name: "Rita Reviewer", roles: ["reviewer"], createdAt: "2026-09-01T00:00:00Z" };
const principal: Principal = { kind: "user", id: "u1", name: "Rita Reviewer", email: "rita@example.com", roles: ["reviewer"] };
const apiKey: ApiKey = { id: "k1", name: "ci", prefix: "ofk_abcd", roles: ["admin"], createdAt: "2026-09-01T00:00:00Z", lastUsedAt: null, revokedAt: null };
const formRecord: FormRecord = { slug: "contact", version: 2, source: "cli", updatedAt: "2026-09-23T10:00:00Z", workflowVersion: 1, definition: form };
const workflowRecord: WorkflowRecord = { slug: "contact-triage", version: 1, source: "cli", updatedAt: "2026-09-23T10:00:00Z", definition: workflow };
const formSummary: FormSummary = { slug: "contact", title: "Contact us", workflow: "contact-triage", public: true, version: 2, source: "cli", updatedAt: "2026-09-23T10:00:00Z", submissionCount: 3 };
const workflowSummary: WorkflowSummary = { slug: "contact-triage", title: "Contact triage", version: 1, source: "cli", updatedAt: "2026-09-23T10:00:00Z", stateCount: 2 };
const version: VersionInfo = { version: 2, hash: "abc", source: "ui", createdBy: "rita@example.com", createdAt: "2026-09-23T10:00:00Z" };
const item: ApplyItem = { kind: "form", slug: "contact", version: 3, changed: true, created: false };
const bundle: Bundle = { forms: [form], workflows: [workflow] };
const job: Job = { id: 7, kind: "action.webhook", status: "failed", attempts: 8, maxAttempts: 8, runAt: "2026-09-23T10:00:00Z", lastError: "HTTP 500", payload: {}, createdAt: "2026-09-23T10:00:00Z", updatedAt: "2026-09-23T10:00:00Z" };
const submitResult: PublicSubmitResult = { id: "s1", state: "new", stateLabel: "New", receiptToken: "tok", confirmationMessage: "Thanks!" };
const status: PublicStatus = { id: "s1", formTitle: "Contact us", state: "new", stateLabel: "New", terminal: false, states: workflow.states, history: [{ state: "new", label: "New", at: "2026-09-23T10:00:00Z" }], createdAt: "2026-09-23T10:00:00Z" };
const detail: SubmissionDetail = { submission, form, workflow, events: [event], transitions: [] };

interface Case {
  name: string;
  call: (c: OpenFormsClient) => Promise<unknown>;
  method: string;
  path: string;
  body?: unknown;
  status?: number;
  response?: unknown;
  expected: unknown;
}

const cases: Case[] = [
  { name: "getPublicForm", call: (c) => c.getPublicForm("job application"), method: "GET", path: "/api/v1/public/forms/job%20application", response: { form }, expected: form },
  { name: "submitPublic", call: (c) => c.submitPublic("contact", { email: "ada@example.com" }), method: "POST", path: "/api/v1/public/forms/contact/submissions", body: { data: { email: "ada@example.com" } }, status: 201, response: submitResult, expected: submitResult },
  { name: "getPublicStatus", call: (c) => c.getPublicStatus("s1", "a/b+c"), method: "GET", path: "/api/v1/public/submissions/s1?token=a%2Fb%2Bc", response: status, expected: status },
  { name: "login", call: (c) => c.login("rita@example.com", "secret123"), method: "POST", path: "/api/v1/auth/login", body: { email: "rita@example.com", password: "secret123" }, response: { user }, expected: user },
  { name: "logout", call: (c) => c.logout(), method: "POST", path: "/api/v1/auth/logout", status: 204, expected: undefined },
  { name: "me", call: (c) => c.me(), method: "GET", path: "/api/v1/auth/me", response: { principal }, expected: principal },
  { name: "listForms", call: (c) => c.listForms(), method: "GET", path: "/api/v1/forms", response: { items: [formSummary] }, expected: [formSummary] },
  { name: "getForm", call: (c) => c.getForm("contact"), method: "GET", path: "/api/v1/forms/contact", response: { form: formRecord }, expected: formRecord },
  { name: "putForm", call: (c) => c.putForm("contact", form, { source: "ui" }), method: "PUT", path: "/api/v1/forms/contact?source=ui", body: form, response: { item }, expected: item },
  { name: "formVersions", call: (c) => c.formVersions("contact"), method: "GET", path: "/api/v1/forms/contact/versions", response: { items: [version] }, expected: [version] },
  { name: "formVersion", call: (c) => c.formVersion("contact", 2), method: "GET", path: "/api/v1/forms/contact/versions/2", response: { form: formRecord }, expected: formRecord },
  { name: "listWorkflows", call: (c) => c.listWorkflows(), method: "GET", path: "/api/v1/workflows", response: { items: [workflowSummary] }, expected: [workflowSummary] },
  { name: "getWorkflow", call: (c) => c.getWorkflow("contact-triage"), method: "GET", path: "/api/v1/workflows/contact-triage", response: { workflow: workflowRecord }, expected: workflowRecord },
  { name: "putWorkflow", call: (c) => c.putWorkflow("contact-triage", workflow), method: "PUT", path: "/api/v1/workflows/contact-triage", body: workflow, response: { item }, expected: item },
  { name: "workflowVersions", call: (c) => c.workflowVersions("contact-triage"), method: "GET", path: "/api/v1/workflows/contact-triage/versions", response: { items: [version] }, expected: [version] },
  { name: "workflowVersion", call: (c) => c.workflowVersion("contact-triage", 1), method: "GET", path: "/api/v1/workflows/contact-triage/versions/1", response: { workflow: workflowRecord }, expected: workflowRecord },
  { name: "validateDefinitions", call: (c) => c.validateDefinitions(bundle), method: "POST", path: "/api/v1/definitions/validate", body: bundle, response: { valid: true }, expected: { valid: true } },
  { name: "applyDefinitions", call: (c) => c.applyDefinitions(bundle, { dryRun: true, source: "cli" }), method: "POST", path: "/api/v1/definitions/apply?dryRun=true&source=cli", body: bundle, response: { items: [item] }, expected: [item] },
  { name: "exportDefinitions", call: (c) => c.exportDefinitions(), method: "GET", path: "/api/v1/definitions", response: bundle, expected: bundle },
  { name: "listSubmissions", call: (c) => c.listSubmissions({ form: "contact", state: "new", assignee: "me", cursor: "abc", limit: 25 }), method: "GET", path: "/api/v1/submissions?form=contact&state=new&assignee=me&cursor=abc&limit=25", response: { items: [submission], nextCursor: null }, expected: { items: [submission], nextCursor: null } },
  { name: "getSubmission", call: (c) => c.getSubmission("s1"), method: "GET", path: "/api/v1/submissions/s1", response: detail, expected: detail },
  { name: "createSubmission", call: (c) => c.createSubmission("contact", { email: "ada@example.com" }), method: "POST", path: "/api/v1/submissions", body: { form: "contact", data: { email: "ada@example.com" } }, status: 201, response: { submission, receiptToken: "tok" }, expected: { submission, receiptToken: "tok" } },
  { name: "transition", call: (c) => c.transition("s1", { transition: "close", comment: "done", expectedState: "new" }), method: "POST", path: "/api/v1/submissions/s1/transitions", body: { transition: "close", comment: "done", expectedState: "new" }, response: { submission }, expected: submission },
  { name: "updateFields", call: (c) => c.updateFields("s1", { score: 4 }), method: "PATCH", path: "/api/v1/submissions/s1/fields", body: { fields: { score: 4 } }, response: { submission }, expected: submission },
  { name: "comment", call: (c) => c.comment("s1", "hi"), method: "POST", path: "/api/v1/submissions/s1/comments", body: { body: "hi" }, status: 201, response: { event }, expected: event },
  { name: "assign", call: (c) => c.assign("s1", null), method: "PUT", path: "/api/v1/submissions/s1/assignee", body: { userId: null }, response: { submission }, expected: submission },
  { name: "listUsers", call: (c) => c.listUsers(), method: "GET", path: "/api/v1/users", response: { items: [user] }, expected: [user] },
  { name: "createUser", call: (c) => c.createUser({ email: "rita@example.com", name: "Rita Reviewer", password: "secret123", roles: ["reviewer"] }), method: "POST", path: "/api/v1/users", body: { email: "rita@example.com", name: "Rita Reviewer", password: "secret123", roles: ["reviewer"] }, status: 201, response: { user }, expected: user },
  { name: "updateUser", call: (c) => c.updateUser("u1", { roles: ["reviewer", "hiring-manager"] }), method: "PATCH", path: "/api/v1/users/u1", body: { roles: ["reviewer", "hiring-manager"] }, response: { user }, expected: user },
  { name: "deleteUser", call: (c) => c.deleteUser("u1"), method: "DELETE", path: "/api/v1/users/u1", status: 204, expected: undefined },
  { name: "listApiKeys", call: (c) => c.listApiKeys(), method: "GET", path: "/api/v1/api-keys", response: { items: [apiKey] }, expected: [apiKey] },
  { name: "createApiKey", call: (c) => c.createApiKey({ name: "ci", roles: ["admin"] }), method: "POST", path: "/api/v1/api-keys", body: { name: "ci", roles: ["admin"] }, status: 201, response: { apiKey, key: "ofk_secret" }, expected: { apiKey, key: "ofk_secret" } },
  { name: "revokeApiKey", call: (c) => c.revokeApiKey("k1"), method: "DELETE", path: "/api/v1/api-keys/k1", status: 204, expected: undefined },
  { name: "listJobs", call: (c) => c.listJobs("failed"), method: "GET", path: "/api/v1/jobs?status=failed", response: { items: [job] }, expected: [job] },
  { name: "retryJob", call: (c) => c.retryJob(7), method: "POST", path: "/api/v1/jobs/7/retry", status: 204, expected: undefined },
];

describe("OpenFormsClient endpoints", () => {
  it.each(cases)("$name → $method $path", async (tc) => {
    const fetchMock = mockFetch(tc.status ?? 200, tc.response);
    const client = new OpenFormsClient({ baseUrl: "http://api.test/", fetch: fetchMock });

    await expect(tc.call(client)).resolves.toEqual(tc.expected);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0]!;
    expect(url).toBe(`http://api.test${tc.path}`);
    expect(init?.method).toBe(tc.method);
    expect(init?.credentials).toBe("include");
    if (tc.body === undefined) {
      expect(init?.body).toBeUndefined();
    } else {
      expect((init?.headers as Record<string, string>)["Content-Type"]).toBe("application/json");
      expect(JSON.parse(String(init?.body))).toEqual(tc.body);
    }
  });

  it("covers every client method listed in the spec", () => {
    const names = new Set(cases.map((c) => c.name));
    const spec = [
      "getPublicForm", "submitPublic", "getPublicStatus", "login", "logout", "me", "listForms", "getForm", "putForm",
      "formVersions", "formVersion", "listWorkflows", "getWorkflow", "putWorkflow", "workflowVersions", "workflowVersion",
      "validateDefinitions", "applyDefinitions", "exportDefinitions", "listSubmissions", "getSubmission", "createSubmission",
      "transition", "updateFields", "comment", "assign", "listUsers", "createUser", "updateUser", "deleteUser",
      "listApiKeys", "createApiKey", "revokeApiKey", "listJobs", "retryJob",
    ];
    expect(spec.filter((n) => !names.has(n))).toEqual([]);
  });

  it("builds the CSV export URL without fetching", () => {
    const fetchMock = mockFetch(200, {});
    const client = new OpenFormsClient({ baseUrl: "https://forms.example.com", fetch: fetchMock });
    expect(client.csvUrl("job application")).toBe("https://forms.example.com/api/v1/forms/job%20application/submissions.csv");
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("defaults to same-origin relative URLs", () => {
    expect(new OpenFormsClient().url("/forms")).toBe("/api/v1/forms");
  });

  it("omits empty query parameters", async () => {
    const fetchMock = mockFetch(200, { items: [], nextCursor: null });
    const client = new OpenFormsClient({ baseUrl: "http://api.test", fetch: fetchMock });
    await client.listSubmissions({ form: "contact", state: "", cursor: undefined });
    expect(fetchMock.mock.calls[0]![0]).toBe("http://api.test/api/v1/submissions?form=contact");
  });

  it("sends the API key as a bearer token", async () => {
    const fetchMock = mockFetch(200, { items: [] });
    const client = new OpenFormsClient({ baseUrl: "http://api.test", apiKey: "ofk_test", fetch: fetchMock });
    await client.listForms();
    const init = fetchMock.mock.calls[0]![1];
    expect((init?.headers as Record<string, string>).Authorization).toBe("Bearer ofk_test");
  });
});

describe("OpenFormsClient errors", () => {
  it("throws OpenFormsError from the error envelope", async () => {
    const client = new OpenFormsClient({
      baseUrl: "http://api.test",
      fetch: mockFetch(422, {
        error: { code: "validation_failed", message: "submission is invalid", details: [{ path: "data.email", message: "is required" }] },
      }),
    });
    const err = await client.submitPublic("contact", {}).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(OpenFormsError);
    expect(err).toMatchObject({
      status: 422,
      code: "validation_failed",
      message: "submission is invalid",
      details: [{ path: "data.email", message: "is required" }],
    });
  });

  it("defaults details to an empty array", async () => {
    const client = new OpenFormsClient({ baseUrl: "http://api.test", fetch: mockFetch(404, { error: { code: "not_found", message: "form not found" } }) });
    await expect(client.getPublicForm("nope")).rejects.toMatchObject({ status: 404, code: "not_found", details: [] });
  });

  it("maps non-JSON error bodies to http_error", async () => {
    const client = new OpenFormsClient({ baseUrl: "http://api.test", fetch: mockFetch(502, "<html>Bad gateway</html>", "text/html") });
    await expect(client.listForms()).rejects.toMatchObject({ status: 502, code: "http_error" });
  });

  it("maps a 2xx non-JSON body to invalid_response", async () => {
    const client = new OpenFormsClient({ baseUrl: "http://api.test", fetch: mockFetch(200, "<html>login page</html>", "text/html") });
    await expect(client.listForms()).rejects.toMatchObject({ status: 200, code: "invalid_response" });
  });

  it("maps network failures to network_error", async () => {
    const client = new OpenFormsClient({
      baseUrl: "http://api.test",
      fetch: vi.fn<typeof fetch>(async () => {
        throw new TypeError("Failed to fetch");
      }),
    });
    await expect(client.me()).rejects.toMatchObject({ status: 0, code: "network_error", message: "Failed to fetch" });
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `pnpm -C web/packages/sdk exec vitest run src/client.test.ts`
Expected: FAIL with `Failed to resolve import "./client.js"`.

- [ ] **Step 3: Write the implementation**

`web/packages/sdk/src/client.ts`:
```ts
import type {
  ApiKey,
  ApplyItem,
  Bundle,
  CreateApiKeyInput,
  CreatedApiKey,
  CreateUserInput,
  FormRecord,
  FormSummary,
  Job,
  JobStatus,
  Page,
  Principal,
  Problem,
  PublicStatus,
  PublicSubmitResult,
  Source,
  Submission,
  SubmissionDetail,
  SubmissionEvent,
  SubmissionFilter,
  TransitionInput,
  UpdateUserInput,
  User,
  VersionInfo,
  WorkflowRecord,
  WorkflowSummary,
} from "./api-types.js";
import type { FormDefinition, WorkflowDefinition } from "./types.js";

export class OpenFormsError extends Error {
  readonly status: number;
  readonly code: string;
  readonly details: Problem[];

  constructor(status: number, code: string, message: string, details: Problem[] = []) {
    super(message);
    this.name = "OpenFormsError";
    this.status = status;
    this.code = code;
    this.details = details;
  }
}

export interface ClientOptions {
  /** Origin of the openforms server, e.g. "https://forms.example.com". Defaults to same origin. */
  baseUrl?: string;
  /** API key (`ofk_…`) sent as a bearer token. Omit in browsers that use the session cookie. */
  apiKey?: string;
  fetch?: typeof fetch;
}

type Query = Record<string, string | number | boolean | null | undefined>;

interface RequestOptions {
  body?: unknown;
  query?: Query;
}

interface ErrorEnvelope {
  error?: { code?: unknown; message?: unknown; details?: unknown };
}

const seg = encodeURIComponent;

export class OpenFormsClient {
  readonly baseUrl: string;
  private readonly apiKey: string | undefined;
  private readonly fetchImpl: typeof fetch;

  constructor(opts: ClientOptions = {}) {
    this.baseUrl = (opts.baseUrl ?? "").replace(/\/+$/, "");
    this.apiKey = opts.apiKey;
    // Resolve globalThis.fetch lazily so test interceptors (MSW) installed later are honoured.
    this.fetchImpl = opts.fetch ?? ((input, init) => globalThis.fetch(input, init));
  }

  url(path: string, query?: Query): string {
    let url = `${this.baseUrl}/api/v1${path}`;
    if (query) {
      const params = new URLSearchParams();
      for (const [key, value] of Object.entries(query)) {
        if (value === undefined || value === null || value === "") continue;
        params.set(key, String(value));
      }
      const qs = params.toString();
      if (qs) url += `?${qs}`;
    }
    return url;
  }

  private async request<T>(method: string, path: string, opts: RequestOptions = {}): Promise<T> {
    const headers: Record<string, string> = { Accept: "application/json" };
    if (opts.body !== undefined) headers["Content-Type"] = "application/json";
    if (this.apiKey) headers.Authorization = `Bearer ${this.apiKey}`;

    let res: Response;
    try {
      res = await this.fetchImpl(this.url(path, opts.query), {
        method,
        headers,
        credentials: "include",
        body: opts.body === undefined ? undefined : JSON.stringify(opts.body),
      });
    } catch (err) {
      throw new OpenFormsError(0, "network_error", err instanceof Error ? err.message : "Network request failed");
    }

    if (res.status === 204) return undefined as T;

    const text = await res.text();
    let json: unknown;
    let parsed = false;
    if (text) {
      try {
        json = JSON.parse(text);
        parsed = true;
      } catch {
        parsed = false;
      }
    }

    if (!res.ok) {
      const env = parsed ? (json as ErrorEnvelope).error : undefined;
      if (env && typeof env.code === "string") {
        const message = typeof env.message === "string" ? env.message : env.code;
        const details = Array.isArray(env.details) ? (env.details as Problem[]) : [];
        throw new OpenFormsError(res.status, env.code, message, details);
      }
      throw new OpenFormsError(res.status, "http_error", `Request failed with status ${res.status}`);
    }

    if (text && !parsed) {
      throw new OpenFormsError(res.status, "invalid_response", "The server returned a response that is not JSON");
    }
    return json as T;
  }

  // ---- Public (no auth) ----
  async getPublicForm(slug: string): Promise<FormDefinition> {
    return (await this.request<{ form: FormDefinition }>("GET", `/public/forms/${seg(slug)}`)).form;
  }

  submitPublic(slug: string, data: Record<string, unknown>): Promise<PublicSubmitResult> {
    return this.request("POST", `/public/forms/${seg(slug)}/submissions`, { body: { data } });
  }

  getPublicStatus(id: string, token: string): Promise<PublicStatus> {
    return this.request("GET", `/public/submissions/${seg(id)}`, { query: { token } });
  }

  // ---- Auth ----
  async login(email: string, password: string): Promise<User> {
    return (await this.request<{ user: User }>("POST", "/auth/login", { body: { email, password } })).user;
  }

  async logout(): Promise<void> {
    await this.request<void>("POST", "/auth/logout");
  }

  async me(): Promise<Principal> {
    return (await this.request<{ principal: Principal }>("GET", "/auth/me")).principal;
  }

  // ---- Forms ----
  async listForms(): Promise<FormSummary[]> {
    return (await this.request<{ items: FormSummary[] }>("GET", "/forms")).items;
  }

  async getForm(slug: string): Promise<FormRecord> {
    return (await this.request<{ form: FormRecord }>("GET", `/forms/${seg(slug)}`)).form;
  }

  async putForm(slug: string, definition: FormDefinition, opts: { source?: Source } = {}): Promise<ApplyItem> {
    return (
      await this.request<{ item: ApplyItem }>("PUT", `/forms/${seg(slug)}`, {
        body: definition,
        query: { source: opts.source },
      })
    ).item;
  }

  async formVersions(slug: string): Promise<VersionInfo[]> {
    return (await this.request<{ items: VersionInfo[] }>("GET", `/forms/${seg(slug)}/versions`)).items;
  }

  async formVersion(slug: string, version: number): Promise<FormRecord> {
    return (await this.request<{ form: FormRecord }>("GET", `/forms/${seg(slug)}/versions/${version}`)).form;
  }

  // ---- Workflows ----
  async listWorkflows(): Promise<WorkflowSummary[]> {
    return (await this.request<{ items: WorkflowSummary[] }>("GET", "/workflows")).items;
  }

  async getWorkflow(slug: string): Promise<WorkflowRecord> {
    return (await this.request<{ workflow: WorkflowRecord }>("GET", `/workflows/${seg(slug)}`)).workflow;
  }

  async putWorkflow(slug: string, definition: WorkflowDefinition, opts: { source?: Source } = {}): Promise<ApplyItem> {
    return (
      await this.request<{ item: ApplyItem }>("PUT", `/workflows/${seg(slug)}`, {
        body: definition,
        query: { source: opts.source },
      })
    ).item;
  }

  async workflowVersions(slug: string): Promise<VersionInfo[]> {
    return (await this.request<{ items: VersionInfo[] }>("GET", `/workflows/${seg(slug)}/versions`)).items;
  }

  async workflowVersion(slug: string, version: number): Promise<WorkflowRecord> {
    return (await this.request<{ workflow: WorkflowRecord }>("GET", `/workflows/${seg(slug)}/versions/${version}`))
      .workflow;
  }

  // ---- Definitions bundle ----
  validateDefinitions(bundle: Bundle): Promise<{ valid: true }> {
    return this.request("POST", "/definitions/validate", { body: bundle });
  }

  async applyDefinitions(bundle: Bundle, opts: { dryRun?: boolean; source?: Source } = {}): Promise<ApplyItem[]> {
    return (
      await this.request<{ items: ApplyItem[] }>("POST", "/definitions/apply", {
        body: bundle,
        query: { dryRun: opts.dryRun ? "true" : undefined, source: opts.source },
      })
    ).items;
  }

  exportDefinitions(): Promise<Bundle> {
    return this.request("GET", "/definitions");
  }

  // ---- Submissions ----
  listSubmissions(filter: SubmissionFilter = {}): Promise<Page<Submission>> {
    return this.request("GET", "/submissions", {
      query: {
        form: filter.form,
        state: filter.state,
        assignee: filter.assignee,
        cursor: filter.cursor,
        limit: filter.limit,
      },
    });
  }

  getSubmission(id: string): Promise<SubmissionDetail> {
    return this.request("GET", `/submissions/${seg(id)}`);
  }

  createSubmission(form: string, data: Record<string, unknown>): Promise<{ submission: Submission; receiptToken: string }> {
    return this.request("POST", "/submissions", { body: { form, data } });
  }

  async transition(id: string, input: TransitionInput): Promise<Submission> {
    return (await this.request<{ submission: Submission }>("POST", `/submissions/${seg(id)}/transitions`, { body: input }))
      .submission;
  }

  async updateFields(id: string, fields: Record<string, unknown>): Promise<Submission> {
    return (await this.request<{ submission: Submission }>("PATCH", `/submissions/${seg(id)}/fields`, { body: { fields } }))
      .submission;
  }

  async comment(id: string, body: string): Promise<SubmissionEvent> {
    return (await this.request<{ event: SubmissionEvent }>("POST", `/submissions/${seg(id)}/comments`, { body: { body } }))
      .event;
  }

  async assign(id: string, userId: string | null): Promise<Submission> {
    return (await this.request<{ submission: Submission }>("PUT", `/submissions/${seg(id)}/assignee`, { body: { userId } }))
      .submission;
  }

  csvUrl(slug: string): string {
    return this.url(`/forms/${seg(slug)}/submissions.csv`);
  }

  // ---- Users (admin) ----
  async listUsers(): Promise<User[]> {
    return (await this.request<{ items: User[] }>("GET", "/users")).items;
  }

  async createUser(input: CreateUserInput): Promise<User> {
    return (await this.request<{ user: User }>("POST", "/users", { body: input })).user;
  }

  async updateUser(id: string, input: UpdateUserInput): Promise<User> {
    return (await this.request<{ user: User }>("PATCH", `/users/${seg(id)}`, { body: input })).user;
  }

  async deleteUser(id: string): Promise<void> {
    await this.request<void>("DELETE", `/users/${seg(id)}`);
  }

  // ---- API keys (admin) ----
  async listApiKeys(): Promise<ApiKey[]> {
    return (await this.request<{ items: ApiKey[] }>("GET", "/api-keys")).items;
  }

  createApiKey(input: CreateApiKeyInput): Promise<CreatedApiKey> {
    return this.request("POST", "/api-keys", { body: input });
  }

  async revokeApiKey(id: string): Promise<void> {
    await this.request<void>("DELETE", `/api-keys/${seg(id)}`);
  }

  // ---- Jobs (admin) ----
  async listJobs(status?: JobStatus): Promise<Job[]> {
    return (await this.request<{ items: Job[] }>("GET", "/jobs", { query: { status } })).items;
  }

  async retryJob(id: number): Promise<void> {
    await this.request<void>("POST", `/jobs/${id}/retry`);
  }
}
```

Append to `web/packages/sdk/src/index.ts`:
```ts
export * from "./client.js";
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `pnpm -C web/packages/sdk exec vitest run src/client.test.ts && pnpm -C web/packages/sdk typecheck`
Expected: PASS, with 35 endpoint cases plus 10 other tests in `src/client.test.ts`. Typecheck exits 0.

- [ ] **Step 5: Commit**

```bash
git add web/packages/sdk/src/client.ts web/packages/sdk/src/client.test.ts web/packages/sdk/src/index.ts
git commit -m "feat(sdk): typed OpenFormsClient for the v1 HTTP API"
```

---

### Task 4: Shared submission logic (`visible`, `validateSubmission`)

**Parallel group:** A (with Tasks 3 and 8). This task appends one export line to `src/index.ts`.

**Files:**
- Create: `web/packages/sdk/src/logic.ts`
- Modify: `web/packages/sdk/src/index.ts` (append `export * from "./logic.js";`)
- Test: `web/packages/sdk/src/logic.test.ts`

**Interfaces:**
- Consumes: `Problem` from `api-types.ts`. Reads `schemas/fixtures/visibility.json` and `schemas/fixtures/submission.json` from Plan 02.
- Produces:
```ts
type Values = Record<string, unknown>;
interface LogicField { key: string; type: string; required?: boolean; options?: readonly { value: string }[];
  validation?: { minLength?: number; maxLength?: number; pattern?: string; min?: number; max?: number };
  showIf?: { field: string; equals?: unknown; notEquals?: unknown; in?: readonly unknown[] } }
interface LogicForm { fields?: readonly LogicField[] }   // FormDefinition is assignable
interface ValidationResult { clean: Values | null; problems: Problem[] }
const FORM_ERROR_KEY = "_form";
function isProvided(v: unknown): boolean;
function visible(form: LogicForm, data: Values): Record<string, boolean>;
function validateSubmission(form: LogicForm, data: Values): ValidationResult;
function problemsToErrors(problems: readonly Problem[]): Record<string, string>; // "data.email" → { email: msg }; other paths → { _form: msg }
```

- [ ] **Step 1: Write the failing test**

`web/packages/sdk/src/logic.test.ts`:
```ts
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
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `pnpm -C web/packages/sdk exec vitest run src/logic.test.ts`
Expected: FAIL with `Failed to resolve import "./logic.js"`.

- [ ] **Step 3: Write the implementation**

`web/packages/sdk/src/logic.ts`:
```ts
// Client-side mirror of the Go `definition` package's Visible / ValidateSubmission (spec §5.4).
// The server is authoritative; this exists for UX and must pass schemas/fixtures/*.json.
import type { Problem } from "./api-types.js";

export type Values = Record<string, unknown>;

export interface LogicField {
  key: string;
  type: string;
  required?: boolean;
  options?: readonly { value: string }[];
  validation?: { minLength?: number; maxLength?: number; pattern?: string; min?: number; max?: number };
  showIf?: { field: string; equals?: unknown; notEquals?: unknown; in?: readonly unknown[] };
}

export interface LogicForm {
  fields?: readonly LogicField[];
}

export interface ValidationResult {
  clean: Values | null;
  problems: Problem[];
}

export const FORM_ERROR_KEY = "_form";

export const messages = {
  required: "This field is required",
  mustTick: "Please tick this box to continue",
  text: "Enter text",
  number: "Enter a number",
  boolean: "Choose yes or no",
  choice: "Choose one of the listed options",
  duplicate: "Each option can only be chosen once",
  email: "Enter a valid email address, like name@example.com",
  url: "Enter a full web address starting with http:// or https://",
  date: "Enter a real date in the format YYYY-MM-DD",
  pattern: "Use the format shown",
  minLength: (n: number) => `Use at least ${n} ${n === 1 ? "character" : "characters"}`,
  maxLength: (n: number) => `Use at most ${n} ${n === 1 ? "character" : "characters"}`,
  min: (n: number) => `Must be ${n} or more`,
  max: (n: number) => `Must be ${n} or less`,
};

export function isProvided(value: unknown): boolean {
  return !(value === undefined || value === null || value === "" || (Array.isArray(value) && value.length === 0));
}

export function jsonEqual(a: unknown, b: unknown): boolean {
  if (a === b) return true;
  if (Array.isArray(a) || Array.isArray(b)) {
    return Array.isArray(a) && Array.isArray(b) && a.length === b.length && a.every((x, i) => jsonEqual(x, b[i]));
  }
  if (a !== null && b !== null && typeof a === "object" && typeof b === "object") {
    const ka = Object.keys(a);
    const kb = Object.keys(b);
    return ka.length === kb.length && ka.every((k) => jsonEqual((a as Values)[k], (b as Values)[k]));
  }
  return false;
}

function conditionHolds(cond: NonNullable<LogicField["showIf"]>, controller: LogicField, value: unknown): boolean {
  const provided = isProvided(value);
  const multi = controller.type === "multiselect" && Array.isArray(value);
  const matches = (target: unknown) =>
    provided && (multi ? (value as unknown[]).some((v) => jsonEqual(v, target)) : jsonEqual(value, target));

  // null counts as absent, mirroring Go's `omitempty` on `any`.
  if (cond.equals != null) return matches(cond.equals);
  if (cond.notEquals != null) return !matches(cond.notEquals);
  if (Array.isArray(cond.in)) return cond.in.some((t) => matches(t));
  return true;
}

export function visible(form: LogicForm, data: Values): Record<string, boolean> {
  const out: Record<string, boolean> = {};
  const seen = new Map<string, LogicField>();
  for (const field of form.fields ?? []) {
    let shown = true;
    if (field.showIf) {
      const controller = seen.get(field.showIf.field);
      shown =
        controller !== undefined &&
        out[controller.key] === true &&
        conditionHolds(field.showIf, controller, data[controller.key]);
    }
    out[field.key] = shown;
    seen.set(field.key, field);
  }
  return out;
}

const DATE_RE = /^(\d{4})-(\d{2})-(\d{2})$/;
const EMAIL_RE = /^[^\s@<>()[\]\\,;:"]+@[^\s@<>()[\]\\,;:"]+$/;

function isDate(s: string): boolean {
  const m = DATE_RE.exec(s);
  if (!m) return false;
  const [y, mo, d] = [Number(m[1]), Number(m[2]), Number(m[3])];
  const dt = new Date(Date.UTC(y, mo - 1, d));
  return dt.getUTCFullYear() === y && dt.getUTCMonth() === mo - 1 && dt.getUTCDate() === d;
}

function isHttpUrl(s: string): boolean {
  try {
    const u = new URL(s);
    return (u.protocol === "http:" || u.protocol === "https:") && u.hostname !== "";
  } catch {
    return false;
  }
}

function matchesPattern(pattern: string, s: string): boolean {
  try {
    return new RegExp(pattern, "u").test(s);
  } catch {
    return true; // invalid patterns are rejected when the definition is saved; don't block respondents
  }
}

/** Returns an error message, or null when the (provided) value is valid. */
function checkValue(field: LogicField, value: unknown): string | null {
  const optionValues = new Set((field.options ?? []).map((o) => o.value));
  const v = field.validation;

  switch (field.type) {
    case "number": {
      if (typeof value !== "number" || !Number.isFinite(value)) return messages.number;
      if (v?.min != null && value < v.min) return messages.min(v.min);
      if (v?.max != null && value > v.max) return messages.max(v.max);
      return null;
    }
    case "checkbox":
      return typeof value === "boolean" ? null : messages.boolean;
    case "multiselect": {
      if (!Array.isArray(value) || !value.every((x) => typeof x === "string")) return messages.choice;
      if (new Set(value).size !== value.length) return messages.duplicate;
      return value.every((x) => optionValues.has(x)) ? null : messages.choice;
    }
  }

  if (typeof value !== "string") return messages.text;
  switch (field.type) {
    case "email":
      if (!EMAIL_RE.test(value)) return messages.email;
      break;
    case "url":
      if (!isHttpUrl(value)) return messages.url;
      break;
    case "date":
      if (!isDate(value)) return messages.date;
      break;
    case "select":
      if (!optionValues.has(value)) return messages.choice;
      break;
  }
  const length = Array.from(value).length;
  if (v?.minLength != null && length < v.minLength) return messages.minLength(v.minLength);
  if (v?.maxLength != null && length > v.maxLength) return messages.maxLength(v.maxLength);
  if (v?.pattern && !matchesPattern(v.pattern, value)) return messages.pattern;
  return null;
}

export function validateSubmission(form: LogicForm, data: Values): ValidationResult {
  const shown = visible(form, data);
  const clean: Values = {};
  const problems: Problem[] = [];

  for (const field of form.fields ?? []) {
    if (!shown[field.key]) continue;
    const value = data[field.key];
    const path = `data.${field.key}`;

    if (!isProvided(value)) {
      if (field.required) problems.push({ path, message: messages.required });
      continue;
    }
    const error = checkValue(field, value);
    if (error) {
      problems.push({ path, message: error });
      continue;
    }
    if (field.type === "checkbox" && field.required && value !== true) {
      problems.push({ path, message: messages.mustTick });
      continue;
    }
    clean[field.key] = value;
  }

  return problems.length > 0 ? { clean: null, problems } : { clean, problems };
}

export function problemsToErrors(problems: readonly Problem[]): Record<string, string> {
  const errors: Record<string, string> = {};
  for (const p of problems) {
    const key = p.path.startsWith("data.") ? p.path.slice("data.".length) : FORM_ERROR_KEY;
    if (!(key in errors)) errors[key] = p.message;
  }
  return errors;
}
```

Append to `web/packages/sdk/src/index.ts`:
```ts
export * from "./logic.js";
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `pnpm -C web/packages/sdk exec vitest run src/logic.test.ts && pnpm -C web/packages/sdk typecheck`
Expected: PASS. Every fixture case and the hand-written cases are green, and typecheck exits 0.

If a fixture case fails, read that case and the matching rule in spec §5.4, change `logic.ts` to match the fixture, and re-run. The fixtures are shared with Go and are authoritative. Record any semantic change in the commit message body.

- [ ] **Step 5: Commit**

```bash
git add web/packages/sdk/src/logic.ts web/packages/sdk/src/logic.test.ts web/packages/sdk/src/index.ts
git commit -m "feat(sdk): visibility and submission validation passing shared fixtures"
```

---

### Task 5: `@openforms/react` package and `useOpenForm`

**Files:**
- Create: `web/packages/react/package.json`
- Create: `web/packages/react/tsconfig.json`
- Create: `web/packages/react/tsconfig.build.json`
- Create: `web/packages/react/vitest.config.ts`
- Create: `web/packages/react/src/test-setup.ts`
- Create: `web/packages/react/src/test-fixtures.ts`
- Create: `web/packages/react/src/useOpenForm.ts`
- Create: `web/packages/react/src/index.ts`
- Test: `web/packages/react/src/useOpenForm.test.tsx`

**Interfaces:**
- Consumes: `OpenFormsClient`, `OpenFormsError`, `visible`, `validateSubmission`, `problemsToErrors`, `FORM_ERROR_KEY`, `Values`, `FormDefinition` and `PublicSubmitResult` from `@openforms/sdk`.
- Produces:
```ts
type OpenFormStatus = "loading" | "ready" | "submitting" | "submitted" | "error";
type SubmitHandler = (data: Values) => Promise<PublicSubmitResult | void> | PublicSubmitResult | void;
type UseOpenFormOptions =
  | { client: OpenFormsClient; slug: string; definition?: undefined; onSubmit?: undefined }
  | { definition: FormDefinition; onSubmit?: SubmitHandler; client?: undefined; slug?: undefined };
interface UseOpenFormResult {
  form: FormDefinition | null; status: OpenFormStatus; values: Values;
  setValue(key: string, value: unknown): void; visible: Record<string, boolean>;
  errors: Record<string, string>;   // field key → message; "_form" → form-level message
  submit(): Promise<boolean>;       // true when submitted
  result: PublicSubmitResult | undefined; loadError: Error | null; reset(): void;
}
function useOpenForm(options: UseOpenFormOptions): UseOpenFormResult;
```
  Test helpers in `test-fixtures.ts`: `jobForm: FormDefinition`, `API = "http://api.test"`, `submitResult: PublicSubmitResult`, `newClient(): OpenFormsClient`. Tasks 6 and 7 also import these.

- [ ] **Step 1: Create the package config and test helpers**

`web/packages/react/package.json`:
```json
{
  "name": "@openforms/react",
  "version": "0.1.0",
  "license": "MIT",
  "type": "module",
  "exports": { ".": "./src/index.ts", "./styles.css": "./src/styles.css" },
  "types": "./src/index.ts",
  "sideEffects": ["*.css"],
  "files": ["dist"],
  "publishConfig": {
    "exports": {
      ".": { "types": "./dist/index.d.ts", "import": "./dist/index.js" },
      "./styles.css": "./dist/styles.css"
    },
    "types": "./dist/index.d.ts"
  },
  "scripts": {
    "build": "tsc -p tsconfig.build.json && node -e \"require('node:fs').copyFileSync('src/styles.css','dist/styles.css')\"",
    "test": "vitest run",
    "typecheck": "tsc -p tsconfig.json"
  },
  "dependencies": { "@openforms/sdk": "workspace:*" },
  "peerDependencies": { "react": "^18.2.0", "react-dom": "^18.2.0" },
  "devDependencies": {
    "@testing-library/dom": "^10.4.0",
    "@testing-library/jest-dom": "^6.6.3",
    "@testing-library/react": "^16.2.0",
    "@testing-library/user-event": "^14.6.1",
    "@types/node": "^22.10.0",
    "@types/react": "^18.3.18",
    "@types/react-dom": "^18.3.5",
    "@vitejs/plugin-react": "^4.3.4",
    "jsdom": "^26.0.0",
    "msw": "^2.7.0",
    "react": "^18.3.1",
    "react-dom": "^18.3.1",
    "typescript": "^5.7.3",
    "vite": "^6.1.0",
    "vitest": "^3.0.5"
  }
}
```

`web/packages/react/tsconfig.json`:
```json
{
  "extends": "../../tsconfig.base.json",
  "compilerOptions": { "types": ["node"] },
  "include": ["src", "vitest.config.ts"]
}
```

`web/packages/react/tsconfig.build.json`. The `paths` entry points at the SDK's emitted declarations so that `tsc` does not pull SDK sources into this package's `rootDir`. `pnpm -r` builds the SDK first because this package depends on it.
```json
{
  "extends": "../../tsconfig.base.json",
  "compilerOptions": {
    "noEmit": false,
    "declaration": true,
    "outDir": "dist",
    "rootDir": "src",
    "types": [],
    "paths": { "@openforms/sdk": ["../sdk/dist/index.d.ts"] }
  },
  "include": ["src"],
  "exclude": ["src/**/*.test.ts", "src/**/*.test.tsx", "src/test-setup.ts", "src/test-fixtures.ts"]
}
```

`web/packages/react/vitest.config.ts`:
```ts
import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

export default defineConfig({
  plugins: [react()],
  test: { environment: "jsdom", setupFiles: ["./src/test-setup.ts"], include: ["src/**/*.test.{ts,tsx}"] },
});
```

`web/packages/react/src/test-setup.ts`:
```ts
import "@testing-library/jest-dom/vitest";
import { cleanup } from "@testing-library/react";
import { afterEach } from "vitest";

afterEach(() => cleanup());
```

`web/packages/react/src/test-fixtures.ts`:
```ts
import { OpenFormsClient, type FormDefinition, type PublicSubmitResult } from "@openforms/sdk";

export const API = "http://api.test";

export const newClient = () => new OpenFormsClient({ baseUrl: API });

export const jobForm: FormDefinition = {
  slug: "job-application",
  title: "Job application",
  description: "Apply to join the team.",
  workflow: "hiring",
  settings: { public: true, submitLabel: "Send application", confirmationMessage: "Thanks! We'll be in touch." },
  fields: [
    { key: "name", type: "text", label: "Full name", required: true, validation: { minLength: 2, maxLength: 100 } },
    { key: "email", type: "email", label: "Email", required: true, help: "We only use this to reply to you." },
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
    { key: "portfolio", type: "url", label: "Portfolio URL", required: true, showIf: { field: "role", equals: "designer" } },
    { key: "years", type: "number", label: "Years of experience", validation: { min: 0, max: 50 } },
    {
      key: "skills",
      type: "multiselect",
      label: "Skills",
      options: [
        { value: "go", label: "Go" },
        { value: "ts", label: "TypeScript" },
      ],
    },
    { key: "start", type: "date", label: "Earliest start date" },
    { key: "cover", type: "textarea", label: "Cover letter", placeholder: "Tell us about yourself" },
    { key: "consent", type: "checkbox", label: "I agree to the privacy policy", required: true },
  ],
};

export const submitResult: PublicSubmitResult = {
  id: "sub-1",
  state: "new",
  stateLabel: "New",
  receiptToken: "tok-123",
  confirmationMessage: "Thanks! We'll be in touch.",
};
```

Run `pnpm -C web install`.

- [ ] **Step 2: Write the failing test**

`web/packages/react/src/useOpenForm.test.tsx`:
```tsx
import { act, renderHook, waitFor } from "@testing-library/react";
import { OpenFormsError } from "@openforms/sdk";
import { delay, http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import { API, jobForm, newClient, submitResult } from "./test-fixtures.js";
import { useOpenForm } from "./useOpenForm.js";

const posts: unknown[] = [];
const server = setupServer(
  http.get(`${API}/api/v1/public/forms/:slug`, ({ params }) =>
    params.slug === "job-application"
      ? HttpResponse.json({ form: jobForm })
      : HttpResponse.json({ error: { code: "not_found", message: "form not found" } }, { status: 404 }),
  ),
  http.post(`${API}/api/v1/public/forms/:slug/submissions`, async ({ request }) => {
    posts.push(await request.json());
    await delay(30);
    return HttpResponse.json(submitResult, { status: 201 });
  }),
);

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => {
  server.resetHandlers();
  posts.length = 0;
});
afterAll(() => server.close());

const client = newClient();

async function readyHook() {
  const hook = renderHook(() => useOpenForm({ client, slug: "job-application" }));
  await waitFor(() => expect(hook.result.current.status).toBe("ready"));
  return hook;
}

function fillValid(set: (k: string, v: unknown) => void) {
  set("name", "Ada Lovelace");
  set("email", "ada@example.com");
  set("role", "engineer");
  set("consent", true);
}

describe("useOpenForm (slug mode)", () => {
  it("loads the public form", async () => {
    const { result } = renderHook(() => useOpenForm({ client, slug: "job-application" }));
    expect(result.current.status).toBe("loading");
    await waitFor(() => expect(result.current.status).toBe("ready"));
    expect(result.current.form?.title).toBe("Job application");
    expect(result.current.visible).toMatchObject({ name: true, portfolio: false });
  });

  it("reports a load error for unavailable forms", async () => {
    const { result } = renderHook(() => useOpenForm({ client, slug: "missing" }));
    await waitFor(() => expect(result.current.status).toBe("error"));
    expect(result.current.loadError).toBeInstanceOf(OpenFormsError);
    expect((result.current.loadError as OpenFormsError).code).toBe("not_found");
  });

  it("validates locally without sending a request", async () => {
    const { result } = await readyHook();
    let ok = true;
    await act(async () => {
      ok = await result.current.submit();
    });
    expect(ok).toBe(false);
    expect(Object.keys(result.current.errors).sort()).toEqual(["consent", "email", "name", "role"]);
    expect(posts).toEqual([]);
  });

  it("clears a field's error when its value changes", async () => {
    const { result } = await readyHook();
    await act(async () => {
      await result.current.submit();
    });
    act(() => result.current.setValue("name", "Ada"));
    expect(result.current.errors.name).toBeUndefined();
    expect(result.current.errors.email).toBeDefined();
  });

  it("submits clean data and exposes the result", async () => {
    const { result } = await readyHook();
    act(() => fillValid(result.current.setValue));
    let ok = false;
    await act(async () => {
      ok = await result.current.submit();
    });
    expect(ok).toBe(true);
    expect(result.current.status).toBe("submitted");
    expect(result.current.result).toEqual(submitResult);
    expect(posts).toEqual([{ data: { name: "Ada Lovelace", email: "ada@example.com", role: "engineer", consent: true } }]);
  });

  it("drops values of fields that became hidden", async () => {
    const { result } = await readyHook();
    act(() => {
      fillValid(result.current.setValue);
      result.current.setValue("role", "designer");
      result.current.setValue("portfolio", "https://ada.design");
    });
    act(() => result.current.setValue("role", "engineer"));
    await act(async () => {
      await result.current.submit();
    });
    expect(posts).toHaveLength(1);
    expect((posts[0] as { data: Record<string, unknown> }).data).not.toHaveProperty("portfolio");
  });

  it("ignores a second submit while one is in flight", async () => {
    const { result } = await readyHook();
    act(() => fillValid(result.current.setValue));
    await act(async () => {
      await Promise.all([result.current.submit(), result.current.submit()]);
    });
    expect(posts).toHaveLength(1);
  });

  it("maps server validation errors onto fields", async () => {
    server.use(
      http.post(`${API}/api/v1/public/forms/:slug/submissions`, () =>
        HttpResponse.json(
          {
            error: {
              code: "validation_failed",
              message: "submission is invalid",
              details: [{ path: "data.email", message: "That email domain is not accepted" }],
            },
          },
          { status: 422 },
        ),
      ),
    );
    const { result } = await readyHook();
    act(() => fillValid(result.current.setValue));
    let ok = true;
    await act(async () => {
      ok = await result.current.submit();
    });
    expect(ok).toBe(false);
    expect(result.current.status).toBe("ready");
    expect(result.current.errors).toEqual({ email: "That email domain is not accepted" });
  });

  it("shows a friendly form-level error for rate limiting and other failures", async () => {
    server.use(
      http.post(`${API}/api/v1/public/forms/:slug/submissions`, () =>
        HttpResponse.json({ error: { code: "rate_limited", message: "slow down" } }, { status: 429 }),
      ),
    );
    const { result } = await readyHook();
    act(() => fillValid(result.current.setValue));
    await act(async () => {
      await result.current.submit();
    });
    expect(result.current.errors._form).toMatch(/too many submissions/i);
  });
});

describe("useOpenForm (definition mode)", () => {
  it("is ready immediately and passes clean data to onSubmit", async () => {
    const onSubmit = vi.fn(async () => undefined);
    const { result } = renderHook(() => useOpenForm({ definition: jobForm, onSubmit }));
    expect(result.current.status).toBe("ready");
    act(() => fillValid(result.current.setValue));
    await act(async () => {
      await result.current.submit();
    });
    expect(onSubmit).toHaveBeenCalledWith({ name: "Ada Lovelace", email: "ada@example.com", role: "engineer", consent: true });
    expect(result.current.status).toBe("submitted");
    act(() => result.current.reset());
    expect(result.current.status).toBe("ready");
    expect(result.current.values).toEqual({});
  });
});
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `pnpm -C web/packages/react exec vitest run src/useOpenForm.test.tsx`
Expected: FAIL with `Failed to resolve import "./useOpenForm.js"`.

- [ ] **Step 4: Write the implementation**

`web/packages/react/src/useOpenForm.ts`:
```ts
import {
  FORM_ERROR_KEY,
  OpenFormsError,
  problemsToErrors,
  validateSubmission,
  visible as computeVisible,
  type FormDefinition,
  type OpenFormsClient,
  type PublicSubmitResult,
  type Values,
} from "@openforms/sdk";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";

export type OpenFormStatus = "loading" | "ready" | "submitting" | "submitted" | "error";

export type SubmitHandler = (data: Values) => Promise<PublicSubmitResult | void> | PublicSubmitResult | void;

export type UseOpenFormOptions =
  | { client: OpenFormsClient; slug: string; definition?: undefined; onSubmit?: undefined }
  | { definition: FormDefinition; onSubmit?: SubmitHandler; client?: undefined; slug?: undefined };

export interface UseOpenFormResult {
  form: FormDefinition | null;
  status: OpenFormStatus;
  values: Values;
  setValue(key: string, value: unknown): void;
  visible: Record<string, boolean>;
  errors: Record<string, string>;
  submit(): Promise<boolean>;
  result: PublicSubmitResult | undefined;
  loadError: Error | null;
  reset(): void;
}

const GENERIC_FAILURE = "Something went wrong while sending your response. Please try again.";
const RATE_LIMITED = "Too many submissions from your network. Please wait a minute and try again.";

export function useOpenForm(options: UseOpenFormOptions): UseOpenFormResult {
  const { client, slug, definition } = options;
  const onSubmitRef = useRef(options.onSubmit);
  onSubmitRef.current = options.onSubmit;

  const [loaded, setLoaded] = useState<FormDefinition | null>(null);
  const [loadError, setLoadError] = useState<Error | null>(null);
  const [values, setValues] = useState<Values>({});
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [phase, setPhase] = useState<"idle" | "submitting" | "submitted">("idle");
  const [result, setResult] = useState<PublicSubmitResult | undefined>(undefined);
  const inFlight = useRef(false);

  useEffect(() => {
    if (definition || !client || !slug) return;
    let cancelled = false;
    setLoaded(null);
    setLoadError(null);
    client.getPublicForm(slug).then(
      (form) => {
        if (!cancelled) setLoaded(form);
      },
      (err: unknown) => {
        if (!cancelled) setLoadError(err instanceof Error ? err : new Error(String(err)));
      },
    );
    return () => {
      cancelled = true;
    };
  }, [client, slug, definition]);

  const form = definition ?? loaded;
  const visibleMap = useMemo(() => (form ? computeVisible(form, values) : {}), [form, values]);
  const status: OpenFormStatus = loadError ? "error" : !form ? "loading" : phase === "idle" ? "ready" : phase;

  const setValue = useCallback((key: string, value: unknown) => {
    setValues((prev) => ({ ...prev, [key]: value }));
    setErrors((prev) => {
      if (!(key in prev) && !(FORM_ERROR_KEY in prev)) return prev;
      const next = { ...prev };
      delete next[key];
      delete next[FORM_ERROR_KEY];
      return next;
    });
  }, []);

  const submit = useCallback(async (): Promise<boolean> => {
    if (!form || inFlight.current) return false;
    const { clean, problems } = validateSubmission(form, values);
    if (!clean) {
      setErrors(problemsToErrors(problems));
      return false;
    }

    inFlight.current = true;
    setPhase("submitting");
    setErrors({});
    try {
      let res: PublicSubmitResult | undefined;
      if (definition) {
        res = (await onSubmitRef.current?.(clean)) ?? undefined;
      } else if (client && slug) {
        res = await client.submitPublic(slug, clean);
      }
      setResult(res);
      setPhase("submitted");
      return true;
    } catch (err) {
      setPhase("idle");
      if (err instanceof OpenFormsError && err.code === "validation_failed" && err.details.length > 0) {
        setErrors(problemsToErrors(err.details));
      } else if (err instanceof OpenFormsError && err.code === "rate_limited") {
        setErrors({ [FORM_ERROR_KEY]: RATE_LIMITED });
      } else {
        setErrors({ [FORM_ERROR_KEY]: GENERIC_FAILURE });
      }
      return false;
    } finally {
      inFlight.current = false;
    }
  }, [form, values, definition, client, slug]);

  const reset = useCallback(() => {
    setValues({});
    setErrors({});
    setResult(undefined);
    setPhase("idle");
  }, []);

  return { form, status, values, setValue, visible: visibleMap, errors, submit, result, loadError, reset };
}
```

`web/packages/react/src/index.ts`:
```ts
export * from "./useOpenForm.js";
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `pnpm -C web/packages/react exec vitest run src/useOpenForm.test.tsx && pnpm -C web/packages/react typecheck`
Expected: PASS, with 10 tests in `src/useOpenForm.test.tsx`. Typecheck exits 0.

- [ ] **Step 6: Commit**

```bash
git add web/packages/react web/pnpm-lock.yaml
git commit -m "feat(react): useOpenForm hook with client validation and server error mapping"
```

---

### Task 6: `<OpenForm>`, default field components, `styles.css`

**Parallel group:** B (with Task 7). This task appends one export line to `src/index.ts`. `styles.css` in this task also holds the StatusTracker styles, so Task 7 touches no CSS.

**Files:**
- Create: `web/packages/react/src/fields.tsx`
- Create: `web/packages/react/src/OpenForm.tsx`
- Create: `web/packages/react/src/styles.css`
- Modify: `web/packages/react/src/index.ts` (append `export * from "./OpenForm.js";` and `export * from "./fields.js";`)
- Test: `web/packages/react/src/OpenForm.test.tsx`

**Interfaces:**
- Consumes: `useOpenForm` and `SubmitHandler` from Task 5; `jobForm`, `API`, `newClient` and `submitResult` from `test-fixtures.ts`.
- Produces:
```ts
interface FieldProps { field: Field; id: string; value: unknown; onChange(value: unknown): void;
  invalid: boolean; describedBy?: string; required: boolean; disabled: boolean }
type FieldComponent = ComponentType<FieldProps>;
type FieldComponents = Partial<Record<FieldType, FieldComponent>>;
const TextInput, TextArea, EmailInput, NumberInput, Select, MultiSelect, Checkbox, DateInput, UrlInput: FieldComponent;
const defaultComponents: Record<FieldType, FieldComponent>;
interface OpenFormProps {
  client?: OpenFormsClient; slug?: string; definition?: FormDefinition; onSubmit?: SubmitHandler;
  onSubmitted?: (result: PublicSubmitResult | undefined) => void;
  onLoaded?: (form: FormDefinition) => void;
  components?: FieldComponents; className?: string; hideHeader?: boolean;
  renderConfirmation?: (result: PublicSubmitResult | undefined, form: FormDefinition) => ReactNode;
  renderLoadError?: (error: Error) => ReactNode;
}
function OpenForm(props: OpenFormProps): JSX.Element;
```
  The DOM contract, which other plans' tests may rely on:
  - The root has class `.of-form`, and the submit button has class `.of-submit`.
  - The error summary is `role="alert"` with class `.of-error-summary`, and it takes focus after a failed submit.
  - Confirmation is rendered as `role="status"` with class `.of-confirmation`.
  - Each input's id is `${useId-derived prefix}${field.key}`.

- [ ] **Step 1: Write the failing test**

`web/packages/react/src/OpenForm.test.tsx`:
```tsx
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { delay, http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import type { FieldProps } from "./fields.js";
import { OpenForm } from "./OpenForm.js";
import { API, jobForm, newClient, submitResult } from "./test-fixtures.js";

const posts: unknown[] = [];
const server = setupServer(
  http.get(`${API}/api/v1/public/forms/:slug`, ({ params }) =>
    params.slug === "job-application"
      ? HttpResponse.json({ form: jobForm })
      : HttpResponse.json({ error: { code: "not_found", message: "form not found" } }, { status: 404 }),
  ),
  http.post(`${API}/api/v1/public/forms/:slug/submissions`, async ({ request }) => {
    posts.push(await request.json());
    await delay(50);
    return HttpResponse.json(submitResult, { status: 201 });
  }),
);
beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => {
  server.resetHandlers();
  posts.length = 0;
});
afterAll(() => server.close());

const client = newClient();

async function renderJobForm(extra: Partial<React.ComponentProps<typeof OpenForm>> = {}) {
  const user = userEvent.setup();
  render(<OpenForm client={client} slug="job-application" {...extra} />);
  await screen.findByRole("heading", { name: "Job application" });
  return user;
}

async function fillValid(user: ReturnType<typeof userEvent.setup>) {
  await user.type(screen.getByLabelText(/Full name/), "Ada Lovelace");
  await user.type(screen.getByLabelText(/^Email/), "ada@example.com");
  await user.selectOptions(screen.getByLabelText(/^Role/), "engineer");
  await user.click(screen.getByLabelText(/I agree to the privacy policy/));
}

describe("<OpenForm>", () => {
  it("renders the header, labelled fields and help text", async () => {
    await renderJobForm();
    expect(screen.getByText("Apply to join the team.")).toBeInTheDocument();
    const email = screen.getByLabelText(/^Email/);
    expect(email).toHaveAttribute("type", "email");
    expect(email).toHaveAccessibleDescription("We only use this to reply to you.");
    expect(screen.getByRole("group", { name: /Skills/ })).toBeInTheDocument();
    expect(screen.getByLabelText(/Cover letter/)).toHaveAttribute("placeholder", "Tell us about yourself");
    expect(screen.getByRole("button", { name: "Send application" })).toBeInTheDocument();
  });

  it("shows conditional fields only when their condition holds", async () => {
    const user = await renderJobForm();
    expect(screen.queryByLabelText(/Portfolio URL/)).not.toBeInTheDocument();
    await user.selectOptions(screen.getByLabelText(/^Role/), "designer");
    expect(screen.getByLabelText(/Portfolio URL/)).toBeInTheDocument();
  });

  it("focuses an error summary and marks invalid fields on a failed submit", async () => {
    const user = await renderJobForm();
    await user.click(screen.getByRole("button", { name: "Send application" }));
    const summary = await screen.findByRole("alert");
    expect(summary).toHaveTextContent("Please fix 4 fields before submitting.");
    await waitFor(() => expect(summary).toHaveFocus());
    const name = screen.getByLabelText(/Full name/);
    expect(name).toHaveAttribute("aria-invalid", "true");
    expect(name).toHaveAccessibleDescription("This field is required");
    expect(posts).toEqual([]);

    await user.click(screen.getByRole("link", { name: /Full name/ }));
    expect(name).toHaveFocus();
  });

  it("submits and shows the confirmation", async () => {
    const onSubmitted = vi.fn();
    const user = await renderJobForm({ onSubmitted });
    await fillValid(user);
    await user.type(screen.getByLabelText(/Years of experience/), "7");
    await user.click(screen.getByLabelText("TypeScript"));
    await user.click(screen.getByRole("button", { name: "Send application" }));
    expect(await screen.findByRole("status")).toHaveTextContent("Thanks! We'll be in touch.");
    expect(onSubmitted).toHaveBeenCalledTimes(1);
    expect(onSubmitted).toHaveBeenCalledWith(submitResult);
    expect(posts).toEqual([
      {
        data: { name: "Ada Lovelace", email: "ada@example.com", role: "engineer", years: 7, skills: ["ts"], consent: true },
      },
    ]);
  });

  it("double-clicking submit sends one request", async () => {
    const user = await renderJobForm();
    await fillValid(user);
    await user.dblClick(screen.getByRole("button", { name: "Send application" }));
    await screen.findByRole("status");
    expect(posts).toHaveLength(1);
  });

  it("shows server-side field errors", async () => {
    server.use(
      http.post(`${API}/api/v1/public/forms/:slug/submissions`, () =>
        HttpResponse.json(
          {
            error: {
              code: "validation_failed",
              message: "submission is invalid",
              details: [{ path: "data.email", message: "That email domain is not accepted" }],
            },
          },
          { status: 422 },
        ),
      ),
    );
    const user = await renderJobForm();
    await fillValid(user);
    await user.click(screen.getByRole("button", { name: "Send application" }));
    const email = screen.getByLabelText(/^Email/);
    await waitFor(() => expect(email).toHaveAttribute("aria-invalid", "true"));
    expect(email).toHaveAccessibleDescription("We only use this to reply to you. That email domain is not accepted");
    expect(screen.getByRole("button", { name: "Send application" })).toBeEnabled();
  });

  it("renders a load error for unavailable forms", async () => {
    render(<OpenForm client={client} slug="missing" />);
    expect(await screen.findByRole("alert")).toHaveTextContent("This form isn't available right now.");
  });

  it("uses custom confirmation and load-error renderers", async () => {
    render(<OpenForm client={client} slug="missing" renderLoadError={() => <p>Custom unavailable</p>} />);
    expect(await screen.findByText("Custom unavailable")).toBeInTheDocument();
  });

  it("renders from a definition without the network and calls onSubmit", async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();
    render(
      <OpenForm
        definition={jobForm}
        onSubmit={onSubmit}
        hideHeader
        renderConfirmation={() => <p>Preview submitted</p>}
      />,
    );
    expect(screen.queryByRole("heading")).not.toBeInTheDocument();
    await fillValid(user);
    await user.click(screen.getByRole("button", { name: "Send application" }));
    expect(await screen.findByText("Preview submitted")).toBeInTheDocument();
    expect(onSubmit).toHaveBeenCalledWith({ name: "Ada Lovelace", email: "ada@example.com", role: "engineer", consent: true });
  });

  it("lets callers override field components by type", async () => {
    function Stars({ id, value, onChange, field }: FieldProps) {
      return (
        <input id={id} aria-label={`${field.label} stars`} value={String(value ?? "")} onChange={(e) => onChange(e.target.value)} />
      );
    }
    render(<OpenForm definition={jobForm} components={{ text: Stars }} />);
    expect(screen.getByLabelText("Full name stars")).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `pnpm -C web/packages/react exec vitest run src/OpenForm.test.tsx`
Expected: FAIL with `Failed to resolve import "./fields.js"`.

- [ ] **Step 3: Write the field components**

`web/packages/react/src/fields.tsx`:
```tsx
import type { Field, FieldType } from "@openforms/sdk";
import type { ComponentType } from "react";

export interface FieldProps {
  field: Field;
  /** DOM id for the primary input; the surrounding <label htmlFor> points at it. */
  id: string;
  value: unknown;
  onChange(value: unknown): void;
  invalid: boolean;
  /** Space-separated ids of help/error text, for aria-describedby. */
  describedBy?: string;
  required: boolean;
  disabled: boolean;
}

export type FieldComponent = ComponentType<FieldProps>;
export type FieldComponents = Partial<Record<FieldType, FieldComponent>>;

const asString = (v: unknown) => (typeof v === "string" ? v : "");

function inputProps(p: FieldProps) {
  return {
    id: p.id,
    name: p.field.key,
    "aria-invalid": p.invalid ? ("true" as const) : undefined,
    "aria-describedby": p.describedBy,
    "aria-required": p.required ? ("true" as const) : undefined,
    disabled: p.disabled,
  };
}

function StringInput({ type, ...p }: FieldProps & { type: string }) {
  return (
    <input
      className="of-input"
      type={type}
      {...inputProps(p)}
      placeholder={p.field.placeholder}
      value={asString(p.value)}
      onChange={(e) => p.onChange(e.target.value)}
    />
  );
}

export function TextInput(p: FieldProps) {
  return <StringInput type="text" {...p} />;
}

export function EmailInput(p: FieldProps) {
  return <StringInput type="email" {...p} />;
}

export function UrlInput(p: FieldProps) {
  return <StringInput type="url" {...p} />;
}

export function DateInput(p: FieldProps) {
  return <StringInput type="date" {...p} />;
}

export function TextArea(p: FieldProps) {
  return (
    <textarea
      className="of-input of-textarea"
      rows={5}
      {...inputProps(p)}
      placeholder={p.field.placeholder}
      value={asString(p.value)}
      onChange={(e) => p.onChange(e.target.value)}
    />
  );
}

export function NumberInput(p: FieldProps) {
  // Uncontrolled so partially typed numbers ("1.") are not clobbered by re-renders.
  return (
    <input
      className="of-input"
      type="number"
      inputMode="decimal"
      {...inputProps(p)}
      placeholder={p.field.placeholder}
      defaultValue={typeof p.value === "number" ? String(p.value) : ""}
      onChange={(e) => {
        const raw = e.target.value;
        const n = raw === "" ? undefined : Number(raw);
        p.onChange(n === undefined || Number.isNaN(n) ? undefined : n);
      }}
    />
  );
}

export function Select(p: FieldProps) {
  return (
    <select
      className="of-input of-select"
      {...inputProps(p)}
      value={asString(p.value)}
      onChange={(e) => p.onChange(e.target.value === "" ? undefined : e.target.value)}
    >
      <option value="">{p.field.placeholder || "Select…"}</option>
      {(p.field.options ?? []).map((o) => (
        <option key={o.value} value={o.value}>
          {o.label}
        </option>
      ))}
    </select>
  );
}

export function MultiSelect(p: FieldProps) {
  const options = p.field.options ?? [];
  const selected = Array.isArray(p.value) ? (p.value as string[]) : [];
  const toggle = (value: string, on: boolean) => {
    // Keep the option order stable regardless of click order.
    const next = options.map((o) => o.value).filter((v) => (v === value ? on : selected.includes(v)));
    p.onChange(next.length > 0 ? next : undefined);
  };
  return (
    <div className="of-choices">
      {options.map((o, i) => (
        <label key={o.value} className="of-choice">
          <input
            type="checkbox"
            className="of-checkbox"
            id={i === 0 ? p.id : `${p.id}-${i}`}
            name={p.field.key}
            value={o.value}
            checked={selected.includes(o.value)}
            disabled={p.disabled}
            aria-invalid={p.invalid ? "true" : undefined}
            onChange={(e) => toggle(o.value, e.target.checked)}
          />
          <span>{o.label}</span>
        </label>
      ))}
    </div>
  );
}

export function Checkbox(p: FieldProps) {
  return (
    <input
      className="of-checkbox"
      type="checkbox"
      {...inputProps(p)}
      checked={p.value === true}
      onChange={(e) => p.onChange(e.target.checked)}
    />
  );
}

export const defaultComponents: Record<FieldType, FieldComponent> = {
  text: TextInput,
  textarea: TextArea,
  email: EmailInput,
  number: NumberInput,
  select: Select,
  multiselect: MultiSelect,
  checkbox: Checkbox,
  date: DateInput,
  url: UrlInput,
};
```

- [ ] **Step 4: Write `<OpenForm>`**

`web/packages/react/src/OpenForm.tsx`:
```tsx
import type { Field, FormDefinition, OpenFormsClient, PublicSubmitResult } from "@openforms/sdk";
import { useEffect, useId, useRef, useState, type FormEvent, type ReactNode } from "react";
import { defaultComponents, TextInput, type FieldComponents } from "./fields.js";
import { useOpenForm, type SubmitHandler } from "./useOpenForm.js";

export interface OpenFormProps {
  client?: OpenFormsClient;
  slug?: string;
  definition?: FormDefinition;
  onSubmit?: SubmitHandler;
  onSubmitted?: (result: PublicSubmitResult | undefined) => void;
  onLoaded?: (form: FormDefinition) => void;
  components?: FieldComponents;
  className?: string;
  hideHeader?: boolean;
  renderConfirmation?: (result: PublicSubmitResult | undefined, form: FormDefinition) => ReactNode;
  renderLoadError?: (error: Error) => ReactNode;
}

const FORM_KEY = "_form";
const DEFAULT_CONFIRMATION = "Thanks! Your response has been recorded.";

const cx = (...names: Array<string | false | null | undefined>) => names.filter(Boolean).join(" ");

export function OpenForm(props: OpenFormProps) {
  const { client, slug, definition, onSubmit, components, className, hideHeader } = props;
  if (!definition && !(client && slug)) {
    throw new Error("<OpenForm> needs either `definition` or both `client` and `slug`.");
  }
  const state = useOpenForm(definition ? { definition, onSubmit } : { client: client!, slug: slug! });
  const { form, status, values, setValue, visible, errors, result, loadError } = state;

  const idBase = `of${useId().replace(/:/g, "")}-`;
  const fieldId = (key: string) => `${idBase}${key}`;
  const titleId = `${idBase}title`;

  const summaryRef = useRef<HTMLDivElement>(null);
  const [failedAttempts, setFailedAttempts] = useState(0);
  useEffect(() => {
    if (failedAttempts > 0) summaryRef.current?.focus();
  }, [failedAttempts]);

  const onLoadedRef = useRef(props.onLoaded);
  onLoadedRef.current = props.onLoaded;
  useEffect(() => {
    if (form) onLoadedRef.current?.(form);
  }, [form]);

  const notified = useRef(false);
  const onSubmittedRef = useRef(props.onSubmitted);
  onSubmittedRef.current = props.onSubmitted;
  useEffect(() => {
    if (status === "submitted" && !notified.current) {
      notified.current = true;
      onSubmittedRef.current?.(result);
    }
    if (status !== "submitted") notified.current = false;
  }, [status, result]);

  if (status === "error") {
    return (
      <div className={cx("of-form", className)} role="alert">
        {props.renderLoadError && loadError ? (
          props.renderLoadError(loadError)
        ) : (
          <p className="of-load-error">This form isn't available right now.</p>
        )}
      </div>
    );
  }

  if (!form) {
    return (
      <div className={cx("of-form", "of-loading", className)} aria-busy="true">
        Loading form…
      </div>
    );
  }

  if (status === "submitted") {
    return (
      <div className={cx("of-form", className)}>
        {props.renderConfirmation ? (
          props.renderConfirmation(result, form)
        ) : (
          <div className="of-confirmation" role="status">
            <p>{result?.confirmationMessage || form.settings?.confirmationMessage || DEFAULT_CONFIRMATION}</p>
          </div>
        )}
      </div>
    );
  }

  const fields: Field[] = form.fields ?? [];
  const labelFor = (key: string) => fields.find((f) => f.key === key)?.label ?? key;
  const fieldErrors = Object.entries(errors).filter(([key]) => key !== FORM_KEY);
  const formError = errors[FORM_KEY];
  const submitting = status === "submitting";

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const ok = await state.submit();
    if (!ok) setFailedAttempts((n) => n + 1);
  }

  function renderField(field: Field) {
    const id = fieldId(field.key);
    const error = errors[field.key];
    const helpId = field.help ? `${id}-help` : undefined;
    const errorId = error ? `${id}-error` : undefined;
    const describedBy = [helpId, errorId].filter(Boolean).join(" ") || undefined;
    const Component = components?.[field.type] ?? defaultComponents[field.type] ?? TextInput;
    const required = Boolean(field.required);

    const input = (
      <Component
        field={field}
        id={id}
        value={values[field.key]}
        onChange={(v) => setValue(field.key, v)}
        invalid={Boolean(error)}
        describedBy={describedBy}
        required={required}
        disabled={submitting}
      />
    );
    const marker = required ? (
      <span className="of-required" aria-hidden="true">
        {" *"}
      </span>
    ) : null;
    const help = field.help ? (
      <p id={helpId} className="of-help">
        {field.help}
      </p>
    ) : null;
    const errorText = error ? (
      <p id={errorId} className="of-field-error">
        {error}
      </p>
    ) : null;
    const wrapperClass = cx("of-field", `of-field-${field.type}`, error && "of-field-invalid");

    if (field.type === "multiselect") {
      return (
        <fieldset key={field.key} className={wrapperClass} aria-describedby={describedBy}>
          <legend className="of-label">
            {field.label}
            {marker}
          </legend>
          {help}
          {input}
          {errorText}
        </fieldset>
      );
    }
    if (field.type === "checkbox") {
      return (
        <div key={field.key} className={wrapperClass}>
          <div className="of-checkbox-row">
            {input}
            <label htmlFor={id} className="of-label">
              {field.label}
              {marker}
            </label>
          </div>
          {help}
          {errorText}
        </div>
      );
    }
    return (
      <div key={field.key} className={wrapperClass}>
        <label htmlFor={id} className="of-label">
          {field.label}
          {marker}
        </label>
        {help}
        {input}
        {errorText}
      </div>
    );
  }

  return (
    <form
      className={cx("of-form", className)}
      noValidate
      onSubmit={handleSubmit}
      aria-labelledby={hideHeader ? undefined : titleId}
      aria-busy={submitting || undefined}
    >
      {!hideHeader && (
        <header className="of-header">
          <h2 id={titleId} className="of-title">
            {form.title}
          </h2>
          {form.description && <p className="of-description">{form.description}</p>}
        </header>
      )}

      {(fieldErrors.length > 0 || formError) && (
        <div ref={summaryRef} className="of-error-summary" role="alert" tabIndex={-1}>
          {fieldErrors.length > 0 ? (
            <>
              <p className="of-error-summary-title">
                Please fix {fieldErrors.length} {fieldErrors.length === 1 ? "field" : "fields"} before submitting.
              </p>
              <ul>
                {fieldErrors.map(([key, message]) => (
                  <li key={key}>
                    <a
                      href={`#${fieldId(key)}`}
                      onClick={(e) => {
                        e.preventDefault();
                        document.getElementById(fieldId(key))?.focus();
                      }}
                    >
                      {labelFor(key)}: {message}
                    </a>
                  </li>
                ))}
              </ul>
              {formError && <p>{formError}</p>}
            </>
          ) : (
            <p className="of-error-summary-title">{formError}</p>
          )}
        </div>
      )}

      {fields.filter((f) => visible[f.key]).map(renderField)}

      <div className="of-actions">
        <button type="submit" className="of-submit" disabled={submitting}>
          {submitting ? "Sending…" : form.settings?.submitLabel || "Submit"}
        </button>
      </div>
    </form>
  );
}
```

- [ ] **Step 5: Write the stylesheet**

`web/packages/react/src/styles.css`:
```css
/* @openforms/react — default styles. Everything is scoped under .of-form / .of-status.
   Theme by overriding the custom properties on .of-form, .of-status or any ancestor. */
.of-form,
.of-status {
  --of-font: system-ui, -apple-system, "Segoe UI", Roboto, sans-serif;
  --of-text: #1f2328;
  --of-muted: #59636e;
  --of-bg: #ffffff;
  --of-border: #d1d9e0;
  --of-accent: #4f46e5;
  --of-accent-text: #ffffff;
  --of-danger: #cf222e;
  --of-success: #1a7f37;
  --of-radius: 8px;
  --of-gap: 1.25rem;
  font-family: var(--of-font);
  color: var(--of-text);
  line-height: 1.5;
  max-width: 40rem;
}

.of-form *,
.of-status * {
  box-sizing: border-box;
}

.of-header {
  margin-bottom: var(--of-gap);
}
.of-title {
  font-size: 1.5rem;
  margin: 0 0 0.25rem;
}
.of-description {
  color: var(--of-muted);
  margin: 0;
}

.of-field {
  margin: 0 0 var(--of-gap);
  padding: 0;
  border: 0;
  min-width: 0;
}
.of-label {
  display: block;
  font-weight: 600;
  margin-bottom: 0.25rem;
}
.of-required {
  color: var(--of-danger);
}
.of-help {
  color: var(--of-muted);
  font-size: 0.875rem;
  margin: 0 0 0.375rem;
}

.of-input {
  display: block;
  width: 100%;
  font: inherit;
  color: inherit;
  background: var(--of-bg);
  border: 1px solid var(--of-border);
  border-radius: var(--of-radius);
  padding: 0.5rem 0.75rem;
}
.of-textarea {
  resize: vertical;
}
.of-input:focus-visible,
.of-checkbox:focus-visible,
.of-submit:focus-visible,
.of-error-summary:focus-visible {
  outline: 3px solid color-mix(in srgb, var(--of-accent) 40%, transparent);
  outline-offset: 2px;
}
.of-field-invalid .of-input {
  border-color: var(--of-danger);
}
.of-field-error {
  color: var(--of-danger);
  font-size: 0.875rem;
  margin: 0.375rem 0 0;
}

.of-choices {
  display: grid;
  gap: 0.375rem;
}
.of-choice,
.of-checkbox-row {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}
.of-checkbox-row .of-label {
  margin: 0;
  font-weight: 500;
}
.of-checkbox {
  width: 1.125rem;
  height: 1.125rem;
  accent-color: var(--of-accent);
}

.of-error-summary {
  border: 2px solid var(--of-danger);
  border-radius: var(--of-radius);
  padding: 0.75rem 1rem;
  margin-bottom: var(--of-gap);
}
.of-error-summary-title {
  font-weight: 600;
  margin: 0 0 0.25rem;
}
.of-error-summary ul {
  margin: 0;
  padding-left: 1.25rem;
}
.of-error-summary a {
  color: var(--of-danger);
}

.of-actions {
  margin-top: var(--of-gap);
}
.of-submit {
  font: inherit;
  font-weight: 600;
  background: var(--of-accent);
  color: var(--of-accent-text);
  border: 0;
  border-radius: var(--of-radius);
  padding: 0.625rem 1.25rem;
  cursor: pointer;
}
.of-submit:disabled {
  opacity: 0.6;
  cursor: progress;
}

.of-confirmation {
  border-left: 4px solid var(--of-success);
  padding: 0.75rem 1rem;
}
.of-load-error,
.of-loading {
  color: var(--of-muted);
}

/* ---- StatusTracker ---- */
.of-status-title {
  font-size: 1.25rem;
  margin: 0 0 0.25rem;
}
.of-status-current {
  margin: 0 0 1rem;
}
.of-status-chain {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
  list-style: none;
  padding: 0;
  margin: 0 0 1.25rem;
}
.of-step {
  --of-step: var(--of-border);
  border: 2px solid var(--of-step);
  border-radius: 999px;
  padding: 0.25rem 0.75rem;
  color: var(--of-muted);
  font-size: 0.875rem;
}
.of-step-done {
  color: var(--of-text);
}
.of-step-current {
  background: var(--of-step);
  color: #ffffff;
  font-weight: 600;
}
.of-color-gray { --of-step: #6e7781; }
.of-color-blue { --of-step: #0969da; }
.of-color-green { --of-step: #1a7f37; }
.of-color-yellow { --of-step: #9a6700; }
.of-color-red { --of-step: #cf222e; }
.of-color-purple { --of-step: #8250df; }
.of-step-current:not([class*="of-color-"]) {
  --of-step: var(--of-accent);
}
.of-status-history {
  list-style: none;
  padding: 0;
  margin: 0;
  border-left: 2px solid var(--of-border);
}
.of-status-history li {
  padding: 0 0 0.75rem 1rem;
}
.of-status-history time {
  display: block;
  color: var(--of-muted);
  font-size: 0.875rem;
}
.of-status-note {
  color: var(--of-muted);
  font-size: 0.875rem;
}
```

Append to `web/packages/react/src/index.ts`:
```ts
export * from "./OpenForm.js";
export * from "./fields.js";
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `pnpm -C web/packages/react exec vitest run src/OpenForm.test.tsx && pnpm -C web/packages/react typecheck`
Expected: PASS, with 10 tests in `src/OpenForm.test.tsx`. Typecheck exits 0.

- [ ] **Step 7: Commit**

```bash
git add web/packages/react/src/fields.tsx web/packages/react/src/OpenForm.tsx web/packages/react/src/OpenForm.test.tsx web/packages/react/src/styles.css web/packages/react/src/index.ts
git commit -m "feat(react): accessible OpenForm renderer with default field components and styles"
```

---

### Task 7: `<StatusTracker>`

**Parallel group:** B (with Task 6). This task appends one export line to `src/index.ts` and touches no CSS: its styles already exist in Task 6's `styles.css`.

**Files:**
- Create: `web/packages/react/src/StatusTracker.tsx`
- Modify: `web/packages/react/src/index.ts` (append `export * from "./StatusTracker.js";`)
- Test: `web/packages/react/src/StatusTracker.test.tsx`

**Interfaces:**
- Consumes: `OpenFormsClient.getPublicStatus`, `OpenFormsError`, `PublicStatus` and `State` from the SDK; `API` and `newClient` from `test-fixtures.ts`.
- Produces:
```ts
interface StatusTrackerProps { client: OpenFormsClient; submissionId: string; token: string; pollMs?: number /* default 5000 */; className?: string }
function StatusTracker(props: StatusTrackerProps): JSX.Element;
```
  The DOM contract:
  - The root has class `.of-status`.
  - The current step is an `li` with `aria-current="step"` and class `of-step-current`.
  - The line "Current status: **<label>**" is `aria-live="polite"`.
  - A 404 renders `role="alert"` with the text "We couldn't find this submission."

- [ ] **Step 1: Write the failing test**

`web/packages/react/src/StatusTracker.test.tsx`:
```tsx
import { render, screen, waitFor } from "@testing-library/react";
import type { PublicStatus, State } from "@openforms/sdk";
import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import { afterAll, afterEach, beforeAll, describe, expect, it } from "vitest";
import { StatusTracker } from "./StatusTracker.js";
import { API, newClient } from "./test-fixtures.js";

const states: State[] = [
  { key: "new", label: "New", color: "gray" },
  { key: "screening", label: "Screening", color: "blue" },
  { key: "hired", label: "Hired", color: "green", terminal: true },
  { key: "rejected", label: "Rejected", color: "red", terminal: true },
];

function snapshot(state: string, history: string[], terminal = false): PublicStatus {
  const label = (key: string) => states.find((s) => s.key === key)!.label;
  return {
    id: "sub-1",
    formTitle: "Job application",
    state,
    stateLabel: label(state),
    terminal,
    states,
    history: history.map((key, i) => ({ state: key, label: label(key), at: `2026-09-2${3 + i}T10:00:00Z` })),
    createdAt: "2026-09-23T10:00:00Z",
  };
}

let calls = 0;
let sequence: PublicStatus[] = [];
let tokens: string[] = [];
const server = setupServer(
  http.get(`${API}/api/v1/public/submissions/:id`, ({ request }) => {
    tokens.push(new URL(request.url).searchParams.get("token") ?? "");
    const next = sequence[Math.min(calls, sequence.length - 1)];
    calls += 1;
    return next
      ? HttpResponse.json(next)
      : HttpResponse.json({ error: { code: "not_found", message: "not found" } }, { status: 404 });
  }),
);
beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => {
  server.resetHandlers();
  calls = 0;
  sequence = [];
  tokens = [];
});
afterAll(() => server.close());

const client = newClient();
const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

describe("<StatusTracker>", () => {
  it("renders the chain with the current and completed steps", async () => {
    sequence = [snapshot("screening", ["new", "screening"])];
    render(<StatusTracker client={client} submissionId="sub-1" token="tok-123" pollMs={10_000} />);
    expect(await screen.findByRole("heading", { name: "Job application" })).toBeInTheDocument();
    expect(screen.getByText("Screening", { selector: "strong" })).toBeInTheDocument();

    const current = screen.getByText("Screening", { selector: "li.of-step" });
    expect(current).toHaveAttribute("aria-current", "step");
    expect(current).toHaveClass("of-step-current", "of-color-blue");
    expect(screen.getByText("New", { selector: "li.of-step" })).toHaveClass("of-step-done");
    // Terminal states are only shown once reached.
    expect(screen.queryByText("Hired", { selector: "li.of-step" })).not.toBeInTheDocument();
    expect(tokens[0]).toBe("tok-123");
  });

  it("polls until the submission reaches a terminal state, then stops", async () => {
    sequence = [snapshot("new", ["new"]), snapshot("screening", ["new", "screening"]), snapshot("hired", ["new", "screening", "hired"], true)];
    render(<StatusTracker client={client} submissionId="sub-1" token="tok-123" pollMs={20} />);
    expect(await screen.findByText("Hired", { selector: "strong" })).toBeInTheDocument();
    expect(screen.getByText("Hired", { selector: "li.of-step" })).toHaveAttribute("aria-current", "step");
    const seen = calls;
    await sleep(120);
    expect(calls).toBe(seen);
    expect(seen).toBe(3);
  });

  it("stops polling when unmounted", async () => {
    sequence = [snapshot("new", ["new"])];
    const { unmount } = render(<StatusTracker client={client} submissionId="sub-1" token="tok-123" pollMs={20} />);
    await screen.findByText("New", { selector: "strong" });
    unmount();
    const seen = calls;
    await sleep(100);
    expect(calls).toBe(seen);
  });

  it("explains an unknown submission or bad token and does not retry", async () => {
    sequence = [];
    render(<StatusTracker client={client} submissionId="sub-x" token="wrong" pollMs={20} />);
    expect(await screen.findByRole("alert")).toHaveTextContent("We couldn't find this submission.");
    await sleep(100);
    expect(calls).toBe(1);
  });

  it("keeps the last status and retries after a transient failure", async () => {
    sequence = [snapshot("new", ["new"])];
    let failNext = false;
    server.use(
      http.get(`${API}/api/v1/public/submissions/:id`, () => {
        calls += 1;
        if (calls === 2) failNext = true;
        if (failNext && calls === 2) return new HttpResponse("upstream down", { status: 502 });
        return HttpResponse.json(calls >= 3 ? snapshot("hired", ["new", "hired"], true) : snapshot("new", ["new"]));
      }),
    );
    render(<StatusTracker client={client} submissionId="sub-1" token="tok-123" pollMs={20} />);
    await screen.findByText("New", { selector: "strong" });
    await waitFor(() => expect(screen.getByText("Hired", { selector: "strong" })).toBeInTheDocument());
    expect(calls).toBe(3);
  });

  it("lists the history with timestamps", async () => {
    sequence = [snapshot("screening", ["new", "screening"])];
    render(<StatusTracker client={client} submissionId="sub-1" token="tok-123" pollMs={10_000} />);
    const history = await screen.findByRole("list", { name: "History" });
    const times = history.querySelectorAll("time");
    expect(times).toHaveLength(2);
    expect(times[0]).toHaveAttribute("dateTime", "2026-09-23T10:00:00Z");
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `pnpm -C web/packages/react exec vitest run src/StatusTracker.test.tsx`
Expected: FAIL with `Failed to resolve import "./StatusTracker.js"`.

- [ ] **Step 3: Write the implementation**

`web/packages/react/src/StatusTracker.tsx`:
```tsx
import { OpenFormsError, type OpenFormsClient, type PublicStatus, type State } from "@openforms/sdk";
import { useEffect, useState } from "react";

export interface StatusTrackerProps {
  client: OpenFormsClient;
  submissionId: string;
  token: string;
  /** Poll interval in ms while the submission is not terminal. Default 5000. */
  pollMs?: number;
  className?: string;
}

const cx = (...names: Array<string | false | null | undefined>) => names.filter(Boolean).join(" ");

function isNotFound(err: unknown) {
  return err instanceof OpenFormsError && err.status === 404;
}

/** Non-terminal states in order, plus the current state when it is terminal. */
function visibleSteps(status: PublicStatus): State[] {
  const steps = status.states.filter((s) => !s.terminal || s.key === status.state);
  if (!steps.some((s) => s.key === status.state)) {
    steps.push({ key: status.state, label: status.stateLabel, terminal: status.terminal });
  }
  return steps;
}

export function StatusTracker({ client, submissionId, token, pollMs = 5000, className }: StatusTrackerProps) {
  const [status, setStatus] = useState<PublicStatus | null>(null);
  const [error, setError] = useState<unknown>(null);

  useEffect(() => {
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout> | undefined;

    const tick = async () => {
      try {
        const next = await client.getPublicStatus(submissionId, token);
        if (cancelled) return;
        setStatus(next);
        setError(null);
        if (!next.terminal) timer = setTimeout(tick, pollMs);
      } catch (err) {
        if (cancelled) return;
        setError(err);
        if (!isNotFound(err)) timer = setTimeout(tick, pollMs);
      }
    };

    setStatus(null);
    setError(null);
    void tick();
    return () => {
      cancelled = true;
      if (timer !== undefined) clearTimeout(timer);
    };
  }, [client, submissionId, token, pollMs]);

  if (isNotFound(error)) {
    return (
      <div className={cx("of-status", className)} role="alert">
        <p>
          <strong>We couldn't find this submission.</strong> Check that you opened the full link from your
          confirmation.
        </p>
      </div>
    );
  }

  if (!status) {
    return (
      <div className={cx("of-status", className)} aria-busy="true">
        {error ? "We couldn't load the status yet. Retrying…" : "Loading status…"}
      </div>
    );
  }

  const visited = new Set(status.history.map((h) => h.state));

  return (
    <div className={cx("of-status", className)}>
      <h2 className="of-status-title">{status.formTitle}</h2>
      <p className="of-status-current" aria-live="polite">
        Current status: <strong>{status.stateLabel}</strong>
      </p>

      <ol className="of-status-chain" aria-label="Progress">
        {visibleSteps(status).map((step) => {
          const current = step.key === status.state;
          return (
            <li
              key={step.key}
              className={cx(
                "of-step",
                step.color && `of-color-${step.color}`,
                current && "of-step-current",
                !current && visited.has(step.key) && "of-step-done",
              )}
              aria-current={current ? "step" : undefined}
            >
              {step.label}
            </li>
          );
        })}
      </ol>

      <ol className="of-status-history" aria-label="History">
        {status.history.map((item, i) => (
          <li key={`${item.state}-${i}`}>
            <span>{item.label}</span>
            <time dateTime={item.at}>{new Date(item.at).toLocaleString()}</time>
          </li>
        ))}
      </ol>

      {error ? <p className="of-status-note">Couldn't refresh the status. Retrying…</p> : null}
    </div>
  );
}
```

Append to `web/packages/react/src/index.ts`:
```ts
export * from "./StatusTracker.js";
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `pnpm -C web/packages/react test && pnpm -C web/packages/react typecheck`
Expected: PASS across all three test files (`useOpenForm`, `OpenForm`, `StatusTracker`). Typecheck exits 0.

- [ ] **Step 5: Commit**

```bash
git add web/packages/react/src/StatusTracker.tsx web/packages/react/src/StatusTracker.test.tsx web/packages/react/src/index.ts
git commit -m "feat(react): StatusTracker with polling until a terminal state"
```

---

### Task 8: `embed.js`

**Parallel group:** A (with Tasks 3 and 4). This task depends only on Task 1.

**Files:**
- Create: `web/packages/embed/package.json`
- Create: `web/packages/embed/tsconfig.json`
- Create: `web/packages/embed/vite.config.ts`
- Create: `web/packages/embed/vitest.config.ts`
- Create: `web/packages/embed/src/embed.ts`
- Create: `web/packages/embed/src/index.ts`
- Test: `web/packages/embed/src/embed.test.ts`

**Interfaces:**
- Consumes: the hosted page contract (Task 9). The hosted page posts `{type:"openforms:resize", height:number}` and `{type:"openforms:submitted", id, state}` to its parent.
- Produces: the build artefact `web/packages/embed/dist/embed.js` (IIFE), which `copy-dist.mjs` copies to `internal/webui/dist/embed/embed.js`. At runtime it installs a global `window.OpenForms = { scan(): void }` that SPA hosts can call after inserting new `[data-openforms]` elements. Module exports for tests:
```ts
const RESIZE = "openforms:resize", SUBMITTED = "openforms:submitted", MOUNTED_ATTR = "data-openforms-mounted";
function originFromScript(src: string | null | undefined, fallback: string): string;
function findScriptSrc(doc: Document): string | null;
function mount(container: HTMLElement, origin: string): HTMLIFrameElement | null;
function handleMessage(event: MessageEvent, origin: string, doc: Document): void;
function init(doc?: Document, win?: Window, scriptSrc?: string | null): { origin: string; scan(): void; destroy(): void };
```

- [ ] **Step 1: Create the package config**

`web/packages/embed/package.json`:
```json
{
  "name": "@openforms/embed",
  "version": "0.1.0",
  "private": true,
  "license": "MIT",
  "type": "module",
  "scripts": {
    "build": "vite build",
    "test": "vitest run",
    "typecheck": "tsc -p tsconfig.json"
  },
  "devDependencies": {
    "jsdom": "^26.0.0",
    "typescript": "^5.7.3",
    "vite": "^6.1.0",
    "vitest": "^3.0.5"
  }
}
```

`web/packages/embed/tsconfig.json`:
```json
{
  "extends": "../../tsconfig.base.json",
  "include": ["src", "vite.config.ts", "vitest.config.ts"]
}
```

`web/packages/embed/vite.config.ts`:
```ts
import { defineConfig } from "vite";

export default defineConfig({
  build: {
    lib: { entry: "src/index.ts", name: "OpenFormsEmbed", formats: ["iife"], fileName: () => "embed.js" },
    outDir: "dist",
    emptyOutDir: true,
    target: "es2019",
    minify: true,
  },
});
```

`web/packages/embed/vitest.config.ts`:
```ts
import { defineConfig } from "vitest/config";

export default defineConfig({ test: { environment: "jsdom", include: ["src/**/*.test.ts"] } });
```

Run `pnpm -C web install`.

- [ ] **Step 2: Write the failing test**

`web/packages/embed/src/embed.test.ts`:
```ts
import { afterEach, describe, expect, it, vi } from "vitest";
import { handleMessage, init, MOUNTED_ATTR, mount, originFromScript, RESIZE, SUBMITTED } from "./embed.js";

const ORIGIN = "https://forms.example.com";

afterEach(() => {
  document.body.innerHTML = "";
});

function container(slug = "job-application") {
  const div = document.createElement("div");
  div.setAttribute("data-openforms", slug);
  document.body.appendChild(div);
  return div;
}

function message(data: unknown, origin: string, source: Window | null) {
  return new MessageEvent("message", { data, origin, source });
}

describe("originFromScript", () => {
  it("derives the origin from the script src", () => {
    expect(originFromScript("https://forms.example.com/embed.js?v=1", "https://site.test")).toBe(ORIGIN);
  });
  it("resolves relative src against the fallback and tolerates garbage", () => {
    expect(originFromScript("/embed.js", "https://site.test")).toBe("https://site.test");
    expect(originFromScript(null, "https://site.test")).toBe("https://site.test");
    expect(originFromScript("http://[bad", "https://site.test")).toBe("https://site.test");
  });
});

describe("mount", () => {
  it("injects a full-width iframe pointing at the hosted embed page", () => {
    const div = container("job application");
    const iframe = mount(div, ORIGIN)!;
    expect(iframe.src).toBe(`${ORIGIN}/f/job%20application?embed=1`);
    expect(iframe.title).toBe("Form: job application");
    expect(iframe.style.width).toBe("100%");
    expect(div.hasAttribute(MOUNTED_ATTR)).toBe(true);
  });

  it("is idempotent and ignores empty slugs", () => {
    const div = container();
    expect(mount(div, ORIGIN)).not.toBeNull();
    expect(mount(div, ORIGIN)).toBeNull();
    expect(div.querySelectorAll("iframe")).toHaveLength(1);
    expect(mount(container("  "), ORIGIN)).toBeNull();
  });

  it("uses data-openforms-title when given", () => {
    const div = container();
    div.setAttribute("data-openforms-title", "Apply now");
    expect(mount(div, ORIGIN)!.title).toBe("Apply now");
  });
});

describe("handleMessage", () => {
  it("resizes the matching iframe for messages from the openforms origin", () => {
    const iframe = mount(container(), ORIGIN)!;
    handleMessage(message({ type: RESIZE, height: 812.4 }, ORIGIN, iframe.contentWindow), ORIGIN, document);
    expect(iframe.style.height).toBe("813px");
  });

  it("ignores messages from other origins", () => {
    const iframe = mount(container(), ORIGIN)!;
    const before = iframe.style.height;
    handleMessage(message({ type: RESIZE, height: 999 }, "https://evil.example", iframe.contentWindow), ORIGIN, document);
    expect(iframe.style.height).toBe(before);
  });

  it("ignores messages whose source is not one of our iframes", () => {
    const iframe = mount(container(), ORIGIN)!;
    const before = iframe.style.height;
    handleMessage(message({ type: RESIZE, height: 999 }, ORIGIN, window), ORIGIN, document);
    expect(iframe.style.height).toBe(before);
  });

  it("ignores malformed or absurd heights and non-object payloads", () => {
    const iframe = mount(container(), ORIGIN)!;
    const before = iframe.style.height;
    for (const height of ["900", -5, 0, Number.NaN, 1e9]) {
      handleMessage(message({ type: RESIZE, height }, ORIGIN, iframe.contentWindow), ORIGIN, document);
    }
    handleMessage(message("openforms:resize", ORIGIN, iframe.contentWindow), ORIGIN, document);
    handleMessage(message(null, ORIGIN, iframe.contentWindow), ORIGIN, document);
    expect(iframe.style.height).toBe(before);
  });

  it("re-dispatches submitted as a bubbling CustomEvent on the container", () => {
    const div = container();
    const iframe = mount(div, ORIGIN)!;
    const listener = vi.fn();
    document.body.addEventListener(SUBMITTED, listener);
    handleMessage(message({ type: SUBMITTED, id: "sub-1", state: "new" }, ORIGIN, iframe.contentWindow), ORIGIN, document);
    expect(listener).toHaveBeenCalledTimes(1);
    const event = listener.mock.calls[0]![0] as CustomEvent;
    expect(event.target).toBe(div);
    expect(event.detail).toEqual({ id: "sub-1", state: "new" });
  });
});

describe("init", () => {
  it("mounts every container and wires the message listener", () => {
    const a = container("contact");
    const b = container("job-application");
    const api = init(document, window, `${ORIGIN}/embed.js`);
    expect(api.origin).toBe(ORIGIN);
    const frameA = a.querySelector("iframe")!;
    expect(b.querySelector("iframe")).not.toBeNull();

    window.dispatchEvent(message({ type: RESIZE, height: 640 }, ORIGIN, frameA.contentWindow));
    expect(frameA.style.height).toBe("640px");

    const c = container("late");
    api.scan();
    expect(c.querySelector("iframe")).not.toBeNull();

    api.destroy();
    window.dispatchEvent(message({ type: RESIZE, height: 700 }, ORIGIN, frameA.contentWindow));
    expect(frameA.style.height).toBe("640px");
  });
});
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `pnpm -C web/packages/embed test`
Expected: FAIL with `Failed to resolve import "./embed.js"`.

- [ ] **Step 4: Write the implementation**

`web/packages/embed/src/embed.ts`:
```ts
// embed.js — turns <div data-openforms="slug"> into an auto-resizing iframe of the hosted form.
export const RESIZE = "openforms:resize";
export const SUBMITTED = "openforms:submitted";
export const MOUNTED_ATTR = "data-openforms-mounted";

const INITIAL_HEIGHT = 480;
const MAX_HEIGHT = 20000;

export function originFromScript(src: string | null | undefined, fallback: string): string {
  if (!src) return fallback;
  try {
    return new URL(src, fallback).origin;
  } catch {
    return fallback;
  }
}

export function findScriptSrc(doc: Document): string | null {
  const current = doc.currentScript as HTMLScriptElement | null;
  if (current?.src) return current.src;
  const tag = doc.querySelector<HTMLScriptElement>('script[src*="/embed.js"]');
  return tag?.src || null;
}

export function mount(container: HTMLElement, origin: string): HTMLIFrameElement | null {
  const slug = container.getAttribute("data-openforms")?.trim();
  if (!slug || container.hasAttribute(MOUNTED_ATTR)) return null;

  const iframe = container.ownerDocument.createElement("iframe");
  iframe.src = `${origin}/f/${encodeURIComponent(slug)}?embed=1`;
  iframe.title = container.getAttribute("data-openforms-title") || `Form: ${slug}`;
  iframe.setAttribute("loading", "lazy");
  iframe.style.width = "100%";
  iframe.style.border = "0";
  iframe.style.display = "block";
  iframe.style.height = `${INITIAL_HEIGHT}px`;

  container.setAttribute(MOUNTED_ATTR, "");
  container.appendChild(iframe);
  return iframe;
}

interface EmbedMessage {
  type?: unknown;
  height?: unknown;
  id?: unknown;
  state?: unknown;
}

export function handleMessage(event: MessageEvent, origin: string, doc: Document): void {
  if (event.origin !== origin) return;
  const data = event.data as EmbedMessage | null;
  if (data === null || typeof data !== "object") return;

  const iframe = Array.from(doc.querySelectorAll<HTMLIFrameElement>(`[${MOUNTED_ATTR}] > iframe`)).find(
    (frame) => frame.contentWindow !== null && frame.contentWindow === event.source,
  );
  if (!iframe) return;

  if (data.type === RESIZE) {
    const h = data.height;
    if (typeof h === "number" && Number.isFinite(h) && h > 0 && h <= MAX_HEIGHT) {
      iframe.style.height = `${Math.ceil(h)}px`;
    }
  } else if (data.type === SUBMITTED) {
    iframe.parentElement?.dispatchEvent(
      new CustomEvent(SUBMITTED, { bubbles: true, detail: { id: data.id, state: data.state } }),
    );
  }
}

export function init(
  doc: Document = document,
  win: Window = window,
  scriptSrc: string | null = findScriptSrc(doc),
): { origin: string; scan(): void; destroy(): void } {
  const origin = originFromScript(scriptSrc, win.location.origin);
  const scan = () => {
    doc.querySelectorAll<HTMLElement>("[data-openforms]").forEach((el) => {
      mount(el, origin);
    });
  };
  const listener = (event: MessageEvent) => handleMessage(event, origin, doc);
  win.addEventListener("message", listener);
  scan();
  return { origin, scan, destroy: () => win.removeEventListener("message", listener) };
}
```

`web/packages/embed/src/index.ts`:
```ts
import { findScriptSrc, init } from "./embed.js";

declare global {
  interface Window {
    OpenForms?: { scan(): void };
  }
}

// document.currentScript is only available while this script is executing, so capture it now.
const scriptSrc = findScriptSrc(document);

function start() {
  if (window.OpenForms) {
    window.OpenForms.scan();
    return;
  }
  const api = init(document, window, scriptSrc);
  window.OpenForms = { scan: api.scan };
}

if (document.readyState === "loading") {
  document.addEventListener("DOMContentLoaded", start, { once: true });
} else {
  start();
}
```

- [ ] **Step 5: Run the tests and the build to verify they pass**

Run: `pnpm -C web/packages/embed test && pnpm -C web/packages/embed typecheck && pnpm -C web/packages/embed build && ls web/packages/embed/dist`
Expected: 12 tests pass, typecheck exits 0, and the build prints `dist/embed.js` (a few KB). `ls` shows `embed.js`.

- [ ] **Step 6: Commit**

```bash
git add web/packages/embed web/pnpm-lock.yaml
git commit -m "feat(embed): embed.js iframe loader with origin-checked resize and submitted events"
```

---

### Task 9: Hosted app (`/f/:slug`, `/s/:id`, embed mode)

**Files:**
- Create: `web/apps/hosted/package.json`
- Create: `web/apps/hosted/tsconfig.json`
- Create: `web/apps/hosted/vite.config.ts`
- Create: `web/apps/hosted/vitest.config.ts`
- Create: `web/apps/hosted/index.html`
- Create: `web/apps/hosted/src/route.ts`
- Create: `web/apps/hosted/src/embed-bridge.ts`
- Create: `web/apps/hosted/src/App.tsx`
- Create: `web/apps/hosted/src/main.tsx`
- Create: `web/apps/hosted/src/hosted.css`
- Create: `web/apps/hosted/src/test-setup.ts`
- Test: `web/apps/hosted/src/route.test.ts`
- Test: `web/apps/hosted/src/App.test.tsx`

**Interfaces:**
- Consumes: `OpenForm`, `StatusTracker` and `@openforms/react/styles.css` from `@openforms/react`; `OpenFormsClient` and `PublicSubmitResult` from `@openforms/sdk`. The Go `webui` handler (Plan 01) serves `dist/hosted/index.html` for `/f/*` and `/s/*`, and serves assets from `/_app/hosted/`.
- Produces:
```ts
type Route = { kind: "form"; slug: string; embed: boolean } | { kind: "status"; id: string; token: string; embed: boolean } | { kind: "notFound"; embed: boolean };
function parseRoute(pathname: string, search: string): Route;
function statusHref(id: string, token: string): string;   // "/s/<id>?token=<token>"
interface ParentLike { postMessage(message: unknown, targetOrigin: string): void }
function postToParent(parent: ParentLike | null, message: { type: string; [k: string]: unknown }): void;
function startResizeReporting(doc: Document, parent: ParentLike, RO?: typeof ResizeObserver): () => void;
function App(props: { route: Route; client: OpenFormsClient; parent: ParentLike | null }): JSX.Element;
```
  Build output: `web/apps/hosted/dist/index.html` with assets under `/_app/hosted/assets/`.

- [ ] **Step 1: Create the app config**

`web/apps/hosted/package.json`:
```json
{
  "name": "@openforms/hosted",
  "version": "0.1.0",
  "private": true,
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "tsc -p tsconfig.json && vite build",
    "test": "vitest run",
    "typecheck": "tsc -p tsconfig.json"
  },
  "dependencies": {
    "@openforms/react": "workspace:*",
    "@openforms/sdk": "workspace:*",
    "react": "^18.3.1",
    "react-dom": "^18.3.1"
  },
  "devDependencies": {
    "@testing-library/dom": "^10.4.0",
    "@testing-library/jest-dom": "^6.6.3",
    "@testing-library/react": "^16.2.0",
    "@testing-library/user-event": "^14.6.1",
    "@types/node": "^22.10.0",
    "@types/react": "^18.3.18",
    "@types/react-dom": "^18.3.5",
    "@vitejs/plugin-react": "^4.3.4",
    "jsdom": "^26.0.0",
    "msw": "^2.7.0",
    "typescript": "^5.7.3",
    "vite": "^6.1.0",
    "vitest": "^3.0.5"
  }
}
```

`web/apps/hosted/tsconfig.json`:
```json
{
  "extends": "../../tsconfig.base.json",
  "compilerOptions": { "types": ["node", "vite/client"] },
  "include": ["src", "vite.config.ts", "vitest.config.ts"]
}
```

`web/apps/hosted/vite.config.ts`:
```ts
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

export default defineConfig({
  base: "/_app/hosted/",
  plugins: [react()],
  server: {
    port: 5174,
    proxy: { "/api": "http://localhost:8080", "/healthz": "http://localhost:8080" },
  },
  build: { outDir: "dist", emptyOutDir: true },
});
```

`web/apps/hosted/vitest.config.ts`:
```ts
import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

export default defineConfig({
  plugins: [react()],
  test: { environment: "jsdom", setupFiles: ["./src/test-setup.ts"], include: ["src/**/*.test.{ts,tsx}"], css: false },
});
```

`web/apps/hosted/index.html`:
```html
<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <meta name="robots" content="noindex" />
    <title>openforms</title>
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/src/main.tsx"></script>
  </body>
</html>
```

`web/apps/hosted/src/test-setup.ts`:
```ts
import "@testing-library/jest-dom/vitest";
import { cleanup } from "@testing-library/react";
import { afterEach } from "vitest";

afterEach(() => cleanup());
```

Run `pnpm -C web install`.

- [ ] **Step 2: Write the failing route test**

`web/apps/hosted/src/route.test.ts`:
```ts
import { describe, expect, it } from "vitest";
import { parseRoute, statusHref } from "./route.js";

describe("parseRoute", () => {
  it.each([
    ["/f/contact", "", { kind: "form", slug: "contact", embed: false }],
    ["/f/contact/", "?embed=1", { kind: "form", slug: "contact", embed: true }],
    ["/f/job%20application", "", { kind: "form", slug: "job application", embed: false }],
    ["/_app/hosted/f/contact", "", { kind: "form", slug: "contact", embed: false }],
    ["/s/sub-1", "?token=a%2Fb", { kind: "status", id: "sub-1", token: "a/b", embed: false }],
    ["/s/sub-1", "", { kind: "notFound", embed: false }],
    ["/f/", "", { kind: "notFound", embed: false }],
    ["/f/a/b", "", { kind: "notFound", embed: false }],
    ["/elsewhere", "?embed=1", { kind: "notFound", embed: true }],
    ["/f/%E0%A4%A", "", { kind: "form", slug: "%E0%A4%A", embed: false }],
  ])("%s%s", (pathname, search, expected) => {
    expect(parseRoute(pathname, search)).toEqual(expected);
  });
});

describe("statusHref", () => {
  it("encodes id and token", () => {
    expect(statusHref("sub 1", "a/b+c")).toBe("/s/sub%201?token=a%2Fb%2Bc");
  });
});
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `pnpm -C web/apps/hosted exec vitest run src/route.test.ts`
Expected: FAIL with `Failed to resolve import "./route.js"`.

- [ ] **Step 4: Write `route.ts` and `embed-bridge.ts`**

`web/apps/hosted/src/route.ts`:
```ts
export type Route =
  | { kind: "form"; slug: string; embed: boolean }
  | { kind: "status"; id: string; token: string; embed: boolean }
  | { kind: "notFound"; embed: boolean };

function decode(segment: string): string {
  try {
    return decodeURIComponent(segment);
  } catch {
    return segment;
  }
}

export function parseRoute(pathname: string, search: string): Route {
  const params = new URLSearchParams(search);
  const embed = params.get("embed") === "1";
  // In `vite dev` the app is served under its base path; in production the Go server serves it at /f and /s.
  const path = pathname.replace(/^\/_app\/hosted(?=\/)/, "");
  const segments = path.split("/").filter(Boolean).map(decode);

  if (segments.length === 2 && segments[0] === "f") {
    return { kind: "form", slug: segments[1]!, embed };
  }
  if (segments.length === 2 && segments[0] === "s") {
    const token = params.get("token");
    if (token) return { kind: "status", id: segments[1]!, token, embed };
  }
  return { kind: "notFound", embed };
}

export function statusHref(id: string, token: string): string {
  return `/s/${encodeURIComponent(id)}?token=${encodeURIComponent(token)}`;
}
```

`web/apps/hosted/src/embed-bridge.ts`:
```ts
export interface ParentLike {
  postMessage(message: unknown, targetOrigin: string): void;
}

export function postToParent(parent: ParentLike | null, message: { type: string; [key: string]: unknown }): void {
  // The embedding page's origin is unknown; messages carry no secrets (height, submission id, state).
  parent?.postMessage(message, "*");
}

export function startResizeReporting(
  doc: Document,
  parent: ParentLike,
  RO: typeof ResizeObserver | undefined = globalThis.ResizeObserver,
): () => void {
  let last = -1;
  const report = () => {
    const height = Math.ceil(doc.documentElement.scrollHeight);
    if (height === last) return;
    last = height;
    postToParent(parent, { type: "openforms:resize", height });
  };
  report();
  if (RO === undefined) {
    const id = setInterval(report, 500);
    return () => clearInterval(id);
  }
  const observer = new RO(report);
  observer.observe(doc.body);
  return () => observer.disconnect();
}
```

Run: `pnpm -C web/apps/hosted exec vitest run src/route.test.ts`
Expected: PASS (11 tests).

- [ ] **Step 5: Write the failing App test**

`web/apps/hosted/src/App.test.tsx`:
```tsx
import { OpenFormsClient, type FormDefinition } from "@openforms/sdk";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import { App } from "./App.js";
import { parseRoute } from "./route.js";

const API = "http://api.test";
const contact: FormDefinition = {
  slug: "contact",
  title: "Contact us",
  description: "We usually reply within a day.",
  settings: { public: true, confirmationMessage: "Thanks, we got your message." },
  fields: [{ key: "email", type: "email", label: "Email", required: true }],
};

const server = setupServer(
  http.get(`${API}/api/v1/public/forms/:slug`, ({ params }) =>
    params.slug === "contact"
      ? HttpResponse.json({ form: contact })
      : HttpResponse.json({ error: { code: "not_found", message: "form not found" } }, { status: 404 }),
  ),
  http.post(`${API}/api/v1/public/forms/contact/submissions`, () =>
    HttpResponse.json(
      { id: "sub-9", state: "new", stateLabel: "New", receiptToken: "r/t", confirmationMessage: "Thanks, we got your message." },
      { status: 201 },
    ),
  ),
  http.get(`${API}/api/v1/public/submissions/:id`, () =>
    HttpResponse.json({
      id: "sub-9",
      formTitle: "Contact us",
      state: "new",
      stateLabel: "New",
      terminal: false,
      states: [{ key: "new", label: "New" }],
      history: [{ state: "new", label: "New", at: "2026-09-23T10:00:00Z" }],
      createdAt: "2026-09-23T10:00:00Z",
    }),
  ),
);
beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => {
  server.resetHandlers();
  document.documentElement.classList.remove("of-embed");
});
afterAll(() => server.close());

const client = new OpenFormsClient({ baseUrl: API });

function renderAt(url: string, parent: { postMessage: ReturnType<typeof vi.fn> } | null = null) {
  const u = new URL(url, "http://hosted.test");
  return render(<App route={parseRoute(u.pathname, u.search)} client={client} parent={parent} />);
}

async function submitContact() {
  const user = userEvent.setup();
  await screen.findByRole("heading", { name: "Contact us" });
  await user.type(screen.getByLabelText(/^Email/), "ada@example.com");
  await user.click(screen.getByRole("button", { name: "Submit" }));
}

describe("hosted form page", () => {
  it("renders the form with page chrome and sets the document title", async () => {
    renderAt("/f/contact");
    expect(await screen.findByRole("heading", { name: "Contact us" })).toBeInTheDocument();
    expect(screen.getByText("We usually reply within a day.")).toBeInTheDocument();
    expect(screen.getByRole("contentinfo")).toHaveTextContent("Powered by openforms");
    await waitFor(() => expect(document.title).toBe("Contact us"));
  });

  it("shows the confirmation and a tracking link after submitting", async () => {
    renderAt("/f/contact");
    await submitContact();
    expect(await screen.findByText("Thanks, we got your message.")).toBeInTheDocument();
    const link = screen.getByRole("link", { name: "Track your submission" });
    expect(link).toHaveAttribute("href", "/s/sub-9?token=r%2Ft");
    expect(link).not.toHaveAttribute("target");
  });

  it("shows a friendly page for unknown or private forms", async () => {
    renderAt("/f/secret");
    expect(await screen.findByRole("heading", { name: "This form isn't available" })).toBeInTheDocument();
  });

  it("shows not found for unknown paths", () => {
    renderAt("/nope");
    expect(screen.getByRole("heading", { name: "Page not found" })).toBeInTheDocument();
  });
});

describe("embed mode", () => {
  it("drops chrome, reports its height and announces submissions to the parent", async () => {
    const parent = { postMessage: vi.fn() };
    renderAt("/f/contact?embed=1", parent);
    await screen.findByRole("heading", { name: "Contact us" });
    expect(screen.queryByRole("contentinfo")).not.toBeInTheDocument();
    expect(document.documentElement).toHaveClass("of-embed");
    expect(parent.postMessage).toHaveBeenCalledWith({ type: "openforms:resize", height: expect.any(Number) }, "*");

    await submitContact();
    await screen.findByText("Thanks, we got your message.");
    expect(parent.postMessage).toHaveBeenCalledWith({ type: "openforms:submitted", id: "sub-9", state: "new" }, "*");
    expect(screen.getByRole("link", { name: "Track your submission" })).toHaveAttribute("target", "_blank");
  });

  it("does not post messages when not framed", async () => {
    renderAt("/f/contact?embed=1", null);
    await screen.findByRole("heading", { name: "Contact us" });
    expect(document.documentElement).toHaveClass("of-embed");
  });
});

describe("status page", () => {
  it("renders the tracker for the submission", async () => {
    renderAt("/s/sub-9?token=r%2Ft");
    expect(await screen.findByText("New", { selector: "strong" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Contact us" })).toBeInTheDocument();
  });
});
```

Run: `pnpm -C web/apps/hosted exec vitest run src/App.test.tsx`
Expected: FAIL with `Failed to resolve import "./App.js"`.

- [ ] **Step 6: Write the App, entry point and styles**

`web/apps/hosted/src/App.tsx`:
```tsx
import { OpenForm, StatusTracker } from "@openforms/react";
import type { OpenFormsClient } from "@openforms/sdk";
import { useEffect, type ReactNode } from "react";
import { postToParent, startResizeReporting, type ParentLike } from "./embed-bridge.js";
import { statusHref, type Route } from "./route.js";

export interface AppProps {
  route: Route;
  client: OpenFormsClient;
  /** window.parent when framed, otherwise null. */
  parent: ParentLike | null;
}

function Message({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="hosted-message">
      <h1>{title}</h1>
      <p>{children}</p>
    </section>
  );
}

export function NotAvailable() {
  return (
    <Message title="This form isn't available">
      The link may be mistyped, or the form is no longer accepting responses.
    </Message>
  );
}

function FormPage({ client, slug, embed, parent }: { client: OpenFormsClient; slug: string; embed: boolean; parent: ParentLike | null }) {
  return (
    <OpenForm
      client={client}
      slug={slug}
      onLoaded={(form) => {
        document.title = form.title;
      }}
      renderLoadError={() => <NotAvailable />}
      onSubmitted={(result) => {
        if (embed && result) postToParent(parent, { type: "openforms:submitted", id: result.id, state: result.state });
      }}
      renderConfirmation={(result, form) => (
        <div className="of-confirmation" role="status">
          <p>{result?.confirmationMessage || form.settings?.confirmationMessage || "Thanks! Your response has been recorded."}</p>
          {result && (
            <p>
              <a
                href={statusHref(result.id, result.receiptToken)}
                target={embed ? "_blank" : undefined}
                rel={embed ? "noopener noreferrer" : undefined}
              >
                Track your submission
              </a>
            </p>
          )}
        </div>
      )}
    />
  );
}

function StatusPage({ client, id, token }: { client: OpenFormsClient; id: string; token: string }) {
  useEffect(() => {
    document.title = "Submission status";
  }, []);
  return <StatusTracker client={client} submissionId={id} token={token} />;
}

export function App({ route, client, parent }: AppProps) {
  useEffect(() => {
    document.documentElement.classList.toggle("of-embed", route.embed);
    if (!route.embed || !parent) return;
    return startResizeReporting(document, parent);
  }, [route.embed, parent]);

  let content: ReactNode;
  switch (route.kind) {
    case "form":
      content = <FormPage client={client} slug={route.slug} embed={route.embed} parent={parent} />;
      break;
    case "status":
      content = <StatusPage client={client} id={route.id} token={route.token} />;
      break;
    default:
      content = <Message title="Page not found">Check the link you were given and try again.</Message>;
  }

  if (route.embed) {
    return <main className="hosted hosted-embed">{content}</main>;
  }
  return (
    <div className="hosted">
      <main className="hosted-main">{content}</main>
      <footer className="hosted-footer">
        Powered by <a href="https://github.com/openforms/openforms">openforms</a>
      </footer>
    </div>
  );
}
```

`web/apps/hosted/src/main.tsx`:
```tsx
import "@openforms/react/styles.css";
import "./hosted.css";
import { OpenFormsClient } from "@openforms/sdk";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "./App.js";
import { parseRoute } from "./route.js";

const route = parseRoute(window.location.pathname, window.location.search);
const parent = window.parent !== window ? window.parent : null;

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App route={route} client={new OpenFormsClient()} parent={parent} />
  </StrictMode>,
);
```

`web/apps/hosted/src/hosted.css`:
```css
:root {
  color-scheme: light;
  background: #f6f8fa;
}
body {
  margin: 0;
  font-family: system-ui, -apple-system, "Segoe UI", Roboto, sans-serif;
  color: #1f2328;
}
.hosted {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 2rem 1rem;
  box-sizing: border-box;
}
.hosted-main {
  width: 100%;
  max-width: 42rem;
  background: #ffffff;
  border: 1px solid #d1d9e0;
  border-radius: 12px;
  padding: 2rem;
  box-sizing: border-box;
}
.hosted-main .of-form,
.hosted-main .of-status {
  max-width: none;
}
.hosted-footer {
  margin-top: 1.5rem;
  font-size: 0.875rem;
  color: #59636e;
}
.hosted-footer a {
  color: inherit;
}
.hosted-message h1 {
  font-size: 1.5rem;
  margin: 0 0 0.5rem;
}
.hosted-message p {
  color: #59636e;
  margin: 0;
}
/* Embedded in an iframe: no chrome, transparent so the host page shows through. */
html.of-embed,
html.of-embed body {
  background: transparent;
}
.hosted-embed {
  padding: 0.25rem;
}
.hosted-embed .of-form,
.hosted-embed .of-status {
  max-width: none;
}
@media (max-width: 480px) {
  .hosted-main {
    padding: 1.25rem;
  }
}
```

- [ ] **Step 7: Run the tests, typecheck and build to verify they pass**

Run: `pnpm -C web/apps/hosted test && pnpm -C web/apps/hosted typecheck && pnpm -C web/apps/hosted build && grep -o '/_app/hosted/assets/[^"]*\.js' web/apps/hosted/dist/index.html`
Expected:
- The route (11) and App (7) tests pass.
- Typecheck exits 0.
- Vite prints `dist/index.html` and `dist/assets/*`.
- `grep` prints one path starting with `/_app/hosted/assets/`.

- [ ] **Step 8: Commit**

```bash
git add web/apps/hosted web/pnpm-lock.yaml
git commit -m "feat(hosted): hosted form and status pages with iframe embed mode"
```

---

### Task 10: Workspace build integration and developer docs

**Files:**
- Modify: `.gitignore` (repo root; create if absent, append if present)
- Create: `web/README.md`

**Interfaces:**
- Consumes: everything above; `internal/webui/dist/.gitkeep` and the Go `webui` handler from Plan 01.
- Produces: a green `pnpm -C web build`, `test`, `typecheck` and `test:scripts`. The build populates `internal/webui/dist/hosted/` and `internal/webui/dist/embed/embed.js`, which Plans 07–09 extend with `admin` and `demo`.

- [ ] **Step 1: Ignore build output in git**

Append to the repo-root `.gitignore`:
```
# web build output (populated by `pnpm -C web build`)
web/**/node_modules/
internal/webui/dist/*
!internal/webui/dist/.gitkeep
```

- [ ] **Step 2: Write the web README**

`web/README.md`:
````markdown
# openforms web workspace

pnpm workspace for everything that runs in a browser.

| Path | Package | What it is |
|---|---|---|
| `packages/sdk` | `@openforms/sdk` | Typed API client, definition types generated from `schemas/`, shared submission logic |
| `packages/react` | `@openforms/react` | `useOpenForm`, `<OpenForm>`, `<StatusTracker>`, `styles.css` |
| `packages/embed` | `@openforms/embed` | `embed.js` — `<div data-openforms="slug">` → auto-resizing iframe |
| `apps/hosted` | `@openforms/hosted` | Hosted pages served by the Go binary at `/f/:slug` and `/s/:id` |

## Commands (from the repo root)

```bash
pnpm -C web install
pnpm -C web test          # all package tests (Vitest)
pnpm -C web test:scripts  # build-script tests (node:test)
pnpm -C web typecheck
pnpm -C web build         # builds packages, then apps, then copies into internal/webui/dist
pnpm -C web/packages/sdk gen   # regenerate definition types after editing schemas/*.schema.json
```

## Developing the hosted pages

Run the Go server on :8080 (`go run ./cmd/openforms serve`), then:

```bash
pnpm -C web/apps/hosted dev
# open http://localhost:5174/_app/hosted/f/<slug>
```

The Vite dev server proxies `/api` and `/healthz` to `http://localhost:8080`.

## Embedding a form

```html
<div data-openforms="contact"></div>
<script src="https://forms.example.com/embed.js" async></script>
<script>
  document.addEventListener("openforms:submitted", (e) => console.log("submitted", e.detail));
</script>
```

Call `window.OpenForms.scan()` after inserting new `data-openforms` elements in a single-page app.
````

- [ ] **Step 3: Run the full workspace verification**

Run:
```bash
pnpm -C web install --frozen-lockfile \
  && pnpm -C web typecheck \
  && pnpm -C web test \
  && pnpm -C web test:scripts \
  && pnpm -C web build \
  && ls internal/webui/dist internal/webui/dist/hosted internal/webui/dist/embed
```
Expected:
- Every step exits 0.
- `pnpm -C web test` shows the sdk (types, client, logic), react (useOpenForm, OpenForm, StatusTracker), embed and hosted suites all passing.
- The build ends with `copy-dist: hosted, embed → …/internal/webui/dist`.
- `ls` shows `embed hosted` (plus `.gitkeep` with `ls -a`), `index.html assets` under `hosted`, and `embed.js` under `embed`.

- [ ] **Step 4: Smoke-test against the Go server (only if Plan 01 is merged on this branch)**

Run:
```bash
go build -o /tmp/openforms ./cmd/openforms && (OPENFORMS_DATABASE_URL=postgres://openforms:openforms@localhost:54329/openforms?sslmode=disable /tmp/openforms serve & echo $! > /tmp/of.pid) && sleep 2 \
  && curl -s http://localhost:8080/f/contact | grep -o '/_app/hosted/assets/[^"]*' | head -1 \
  && curl -s -o /dev/null -w '%{http_code} %{content_type}\n' http://localhost:8080/embed.js; kill "$(cat /tmp/of.pid)"
```
Expected: an asset path under `/_app/hosted/assets/`, then `200 text/javascript; charset=utf-8` (or `application/javascript`). If Plan 01 isn't merged yet, skip this step and note that in the commit message.

- [ ] **Step 5: Commit**

```bash
git add .gitignore web/README.md
git commit -m "chore(web): ignore embedded build output and document the web workspace"
```

---

## Self-review checklist (run once, after all tasks)

- §9.1 names: every client method in the spec list is covered by the endpoint table test in Task 3 (the `covers every client method listed in the spec` test asserts it). `OpenFormsError` exposes `status/code/message/details`. `visible` and `validateSubmission` run against both fixture files.
- §9.2: `useOpenForm` returns `form, status, values, setValue, visible, errors, submit, result, loadError`, plus `reset`. `OpenForm` accepts `client, slug, definition, onSubmitted, components, className`, plus the documented extras. All nine default components are exported. `styles.css` is exported as `@openforms/react/styles.css`. `StatusTracker` accepts `client, submissionId, token, pollMs`.
- §9.3: `/f/:slug` shows the title, description and form, then the confirmation with a "Track your submission" link to `/s/:id?token=`. `?embed=1` posts resize and submitted messages and drops chrome. `/s/:id` renders the tracker. Unknown or private forms show "This form isn't available".
- §9.4: the iframe URL is `<origin>/f/<slug>?embed=1`, with the origin taken from the script `src`. Resize is honoured only from that origin and from our own iframe. `openforms:submitted` is re-dispatched as a `CustomEvent` on the container.
- §3: the `web/package.json` scripts match the contract verbatim. `copy-dist.mjs` keeps `.gitkeep` and copies apps and embed. Vite `base` is `/_app/hosted/`, and the dev proxy covers `/api` and `/healthz`.
