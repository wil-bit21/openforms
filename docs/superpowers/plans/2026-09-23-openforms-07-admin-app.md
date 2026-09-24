# openforms Plan 07 — Admin App Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Tasks marked with the same **Parallel group** touch disjoint files and may be dispatched to concurrent subagents (superpowers:dispatching-parallel-agents), each in its own worktree or with strictly disjoint file sets.

**Goal:** Build the openforms admin SPA at `/admin/*`: sign-in, authenticated shell, reviewer inbox, submission detail with workflow actions, forms/workflows overviews, and admin-only users, API keys and jobs pages.

**Architecture:** `web/apps/admin` is a Vite + React 18 SPA. It talks to the Go API only through the `@openforms/sdk` `OpenFormsClient` singleton (`src/api.ts`). Server state lives in TanStack Query (keys in `src/queryKeys.ts`) and URL state (filters) lives in the router. Routes are declared as data (`src/routes.tsx`, `AdminRoute[]`) and mapped into react-router inside an authenticated shell, so Plan 08 can add editor routes with one-line edits. Tests render real pages against the real SDK, with HTTP mocked by MSW using the wire shapes from spec §7.

**Tech Stack:** TypeScript 5, React 18, Vite 6, react-router-dom 6, @tanstack/react-query 5, Vitest 3 + jsdom, @testing-library/react 16 + user-event 14 + jest-dom 6, MSW 2.

**Spec:** `docs/superpowers/specs/2026-09-23-openforms-design.md` (binding: §3 web build contract, §7 API shapes, §9.1 SDK method names, §9.5 admin routes/behaviour and the "Admin extension points" paragraph). Roadmap: `docs/superpowers/plans/2026-09-23-openforms-00-roadmap.md` (this plan is Wave 3, Lane C).

## Global Constraints

- Node 22 LTS, pnpm 9, TypeScript 5, React 18, Vite 6; the package lives at `web/apps/admin`, name `@openforms/admin`.
- Vite `base: "/_app/admin/"`. Router basename `/admin` in production builds (`/_app/admin` under `vite dev`).
- `pnpm -C web build` must put the build in `internal/webui/dist/admin/` (via Plan 06's `web/scripts/copy-dist.mjs`, which copies every `apps/<name>/dist`).
- In dev, the Vite server proxies `/api` and `/healthz` to `http://localhost:8080`.
- All JSON over the wire is camelCase; timestamps are RFC 3339 UTC strings; errors use the envelope `{"error":{"code","message","details"?}}`.
- The Go server is authoritative for validation. The admin UI's own checks (such as required transition fields) exist only for UX.
- Shipped pages must not use placeholder copy such as "Lorem ipsum".
- Commit after every task with conventional prefixes (`feat:`, `fix:`, `test:`, `chore:`, `docs:`).
- Use only SDK method names from spec §9.1. Treat the wire shapes in spec §7 as fixed. If Plan 06's SDK method *signatures* differ from the "Consumed SDK contract" below, adapt the call sites and keep the wire shapes the tests assert.
- Extension points are binding for Plan 08, with exact names: `src/routes.tsx` (`AdminRoute`, `routes`), `src/nav.ts` (`NavItem`, `navItems`), `src/extensions/OverviewExtras.tsx` (`OverviewExtras`), `src/test/render.tsx` (`renderWithProviders`), `src/test/server.ts` (`server`), `src/test/fixtures.ts` (`make*` factories), `src/api.ts` (`client`), `src/queryKeys.ts` (`qk`).

## Consumed SDK contract (from Plan 06, `@openforms/sdk`)

The SDK unwraps response envelopes and returns the inner value:

```ts
new OpenFormsClient({ baseUrl?: string; apiKey?: string; fetch?: typeof fetch })
login(email: string, password: string): Promise<User>          // POST /auth/login
logout(): Promise<void>                                          // POST /auth/logout
me(): Promise<Principal>                                         // GET /auth/me → .principal
listForms(): Promise<FormSummary[]>                              // GET /forms → .items
getForm(slug: string): Promise<FormRecord>                       // GET /forms/{slug} → .form
formVersions(slug: string): Promise<VersionInfo[]>               // → .items
listWorkflows(): Promise<WorkflowSummary[]>
getWorkflow(slug: string): Promise<WorkflowRecord>               // GET /workflows/{slug} → .workflow
workflowVersions(slug: string): Promise<VersionInfo[]>
listSubmissions(f: { form?: string; state?: string; assignee?: string; cursor?: string; limit?: number }): Promise<Page<Submission>>
getSubmission(id: string): Promise<SubmissionDetail>             // {submission, form, workflow, events, transitions}
transition(id: string, input: { transition: string; fields?: Record<string, unknown>; comment?: string; expectedState?: string }): Promise<Submission>
updateFields(id: string, fields: Record<string, unknown>): Promise<Submission>   // PATCH body {"fields": ...}
comment(id: string, body: string): Promise<SubmissionEvent>                        // POST body {"body": ...}
assign(id: string, userId: string | null): Promise<Submission>                     // PUT body {"userId": ...}
csvUrl(slug: string): string                                     // `${baseUrl}/api/v1/forms/${slug}/submissions.csv`
listUsers(): Promise<User[]>; createUser(i: { email: string; name: string; password: string; roles: string[] }): Promise<User>
updateUser(id: string, i: { name?: string; password?: string; roles?: string[] }): Promise<User>; deleteUser(id: string): Promise<void>
listApiKeys(): Promise<ApiKey[]>; createApiKey(i: { name: string; roles: string[] }): Promise<{ apiKey: ApiKey; key: string }>; revokeApiKey(id: string): Promise<void>
listJobs(status?: string): Promise<Job[]>; retryJob(id: number): Promise<void>
class OpenFormsError extends Error { status: number; code: string; message: string; details: { path: string; message: string }[] }
```
Types exported from `@openforms/sdk`: `FormDefinition, WorkflowDefinition, Principal, User, ApiKey, Job, FormSummary, WorkflowSummary, FormRecord, WorkflowRecord, VersionInfo, Submission, SubmissionEvent, AvailableTransition, SubmissionDetail, Page`.

## Event payload assumptions (to align with Plan 04)

The UI reads these payload keys and degrades gracefully when one is missing:
- `transition`: `{comment?, fields?}`
- `fields_updated`: `{fields}`
- `comment`: `{body}`
- `assigned`: `{assigneeId, assigneeName}`, where `assigneeName` is empty or missing when the assignee is cleared
- `action_succeeded` / `action_failed`: `{action: {type}, error?}`

## Review Focus

1. **Session expiry mid-use:** any API call that returns 401 while the user is on a page should send them to `/login`, not leave a dead page. The test lives in Task 3.
2. **Deep link while signed out:** opening `/admin/submissions/<id>` without a session should go to login and return to that submission afterwards. A `next=//evil.example` parameter must never redirect off-site. Tests live in Task 2.
3. **Pinned or old submission data:** data keys that aren't in the pinned form definition (legacy fields) must still show under their raw key, not disappear. The test lives in Task 4.
4. **Non-admin reviewer:** `GET /users` is admin-only, so the assignee picker for a reviewer must offer "Assign to me" / "Unassign" and never call `/users`. The test lives in Task 5.
5. **Unknown state or workflow-less form in the inbox:** a submission whose form isn't in the catalog, or whose state has no color (e.g. `submitted`), must render a gray badge and the form slug without crashing. The test lives in Task 3.

---

## File structure

```
web/apps/admin/
  package.json, tsconfig.json, vite.config.ts, index.html, README.md
  src/
    main.tsx                 # mounts <App/>
    App.tsx                  # buildRoutes() + App (browser router, providers)
    routes.tsx               # AdminRoute[] — the extension point (Plan 08 appends)
    nav.ts                   # NavItem[] — sidebar items
    api.ts                   # singleton OpenFormsClient + resolvingFetch
    queryKeys.ts             # qk
    styles/tokens.css, styles/app.css
    lib/queryClient.ts       # createQueryClient (401 → invalidate session)
    lib/errors.ts            # errorMessage, errorCode, problemsByPath
    lib/format.ts            # relativeTime, formatValue, submissionSummary, shortId
    lib/roles.ts             # parseRoles, formatRoles
    lib/session.ts           # useSession, isAdmin
    lib/catalog.ts           # useCatalog (forms + workflow defs for inbox)
    lib/events.ts            # describeEvent, actorName
    components/StateBadge.tsx, Dialog.tsx, ErrorMessage.tsx, RelativeTime.tsx,
               CopyButton.tsx, Loading.tsx, RoleTags.tsx, VersionTable.tsx,
               RequireSession.tsx, RequireAdmin.tsx, Shell.tsx
    extensions/OverviewExtras.tsx
    pages/LoginPage.tsx, NotFoundPage.tsx, InboxPage.tsx, SubmissionPage.tsx,
          FormsPage.tsx, FormOverviewPage.tsx, WorkflowsPage.tsx, WorkflowOverviewPage.tsx,
          UsersPage.tsx, ApiKeysPage.tsx, JobsPage.tsx
    pages/submission/AnswersPanel.tsx, Timeline.tsx, TransitionBar.tsx, TransitionDialog.tsx,
                     WorkflowFieldInput.tsx, WorkflowFieldsPanel.tsx, AssigneePicker.tsx,
                     CommentBox.tsx, refresh.ts
    pages/users/UserDialogs.tsx
    pages/api-keys/KeyDialogs.tsx
    test/setup.ts, server.ts, handlers.ts, fixtures.ts, render.tsx
```

## Task order and parallelism

- Tasks 1 and 2 are sequential. They create the scaffold, shared components, the harness, the session and the shell, and they stub every page file.
- After Task 2, these lanes can run in parallel because each lane only replaces its own page files and adds its own tests:
  - **Parallel group A, Lane 1:** Task 3 (Inbox)
  - **Parallel group A, Lane 2:** Task 4, then Task 5 (submission detail)
  - **Parallel group A, Lane 3:** Task 6, then Task 7 (forms, then workflows)
  - **Parallel group A, Lane 4:** Task 8 (Users)
  - **Parallel group A, Lane 5:** Task 9 (API keys)
  - **Parallel group A, Lane 6:** Task 10 (Jobs)
- Task 11 (build integration and README) runs last.

Common commands, all run from the repo root:
- Install: `pnpm -C web install`
- One test file: `pnpm -C web/apps/admin exec vitest run <path relative to web/apps/admin>`
- All admin tests: `pnpm -C web/apps/admin test`
- Typecheck: `pnpm -C web/apps/admin typecheck`

---

### Task 1: Scaffold, design tokens, shared components, test harness

**Files:**
- Create: `web/apps/admin/package.json`, `web/apps/admin/tsconfig.json`, `web/apps/admin/vite.config.ts`, `web/apps/admin/index.html`
- Create: `web/apps/admin/src/api.ts`, `src/queryKeys.ts`, `src/lib/queryClient.ts`, `src/lib/errors.ts`, `src/lib/format.ts`, `src/lib/roles.ts`
- Create: `src/styles/tokens.css`, `src/styles/app.css`
- Create: `src/components/StateBadge.tsx`, `Dialog.tsx`, `ErrorMessage.tsx`, `RelativeTime.tsx`, `CopyButton.tsx`, `Loading.tsx`, `RoleTags.tsx`
- Create: `src/test/setup.ts`, `src/test/server.ts`, `src/test/handlers.ts`, `src/test/fixtures.ts`, `src/test/render.tsx`
- Test: `src/lib/format.test.ts`, `src/lib/roles.test.ts`, `src/components/StateBadge.test.tsx`, `src/components/Dialog.test.tsx`, `src/test/harness.test.tsx`

**Interfaces:**
- Consumes: `@openforms/sdk` (`OpenFormsClient`, `OpenFormsError`, types) from Plan 06. `web/pnpm-workspace.yaml` must include `apps/*` (Plan 06).
- Produces:
  - `src/api.ts`: `export function resolvingFetch(input: RequestInfo | URL, init?: RequestInit): Promise<Response>` and `export const client: OpenFormsClient` (baseUrl `""`).
  - `src/queryKeys.ts`: `export const qk = { me, forms, form(slug), formVersions(slug), workflows, workflow(slug), workflowVersions(slug), submissions(filter), submission(id), users, apiKeys, jobs(status) }`.
  - `src/lib/queryClient.ts`: `createQueryClient(opts?: { test?: boolean }): QueryClient`, `isUnauthenticated(err: unknown): boolean`.
  - `src/lib/errors.ts`: `errorMessage(err: unknown): string`, `errorCode(err: unknown): string | undefined`, `problemsByPath(err: unknown): Record<string, string>`.
  - `src/lib/format.ts`: `FieldLike`, `relativeTime(iso: string, now?: Date): string`, `formatValue(value: unknown, field?: FieldLike): string`, `submissionSummary(data: Record<string, unknown>, form?: { fields: FieldLike[] }): string`, `shortId(id: string): string`.
  - `src/lib/roles.ts`: `parseRoles(input: string): string[]`, `formatRoles(roles: string[]): string`.
  - Components: `StateBadge({label, color?})`, `badgeColor(c?)`, `Dialog({title, onClose, children})`, `ErrorMessage({error})`, `RelativeTime({iso})`, `CopyButton({text, label?})`, `Loading()`, `RoleTags({roles})`.
  - Harness: `server` (MSW); `api(path)`, `API`, `apiError(status, code, message, details?)`, and `defaultHandlers` in `handlers.ts`; fixture factories `makePrincipal, makeReviewerPrincipal, makeUser, makeApiKey, makeJob, makeSubmission, makeEvent, makeTransition, makeDetail, makeFormDefinition, makeWorkflowDefinition, makeFormSummary, makeWorkflowSummary, makeFormRecord, makeWorkflowRecord, makeVersionInfo`, plus `ids`; `renderWithProviders(ui, { route?, path? })` in `render.tsx` (Task 2 adds `renderApp`).

- [ ] **Step 1: Check that the workspace includes apps**

Run: `cat web/pnpm-workspace.yaml`
Expected: the `packages:` list contains `"apps/*"`. If it doesn't, add the line `  - "apps/*"` under `packages:`.

- [ ] **Step 2: Create the package files**

`web/apps/admin/package.json`:
```json
{
  "name": "@openforms/admin",
  "private": true,
  "version": "0.0.0",
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "tsc --noEmit -p tsconfig.json && vite build",
    "test": "vitest run",
    "typecheck": "tsc --noEmit -p tsconfig.json"
  },
  "dependencies": {
    "@openforms/sdk": "workspace:*",
    "@tanstack/react-query": "^5.62.0",
    "react": "^18.3.1",
    "react-dom": "^18.3.1",
    "react-router-dom": "^6.28.0"
  },
  "devDependencies": {
    "@testing-library/dom": "^10.4.0",
    "@testing-library/jest-dom": "^6.6.3",
    "@testing-library/react": "^16.1.0",
    "@testing-library/user-event": "^14.5.2",
    "@types/react": "^18.3.12",
    "@types/react-dom": "^18.3.1",
    "@vitejs/plugin-react": "^4.3.4",
    "jsdom": "^25.0.1",
    "msw": "^2.6.8",
    "typescript": "^5.7.2",
    "vite": "^6.0.3",
    "vitest": "^3.0.5"
  }
}
```

`web/apps/admin/tsconfig.json`. It resolves the SDK from source, so tests and typechecks don't depend on the SDK being built:
```json
{
  "extends": "../../tsconfig.base.json",
  "compilerOptions": {
    "target": "ES2022",
    "lib": ["ES2022", "DOM", "DOM.Iterable"],
    "module": "ESNext",
    "moduleResolution": "Bundler",
    "jsx": "react-jsx",
    "strict": true,
    "noEmit": true,
    "isolatedModules": true,
    "skipLibCheck": true,
    "resolveJsonModule": true,
    "types": ["vite/client"],
    "paths": { "@openforms/sdk": ["../../packages/sdk/src/index.ts"] }
  },
  "include": ["src", "vite.config.ts"]
}
```

`web/apps/admin/vite.config.ts`:
```ts
import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import { fileURLToPath } from "node:url";

const sdkSrc = fileURLToPath(new URL("../../packages/sdk/src/index.ts", import.meta.url));

export default defineConfig({
  base: "/_app/admin/",
  plugins: [react()],
  resolve: { alias: { "@openforms/sdk": sdkSrc } },
  server: {
    port: 5174,
    proxy: {
      "/api": "http://localhost:8080",
      "/healthz": "http://localhost:8080",
    },
  },
  test: {
    environment: "jsdom",
    setupFiles: ["./src/test/setup.ts"],
    css: false,
  },
});
```

`web/apps/admin/index.html`:
```html
<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>openforms admin</title>
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/src/main.tsx"></script>
  </body>
</html>
```

Run: `pnpm -C web install`
Expected: exits 0 and `web/pnpm-lock.yaml` now lists `apps/admin`.

- [ ] **Step 3: Write the failing tests for the lib helpers and components**

`web/apps/admin/src/lib/format.test.ts`:
```ts
import { describe, expect, it } from "vitest";
import { formatValue, relativeTime, shortId, submissionSummary } from "./format";

const now = new Date("2026-09-23T12:00:00Z");

describe("relativeTime", () => {
  it("formats seconds, minutes, hours and days relative to now", () => {
    expect(relativeTime("2026-09-23T12:00:00Z", now)).toBe("now");
    expect(relativeTime("2026-09-23T11:59:00Z", now)).toBe("1 minute ago");
    expect(relativeTime("2026-09-23T10:00:00Z", now)).toBe("2 hours ago");
    expect(relativeTime("2026-09-22T12:00:00Z", now)).toBe("yesterday");
  });
  it("falls back to a date after 30 days", () => {
    expect(relativeTime("2026-01-05T12:00:00Z", now)).toBe("Jan 5, 2026");
  });
});

describe("formatValue", () => {
  const select = { key: "role", type: "select", options: [{ value: "engineer", label: "Engineer" }, { value: "designer", label: "Designer" }] };
  it("renders empty values as an em dash", () => {
    expect(formatValue(undefined)).toBe("—");
    expect(formatValue(null)).toBe("—");
    expect(formatValue("")).toBe("—");
    expect(formatValue([])).toBe("—");
  });
  it("renders booleans, option labels, arrays and numbers", () => {
    expect(formatValue(true)).toBe("Yes");
    expect(formatValue(false)).toBe("No");
    expect(formatValue("engineer", select)).toBe("Engineer");
    expect(formatValue(["engineer", "designer"], { ...select, type: "multiselect" })).toBe("Engineer, Designer");
    expect(formatValue(12)).toBe("12");
    expect(formatValue("unknown", select)).toBe("unknown");
  });
  it("never renders [object Object]", () => {
    expect(formatValue({ a: 1 })).toBe('{"a":1}');
  });
});

describe("submissionSummary", () => {
  const form = { fields: [
    { key: "role", type: "select" },
    { key: "name", type: "text" },
    { key: "email", type: "email" },
    { key: "bio", type: "text" },
  ] };
  it("joins the first two non-empty text/email values in field order", () => {
    expect(submissionSummary({ role: "engineer", name: "Grace", email: "g@example.com", bio: "x" }, form)).toBe("Grace · g@example.com");
    expect(submissionSummary({ name: "  ", email: "g@example.com", bio: "Pioneer" }, form)).toBe("g@example.com · Pioneer");
  });
  it("falls back to string values in data order when the form is unknown", () => {
    expect(submissionSummary({ n: 3, a: "Alpha", b: "Beta", c: "Gamma" })).toBe("Alpha · Beta");
  });
  it("returns an em dash when there is nothing to show", () => {
    expect(submissionSummary({}, form)).toBe("—");
  });
});

describe("shortId", () => {
  it("keeps the first 8 characters", () => {
    expect(shortId("aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee")).toBe("aaaaaaaa");
  });
});
```

`web/apps/admin/src/lib/roles.test.ts`:
```ts
import { describe, expect, it } from "vitest";
import { formatRoles, parseRoles } from "./roles";

describe("roles", () => {
  it("parses comma-separated roles, trimming blanks and duplicates", () => {
    expect(parseRoles(" admin, reviewer ,,reviewer, hiring-manager ")).toEqual(["admin", "reviewer", "hiring-manager"]);
    expect(parseRoles("")).toEqual([]);
  });
  it("formats roles for an input", () => {
    expect(formatRoles(["admin", "reviewer"])).toBe("admin, reviewer");
  });
});
```

`web/apps/admin/src/components/StateBadge.test.tsx`:
```tsx
import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import { StateBadge, badgeColor } from "./StateBadge";

describe("StateBadge", () => {
  it("uses the workflow color", () => {
    render(<StateBadge label="Screening" color="blue" />);
    const badge = screen.getByText("Screening");
    expect(badge).toHaveClass("badge", "badge-blue");
    expect(badge).toHaveAttribute("data-color", "blue");
  });
  it("falls back to gray for unknown or missing colors", () => {
    expect(badgeColor("magenta")).toBe("gray");
    expect(badgeColor(undefined)).toBe("gray");
    expect(badgeColor(null)).toBe("gray");
    render(<StateBadge label="Submitted" />);
    expect(screen.getByText("Submitted")).toHaveAttribute("data-color", "gray");
  });
});
```

`web/apps/admin/src/components/Dialog.test.tsx`:
```tsx
import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Dialog } from "./Dialog";

describe("Dialog", () => {
  it("is a labelled modal dialog that focuses its first input", () => {
    render(<Dialog title="Add user" onClose={() => {}}><input aria-label="Name" /></Dialog>);
    const dialog = screen.getByRole("dialog", { name: "Add user" });
    expect(dialog).toHaveAttribute("aria-modal", "true");
    expect(screen.getByLabelText("Name")).toHaveFocus();
  });
  it("closes on Escape and on backdrop click, but not on inner click", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    render(<Dialog title="Confirm" onClose={onClose}><p>Body text</p></Dialog>);
    await user.click(screen.getByText("Body text"));
    expect(onClose).not.toHaveBeenCalled();
    await user.keyboard("{Escape}");
    expect(onClose).toHaveBeenCalledTimes(1);
    await user.click(screen.getByTestId("dialog-backdrop"));
    expect(onClose).toHaveBeenCalledTimes(2);
  });
});
```

`web/apps/admin/src/test/harness.test.tsx`:
```tsx
import { describe, expect, it } from "vitest";
import { http } from "msw";
import { client } from "../api";
import { qk } from "../queryKeys";
import { createQueryClient } from "../lib/queryClient";
import { errorCode, errorMessage, problemsByPath } from "../lib/errors";
import { server } from "./server";
import { api, apiError } from "./handlers";
import { makePrincipal } from "./fixtures";

describe("test harness", () => {
  it("serves fixture data through the real SDK singleton", async () => {
    await expect(client.me()).resolves.toEqual(makePrincipal());
  });

  it("invalidates the session when any query fails with 401", async () => {
    server.use(http.get(api("/forms"), () => apiError(401, "unauthenticated", "Sign in required")));
    const qc = createQueryClient({ test: true });
    qc.setQueryData(qk.me, makePrincipal());
    await qc.fetchQuery({ queryKey: qk.forms, queryFn: () => client.listForms() }).catch(() => undefined);
    expect(qc.getQueryState(qk.me)?.isInvalidated).toBe(true);
  });

  it("exposes error codes, messages and problem paths", async () => {
    server.use(http.get(api("/forms"), () =>
      apiError(422, "validation_failed", "Invalid", [{ path: "fields.score", message: "must be a number" }])));
    const err = await client.listForms().catch((e: unknown) => e);
    expect(errorCode(err)).toBe("validation_failed");
    expect(errorMessage(err)).toBe("Invalid");
    expect(problemsByPath(err)).toEqual({ "fields.score": "must be a number" });
    expect(errorMessage("weird")).toBe("Something went wrong.");
  });
});
```

- [ ] **Step 4: Run the tests to verify they fail**

Run: `pnpm -C web/apps/admin exec vitest run`
Expected: FAIL. Every suite errors with `Failed to resolve import "./format"` (and the same for `./roles`, `./StateBadge`, `./Dialog`) and `Cannot find module './test/setup.ts'` / `Failed to load setup file`.

- [ ] **Step 5: Write the API singleton, query keys, query client and lib helpers**

`web/apps/admin/src/api.ts`:
```ts
import { OpenFormsClient } from "@openforms/sdk";

/**
 * Resolves same-origin relative URLs against the page origin and looks up
 * `globalThis.fetch` at call time, so tests intercepted by MSW (which patches
 * fetch after module load) and non-browser runtimes both work.
 */
export function resolvingFetch(input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
  const url = typeof input === "string" && input.startsWith("/")
    ? new URL(input, window.location.origin).toString()
    : input;
  return globalThis.fetch(url, init);
}

export const client = new OpenFormsClient({ baseUrl: "", fetch: resolvingFetch as typeof fetch });
```

`web/apps/admin/src/queryKeys.ts`:
```ts
export type SubmissionFilter = { form?: string; state?: string; assignee?: string };

export const qk = {
  me: ["me"] as const,
  forms: ["forms"] as const,
  form: (slug: string) => ["forms", slug] as const,
  formVersions: (slug: string) => ["forms", slug, "versions"] as const,
  workflows: ["workflows"] as const,
  workflow: (slug: string) => ["workflows", slug] as const,
  workflowVersions: (slug: string) => ["workflows", slug, "versions"] as const,
  submissions: (filter: SubmissionFilter) => ["submissions", filter] as const,
  submission: (id: string) => ["submission", id] as const,
  users: ["users"] as const,
  apiKeys: ["api-keys"] as const,
  jobs: (status: string) => ["jobs", status] as const,
};
```

`web/apps/admin/src/lib/queryClient.ts`:
```ts
import { MutationCache, QueryCache, QueryClient } from "@tanstack/react-query";
import { OpenFormsError } from "@openforms/sdk";
import { qk } from "../queryKeys";

export function isUnauthenticated(err: unknown): boolean {
  return err instanceof OpenFormsError && err.status === 401;
}

/**
 * Any 401 from a non-session query or mutation invalidates the session query;
 * RequireSession then refetches /auth/me, gets 401 and redirects to /login.
 */
export function createQueryClient(opts: { test?: boolean } = {}): QueryClient {
  const onAuthError = (err: unknown) => {
    if (isUnauthenticated(err)) void queryClient.invalidateQueries({ queryKey: qk.me });
  };
  const queryClient: QueryClient = new QueryClient({
    queryCache: new QueryCache({
      onError: (err, query) => {
        if (query.queryKey[0] !== qk.me[0]) onAuthError(err);
      },
    }),
    mutationCache: new MutationCache({ onError: (err) => onAuthError(err) }),
    defaultOptions: {
      queries: {
        retry: opts.test
          ? false
          : (count, err) => !(err instanceof OpenFormsError && err.status < 500) && count < 2,
        refetchOnWindowFocus: !opts.test,
      },
      mutations: { retry: false },
    },
  });
  return queryClient;
}
```

`web/apps/admin/src/lib/errors.ts`:
```ts
import { OpenFormsError } from "@openforms/sdk";

export function errorMessage(err: unknown): string {
  if (err instanceof OpenFormsError) return err.message || err.code;
  if (err instanceof Error) return err.message;
  return "Something went wrong.";
}

export function errorCode(err: unknown): string | undefined {
  return err instanceof OpenFormsError ? err.code : undefined;
}

export function problemsByPath(err: unknown): Record<string, string> {
  if (!(err instanceof OpenFormsError)) return {};
  const out: Record<string, string> = {};
  for (const p of err.details ?? []) out[p.path] = p.message;
  return out;
}
```

`web/apps/admin/src/lib/format.ts`:
```ts
export type FieldLike = {
  key: string;
  type: string;
  label?: string;
  options?: { value: string; label: string }[];
};

export function relativeTime(iso: string, now: Date = new Date()): string {
  const diff = Math.round((new Date(iso).getTime() - now.getTime()) / 1000);
  const abs = Math.abs(diff);
  const rtf = new Intl.RelativeTimeFormat("en", { numeric: "auto" });
  if (abs < 60) return rtf.format(diff, "second");
  if (abs < 3600) return rtf.format(Math.round(diff / 60), "minute");
  if (abs < 86400) return rtf.format(Math.round(diff / 3600), "hour");
  if (abs < 86400 * 30) return rtf.format(Math.round(diff / 86400), "day");
  return new Date(iso).toLocaleDateString("en-US", { year: "numeric", month: "short", day: "numeric", timeZone: "UTC" });
}

function isEmptyValue(value: unknown): boolean {
  return value === undefined || value === null || value === "" || (Array.isArray(value) && value.length === 0);
}

export function formatValue(value: unknown, field?: FieldLike): string {
  if (isEmptyValue(value)) return "—";
  const label = (v: unknown) => {
    const option = field?.options?.find((o) => o.value === v);
    return option ? option.label : String(v);
  };
  if (typeof value === "boolean") return value ? "Yes" : "No";
  if (Array.isArray(value)) return value.map(label).join(", ");
  if (typeof value === "object") return JSON.stringify(value);
  return label(value);
}

export function submissionSummary(data: Record<string, unknown>, form?: { fields: FieldLike[] }): string {
  const keys = form
    ? form.fields.filter((f) => f.type === "text" || f.type === "email").map((f) => f.key)
    : Object.keys(data);
  const parts: string[] = [];
  for (const key of keys) {
    const v = data[key];
    if (typeof v === "string" && v.trim() !== "") parts.push(v.trim());
    if (parts.length === 2) break;
  }
  return parts.length ? parts.join(" · ") : "—";
}

export function shortId(id: string): string {
  return id.slice(0, 8);
}
```

`web/apps/admin/src/lib/roles.ts`:
```ts
export function parseRoles(input: string): string[] {
  const out: string[] = [];
  for (const part of input.split(",")) {
    const role = part.trim();
    if (role && !out.includes(role)) out.push(role);
  }
  return out;
}

export function formatRoles(roles: string[]): string {
  return roles.join(", ");
}
```

- [ ] **Step 6: Write the shared components**

`web/apps/admin/src/components/StateBadge.tsx`:
```tsx
const COLORS = ["gray", "blue", "green", "yellow", "red", "purple"] as const;
export type BadgeColor = (typeof COLORS)[number];

export function badgeColor(color?: string | null): BadgeColor {
  return (COLORS as readonly string[]).includes(color ?? "") ? (color as BadgeColor) : "gray";
}

export function StateBadge({ label, color }: { label: string; color?: string | null }) {
  const c = badgeColor(color);
  return (
    <span className={`badge badge-${c}`} data-color={c}>
      {label}
    </span>
  );
}
```

`web/apps/admin/src/components/Dialog.tsx`:
```tsx
import { useEffect, useId, useRef, type ReactNode } from "react";

export function Dialog({ title, onClose, children }: { title: string; onClose: () => void; children: ReactNode }) {
  const titleId = useId();
  const panel = useRef<HTMLDivElement>(null);
  const onCloseRef = useRef(onClose);
  onCloseRef.current = onClose;

  useEffect(() => {
    const first = panel.current?.querySelector<HTMLElement>("[data-autofocus], input, select, textarea, button");
    first?.focus();
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onCloseRef.current();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, []);

  return (
    <div
      className="dialog-backdrop"
      data-testid="dialog-backdrop"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onCloseRef.current();
      }}
    >
      <div ref={panel} className="dialog" role="dialog" aria-modal="true" aria-labelledby={titleId}>
        <h2 id={titleId} className="dialog-title">{title}</h2>
        {children}
      </div>
    </div>
  );
}
```

`web/apps/admin/src/components/ErrorMessage.tsx`:
```tsx
import { errorMessage } from "../lib/errors";

export function ErrorMessage({ error }: { error: unknown }) {
  return <p role="alert" className="error">{errorMessage(error)}</p>;
}
```

`web/apps/admin/src/components/RelativeTime.tsx`:
```tsx
import { relativeTime } from "../lib/format";

export function RelativeTime({ iso }: { iso: string }) {
  return (
    <time dateTime={iso} title={new Date(iso).toLocaleString()}>
      {relativeTime(iso)}
    </time>
  );
}
```

`web/apps/admin/src/components/CopyButton.tsx`:
```tsx
import { useState } from "react";

export function CopyButton({ text, label = "Copy" }: { text: string; label?: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <button
      type="button"
      className="btn"
      onClick={async () => {
        try {
          await navigator.clipboard.writeText(text);
          setCopied(true);
        } catch {
          setCopied(false);
        }
      }}
    >
      {copied ? "Copied" : label}
    </button>
  );
}
```

`web/apps/admin/src/components/Loading.tsx`:
```tsx
export function Loading() {
  return <p role="status" className="muted">Loading…</p>;
}
```

`web/apps/admin/src/components/RoleTags.tsx`:
```tsx
export function RoleTags({ roles }: { roles: string[] }) {
  if (!roles.length) return <span className="muted">No roles</span>;
  return (
    <span className="tags">
      {roles.map((r) => (
        <span key={r} className="tag">{r}</span>
      ))}
    </span>
  );
}
```

- [ ] **Step 7: Write the design tokens and app styles**

`web/apps/admin/src/styles/tokens.css`:
```css
:root {
  --font: ui-sans-serif, system-ui, -apple-system, "Segoe UI", Roboto, "Helvetica Neue", sans-serif;
  --mono: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  --text-xs: 12px; --text-sm: 13px; --text-md: 14px; --text-lg: 18px; --text-xl: 22px;
  --space-1: 4px; --space-2: 8px; --space-3: 12px; --space-4: 16px; --space-5: 20px; --space-6: 24px; --space-8: 32px;
  --radius: 6px; --radius-lg: 10px;

  --bg: #f6f7f9; --surface: #ffffff; --surface-2: #f1f3f5; --border: #e1e4e8; --border-strong: #c9ced6;
  --text: #1b1f24; --text-muted: #5e6672; --accent: #3651c9; --accent-hover: #2b43ad; --accent-contrast: #ffffff;
  --danger: #c0262d; --danger-bg: #fdecec; --notice-bg: #fff6d6; --notice-fg: #6b4e00; --focus: #6b83f2;
  --shadow: 0 8px 28px rgba(15, 23, 42, 0.18);

  --badge-gray-bg: #eceef1;   --badge-gray-fg: #454c56;
  --badge-blue-bg: #e5eeff;   --badge-blue-fg: #1d4fb3;
  --badge-green-bg: #e3f5e8;  --badge-green-fg: #1d6b31;
  --badge-yellow-bg: #fff3cc; --badge-yellow-fg: #7a5800;
  --badge-red-bg: #fde8e8;    --badge-red-fg: #a3191f;
  --badge-purple-bg: #efe8fd; --badge-purple-fg: #5b36bf;
}

@media (prefers-color-scheme: dark) {
  :root {
    --bg: #0f1115; --surface: #171a20; --surface-2: #1f232b; --border: #2a2f38; --border-strong: #3a414d;
    --text: #e6e8ec; --text-muted: #9aa3b0; --accent: #7d93ff; --accent-hover: #97a8ff; --accent-contrast: #0f1115;
    --danger: #ff7b80; --danger-bg: #3a1a1c; --notice-bg: #3a3014; --notice-fg: #f5d77a; --focus: #7d93ff;
    --shadow: 0 8px 28px rgba(0, 0, 0, 0.5);

    --badge-gray-bg: #2a2f38;   --badge-gray-fg: #c6ccd5;
    --badge-blue-bg: #1b2a4d;   --badge-blue-fg: #9dbbff;
    --badge-green-bg: #16341f;  --badge-green-fg: #8fdca3;
    --badge-yellow-bg: #3a3014; --badge-yellow-fg: #f5d77a;
    --badge-red-bg: #3a1a1c;    --badge-red-fg: #ff9ca0;
    --badge-purple-bg: #2c2148; --badge-purple-fg: #c5b0ff;
  }
}
```

`web/apps/admin/src/styles/app.css`:
```css
@import "./tokens.css";

* { box-sizing: border-box; }
html, body, #root { height: 100%; }
body { margin: 0; font-family: var(--font); font-size: var(--text-md); color: var(--text); background: var(--bg); line-height: 1.45; }
a { color: var(--accent); text-decoration: none; }
a:hover { text-decoration: underline; }
code, .mono { font-family: var(--mono); font-size: 0.92em; }
h1 { font-size: var(--text-xl); margin: 0; }
h2 { font-size: var(--text-md); margin: 0 0 var(--space-3); text-transform: uppercase; letter-spacing: 0.04em; color: var(--text-muted); }
:focus-visible { outline: 2px solid var(--focus); outline-offset: 2px; }
.sr-only { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }
.muted { color: var(--text-muted); }

.shell { display: grid; grid-template-columns: 220px 1fr; min-height: 100%; }
.sidebar { background: var(--surface); border-right: 1px solid var(--border); display: flex; flex-direction: column; padding: var(--space-4); gap: var(--space-4); }
.brand { font-weight: 700; font-size: var(--text-lg); letter-spacing: -0.01em; }
.sidebar ul { list-style: none; margin: 0; padding: 0; display: grid; gap: 2px; }
.nav-link { display: block; padding: var(--space-2) var(--space-3); border-radius: var(--radius); color: var(--text); }
.nav-link:hover { background: var(--surface-2); text-decoration: none; }
.nav-link.active { background: var(--surface-2); font-weight: 600; }
.sidebar-footer { margin-top: auto; display: grid; gap: var(--space-2); font-size: var(--text-sm); }
.who { display: grid; }
.main { padding: var(--space-6) var(--space-8); min-width: 0; }

.page { display: grid; gap: var(--space-5); max-width: 1200px; }
.page-header { display: flex; align-items: flex-start; justify-content: space-between; gap: var(--space-4); }
.breadcrumb { font-size: var(--text-sm); color: var(--text-muted); }
.card { background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius-lg); padding: var(--space-4) var(--space-5); }
.button-row { display: flex; flex-wrap: wrap; gap: var(--space-2); align-items: center; }
.toolbar { display: flex; flex-wrap: wrap; gap: var(--space-4); align-items: flex-end; }
.toolbar .field { min-width: 180px; }
.empty { color: var(--text-muted); padding: var(--space-6) 0; }

.btn { display: inline-flex; align-items: center; gap: var(--space-2); font: inherit; font-size: var(--text-sm); padding: 6px 12px; border-radius: var(--radius); border: 1px solid var(--border-strong); background: var(--surface); color: var(--text); cursor: pointer; }
.btn:hover:not(:disabled) { background: var(--surface-2); text-decoration: none; }
.btn:disabled { opacity: 0.5; cursor: not-allowed; }
.btn-primary { background: var(--accent); border-color: var(--accent); color: var(--accent-contrast); }
.btn-primary:hover:not(:disabled) { background: var(--accent-hover); }
.btn-danger { background: var(--danger); border-color: var(--danger); color: #fff; }
.btn-ghost { border-color: transparent; background: transparent; }

.field { display: grid; gap: var(--space-1); margin-bottom: var(--space-3); }
.field label, .toolbar label { font-size: var(--text-sm); font-weight: 600; }
.field-checkbox label { font-weight: 400; display: flex; gap: var(--space-2); align-items: center; }
input, select, textarea { font: inherit; font-size: var(--text-md); padding: 6px 8px; border: 1px solid var(--border-strong); border-radius: var(--radius); background: var(--surface); color: var(--text); width: 100%; }
input[type="checkbox"] { width: auto; }
[aria-invalid="true"] { border-color: var(--danger); }
.field-help { font-size: var(--text-xs); color: var(--text-muted); margin: 0; }
.field-error { font-size: var(--text-xs); color: var(--danger); margin: 0; }
.error { color: var(--danger); background: var(--danger-bg); padding: var(--space-2) var(--space-3); border-radius: var(--radius); margin: 0; }
.notice { color: var(--notice-fg); background: var(--notice-bg); padding: var(--space-2) var(--space-3); border-radius: var(--radius); margin: 0 0 var(--space-3); }

.table { width: 100%; border-collapse: collapse; font-size: var(--text-sm); background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius-lg); overflow: hidden; }
.table th { text-align: left; font-weight: 600; color: var(--text-muted); background: var(--surface-2); padding: var(--space-2) var(--space-3); border-bottom: 1px solid var(--border); white-space: nowrap; }
.table td { padding: var(--space-2) var(--space-3); border-bottom: 1px solid var(--border); vertical-align: top; }
.table tr:last-child td { border-bottom: none; }
.card .table { border: none; }

.badge { display: inline-block; font-size: var(--text-xs); font-weight: 600; padding: 2px 8px; border-radius: 999px; white-space: nowrap; }
.badge-gray { background: var(--badge-gray-bg); color: var(--badge-gray-fg); }
.badge-blue { background: var(--badge-blue-bg); color: var(--badge-blue-fg); }
.badge-green { background: var(--badge-green-bg); color: var(--badge-green-fg); }
.badge-yellow { background: var(--badge-yellow-bg); color: var(--badge-yellow-fg); }
.badge-red { background: var(--badge-red-bg); color: var(--badge-red-fg); }
.badge-purple { background: var(--badge-purple-bg); color: var(--badge-purple-fg); }
.tags { display: inline-flex; flex-wrap: wrap; gap: var(--space-1); }
.tag { font-size: var(--text-xs); padding: 1px 6px; border-radius: var(--radius); background: var(--surface-2); border: 1px solid var(--border); }

.dialog-backdrop { position: fixed; inset: 0; background: rgba(15, 23, 42, 0.45); display: grid; place-items: center; padding: var(--space-4); z-index: 50; }
.dialog { background: var(--surface); border-radius: var(--radius-lg); box-shadow: var(--shadow); padding: var(--space-5); width: min(520px, 100%); max-height: 90vh; overflow: auto; }
.dialog-title { font-size: var(--text-lg); text-transform: none; letter-spacing: 0; color: var(--text); margin-bottom: var(--space-4); }
.dialog-footer { display: flex; justify-content: flex-end; gap: var(--space-2); margin-top: var(--space-4); }

.detail { display: grid; grid-template-columns: minmax(0, 1fr) 320px; gap: var(--space-5); align-items: start; }
.detail-main, .detail-side { display: grid; gap: var(--space-4); }
.answers { margin: 0; display: grid; gap: var(--space-3); }
.answer dt { font-size: var(--text-sm); color: var(--text-muted); }
.answer dd { margin: 0; white-space: pre-wrap; overflow-wrap: anywhere; }
.meta { display: flex; flex-wrap: wrap; gap: var(--space-6); margin: 0; }
.meta dt { font-size: var(--text-xs); color: var(--text-muted); text-transform: uppercase; letter-spacing: 0.04em; }
.meta dd { margin: 0; }
.transition-buttons { display: flex; flex-wrap: wrap; gap: var(--space-2); }
.timeline { list-style: none; margin: 0; padding: 0; display: grid; gap: var(--space-3); }
.timeline-item { border-left: 2px solid var(--border); padding-left: var(--space-3); font-size: var(--text-sm); }
.timeline-item.tone-danger { border-left-color: var(--danger); }
.timeline-title { font-weight: 600; }
.timeline-body { margin: var(--space-1) 0; white-space: pre-wrap; }
.timeline-item time { color: var(--text-muted); font-size: var(--text-xs); }
.comment-box { display: grid; gap: var(--space-2); margin-bottom: var(--space-4); justify-items: start; }
.tabs { display: flex; gap: var(--space-1); }
.tabs .btn[aria-pressed="true"] { background: var(--surface-2); font-weight: 600; }
.login { min-height: 100%; display: grid; place-items: center; padding: var(--space-4); }
.login-card { width: min(380px, 100%); display: grid; gap: var(--space-3); }
.key-display { font-family: var(--mono); }
```

- [ ] **Step 8: Write the test harness**

`web/apps/admin/src/test/fixtures.ts`:
```ts
import type {
  ApiKey, AvailableTransition, FormDefinition, FormRecord, FormSummary, Job, Principal, Submission,
  SubmissionDetail, SubmissionEvent, User, VersionInfo, WorkflowDefinition, WorkflowRecord, WorkflowSummary,
} from "@openforms/sdk";

export const ids = {
  admin: "11111111-1111-4111-8111-111111111111",
  reviewer: "22222222-2222-4222-8222-222222222222",
  manager: "33333333-3333-4333-8333-333333333333",
  sub1: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
  sub2: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
  key1: "cccccccc-cccc-4ccc-8ccc-cccccccccccc",
  key2: "dddddddd-dddd-4ddd-8ddd-dddddddddddd",
};

const T0 = "2026-09-23T11:00:00Z";

export function makeFormDefinition(o: Partial<FormDefinition> = {}): FormDefinition {
  return {
    slug: "job-application",
    title: "Job application",
    description: "Apply to join the team.",
    workflow: "hiring",
    settings: { public: true, submitLabel: "Send application" },
    fields: [
      { key: "name", type: "text", label: "Full name", required: true },
      { key: "email", type: "email", label: "Email", required: true },
      { key: "role", type: "select", label: "Role", required: true, options: [
        { value: "engineer", label: "Engineer" }, { value: "designer", label: "Designer" },
      ] },
      { key: "portfolio", type: "url", label: "Portfolio URL", showIf: { field: "role", equals: "designer" } },
      { key: "years", type: "number", label: "Years of experience" },
      { key: "relocate", type: "checkbox", label: "Willing to relocate" },
    ],
    ...o,
  } as FormDefinition;
}

export function makeWorkflowDefinition(o: Partial<WorkflowDefinition> = {}): WorkflowDefinition {
  return {
    slug: "hiring",
    title: "Hiring pipeline",
    initial: "new",
    states: [
      { key: "new", label: "New", color: "gray" },
      { key: "screening", label: "Screening", color: "blue" },
      { key: "interview", label: "Interview", color: "purple" },
      { key: "hired", label: "Hired", color: "green", terminal: true },
      { key: "rejected", label: "Rejected", color: "red", terminal: true },
    ],
    fields: [
      { key: "score", type: "number", label: "Score" },
      { key: "rejectionReason", type: "textarea", label: "Rejection reason" },
    ],
    onSubmit: [{ type: "assign", role: "reviewer" }],
    transitions: [
      { key: "screen", label: "Start screening", from: ["new"], to: "screening", guard: { roles: ["reviewer"] } },
      { key: "invite", label: "Invite to interview", from: ["screening"], to: "interview",
        guard: { roles: ["reviewer"], requireFields: ["score"] },
        actions: [{ type: "webhook", url: "https://example.com/hooks/interview" }] },
      { key: "hire", label: "Hire", from: ["interview"], to: "hired", guard: { roles: ["hiring-manager"] } },
      { key: "reject", label: "Reject", from: ["new", "screening", "interview"], to: "rejected",
        guard: { roles: ["reviewer", "hiring-manager"], requireFields: ["rejectionReason"] },
        actions: [{ type: "email", to: "{{submission.data.email}}", subject: "Your application", body: "{{submission.fields.rejectionReason}}" }] },
    ],
    ...o,
  } as WorkflowDefinition;
}

export function makePrincipal(o: Partial<Principal> = {}): Principal {
  return { kind: "user", id: ids.admin, name: "Ada Admin", email: "admin@example.com", roles: ["admin"], ...o } as Principal;
}

export function makeReviewerPrincipal(o: Partial<Principal> = {}): Principal {
  return makePrincipal({ id: ids.reviewer, name: "Rita Reviewer", email: "reviewer@example.com", roles: ["reviewer"], ...o });
}

export function makeUser(o: Partial<User> = {}): User {
  return { id: ids.reviewer, email: "reviewer@example.com", name: "Rita Reviewer", roles: ["reviewer"], createdAt: "2026-09-01T09:00:00Z", ...o } as User;
}

export function makeApiKey(o: Partial<ApiKey> = {}): ApiKey {
  return { id: ids.key1, name: "CI deploy", prefix: "ofk_Ab12", roles: ["admin"], createdAt: "2026-09-10T09:00:00Z", lastUsedAt: null, revokedAt: null, ...o } as ApiKey;
}

export function makeJob(o: Partial<Job> = {}): Job {
  return {
    id: 7, kind: "action.webhook", status: "failed", attempts: 8, maxAttempts: 8, runAt: T0,
    lastError: "POST https://example.com/hooks/interview: 500 Internal Server Error",
    payload: { submissionId: ids.sub1, trigger: "invite", eventId: 4, action: { type: "webhook", url: "https://example.com/hooks/interview" } },
    createdAt: T0, updatedAt: T0, ...o,
  } as Job;
}

export function makeSubmission(o: Partial<Submission> = {}): Submission {
  return {
    id: ids.sub1, form: "job-application", formVersion: 1, state: "new", stateLabel: "New", terminal: false,
    data: { name: "Grace Hopper", email: "grace@example.com", role: "engineer", years: 12, relocate: true },
    fields: {}, assignee: null, createdAt: T0, updatedAt: T0, ...o,
  } as Submission;
}

export function makeEvent(o: Partial<SubmissionEvent> = {}): SubmissionEvent {
  return {
    id: 1, type: "created", fromState: null, toState: "new", transition: null,
    actor: { type: "respondent", id: null, name: "" }, payload: {}, createdAt: T0, ...o,
  } as SubmissionEvent;
}

export function makeTransition(o: Partial<AvailableTransition> = {}): AvailableTransition {
  return { key: "screen", label: "Start screening", to: "screening", toLabel: "Screening", requireFields: [], allowed: true, reason: "", ...o } as AvailableTransition;
}

export function makeDetail(o: Partial<SubmissionDetail> = {}): SubmissionDetail {
  return {
    submission: makeSubmission(),
    form: makeFormDefinition(),
    workflow: makeWorkflowDefinition(),
    events: [makeEvent()],
    transitions: [
      makeTransition(),
      makeTransition({ key: "reject", label: "Reject", to: "rejected", toLabel: "Rejected", requireFields: ["rejectionReason"] }),
    ],
    ...o,
  } as SubmissionDetail;
}

export function makeFormSummary(o: Partial<FormSummary> = {}): FormSummary {
  return { slug: "job-application", title: "Job application", workflow: "hiring", public: true, version: 3, source: "cli", updatedAt: T0, submissionCount: 12, ...o } as FormSummary;
}

export function makeWorkflowSummary(o: Partial<WorkflowSummary> = {}): WorkflowSummary {
  return { slug: "hiring", title: "Hiring pipeline", version: 2, source: "cli", updatedAt: T0, stateCount: 5, ...o } as WorkflowSummary;
}

export function makeFormRecord(o: Partial<FormRecord> = {}): FormRecord {
  return { slug: "job-application", version: 3, source: "cli", updatedAt: T0, workflowVersion: 2, definition: makeFormDefinition(), ...o } as FormRecord;
}

export function makeWorkflowRecord(o: Partial<WorkflowRecord> = {}): WorkflowRecord {
  return { slug: "hiring", version: 2, source: "cli", updatedAt: T0, definition: makeWorkflowDefinition(), ...o } as WorkflowRecord;
}

export function makeVersionInfo(o: Partial<VersionInfo> = {}): VersionInfo {
  return { version: 3, hash: "3f2a9c41d07be5aa90e1c2b7f4d6e8a1b3c5d7e9f0a2b4c6d8e0f1a3b5c7d9e1", source: "cli", createdBy: "ci", createdAt: T0, ...o } as VersionInfo;
}
```

`web/apps/admin/src/test/handlers.ts`:
```ts
import { http, HttpResponse } from "msw";
import * as f from "./fixtures";

export const API = `${window.location.origin}/api/v1`;
export const api = (path: string) => `${API}${path}`;

export function apiError(status: number, code: string, message: string, details?: { path: string; message: string }[]) {
  return HttpResponse.json({ error: { code, message, ...(details ? { details } : {}) } }, { status });
}

/** Happy-path defaults: an admin is signed in and one form/workflow/submission exists. */
export const defaultHandlers = [
  http.get(api("/auth/me"), () => HttpResponse.json({ principal: f.makePrincipal() })),
  http.post(api("/auth/logout"), () => new HttpResponse(null, { status: 204 })),
  http.get(api("/forms"), () => HttpResponse.json({ items: [f.makeFormSummary()] })),
  http.get(api("/forms/:slug"), ({ params }) =>
    params.slug === "job-application"
      ? HttpResponse.json({ form: f.makeFormRecord() })
      : apiError(404, "not_found", "form not found")),
  http.get(api("/forms/:slug/versions"), () =>
    HttpResponse.json({ items: [f.makeVersionInfo(), f.makeVersionInfo({ version: 2, source: "ui", createdBy: "admin@example.com" })] })),
  http.get(api("/workflows"), () => HttpResponse.json({ items: [f.makeWorkflowSummary()] })),
  http.get(api("/workflows/:slug"), ({ params }) =>
    params.slug === "hiring"
      ? HttpResponse.json({ workflow: f.makeWorkflowRecord() })
      : apiError(404, "not_found", "workflow not found")),
  http.get(api("/workflows/:slug/versions"), () =>
    HttpResponse.json({ items: [f.makeVersionInfo({ version: 2 }), f.makeVersionInfo({ version: 1, source: "seed", createdBy: "" })] })),
  http.get(api("/submissions"), () => HttpResponse.json({ items: [f.makeSubmission()], nextCursor: null })),
  http.get(api("/users"), () => HttpResponse.json({ items: [
    f.makeUser({ id: ids().admin, email: "admin@example.com", name: "Ada Admin", roles: ["admin"] }),
    f.makeUser(),
  ] })),
];

function ids() {
  return f.ids;
}
```

`web/apps/admin/src/test/server.ts`:
```ts
import { setupServer } from "msw/node";
import { defaultHandlers } from "./handlers";

export const server = setupServer(...defaultHandlers);
```

`web/apps/admin/src/test/setup.ts`:
```ts
import "@testing-library/jest-dom/vitest";
import { afterAll, afterEach, beforeAll } from "vitest";
import { cleanup } from "@testing-library/react";
import { server } from "./server";

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => {
  cleanup();
  server.resetHandlers();
});
afterAll(() => server.close());
```

`web/apps/admin/src/test/render.tsx`:
```tsx
import type { ReactElement } from "react";
import { render } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createMemoryRouter, RouterProvider } from "react-router-dom";
import { createQueryClient } from "../lib/queryClient";

/**
 * Renders `ui` at route pattern `path` (default "/") with the memory router
 * starting at `route` (default: `path`). Paths are relative to the /admin basename.
 */
export function renderWithProviders(ui: ReactElement, opts: { route?: string; path?: string } = {}) {
  const path = opts.path ?? "/";
  const route = opts.route ?? path;
  const queryClient = createQueryClient({ test: true });
  const router = createMemoryRouter([{ path, element: ui }, { path: "*", element: <p>Navigated away</p> }], { initialEntries: [route] });
  const user = userEvent.setup();
  const utils = render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
  return { ...utils, user, router, queryClient };
}
```

- [ ] **Step 9: Run the tests to verify they pass**

Run: `pnpm -C web/apps/admin exec vitest run`
Expected: PASS. The five suites (format, roles, StateBadge, Dialog, harness) are green, with 0 failed.

Run: `pnpm -C web/apps/admin typecheck`
Expected: exits 0 with no output. If a type error comes from an SDK signature that differs from the "Consumed SDK contract", adapt the call site (`src/test/harness.test.tsx`, `src/api.ts`) and re-run.

- [ ] **Step 10: Commit**

```bash
git add web/apps/admin web/pnpm-lock.yaml web/pnpm-workspace.yaml
git commit -m "feat(admin): scaffold admin app, design tokens, shared components and test harness"
```

---

### Task 2: Session, sign-in, shell, route table and page stubs

**Files:**
- Create: `web/apps/admin/src/routes.tsx`, `src/nav.ts`, `src/App.tsx`, `src/main.tsx`, `src/lib/session.ts`
- Create: `src/components/RequireSession.tsx`, `src/components/RequireAdmin.tsx`, `src/components/Shell.tsx`
- Create: `src/pages/LoginPage.tsx`, `src/pages/NotFoundPage.tsx`
- Create (stubs, replaced by Tasks 3–10): `src/pages/InboxPage.tsx`, `SubmissionPage.tsx`, `FormsPage.tsx`, `FormOverviewPage.tsx`, `WorkflowsPage.tsx`, `WorkflowOverviewPage.tsx`, `UsersPage.tsx`, `ApiKeysPage.tsx`, `JobsPage.tsx`
- Modify: `src/test/render.tsx` (add `renderApp`)
- Test: `src/pages/LoginPage.test.tsx`, `src/components/Shell.test.tsx`

**Interfaces:**
- Consumes: `client`, `qk`, `createQueryClient`, `errorCode`, `errorMessage`, `Loading`, `ErrorMessage` from Task 1.
- Produces:
  - `src/routes.tsx`: `export type AdminRoute = { path: string; element: React.ReactNode; adminOnly?: boolean }` and `export const routes: AdminRoute[]`. Paths are relative to the `/admin` basename, e.g. `"submissions/:id"`. **Plan 08 appends** `{ path: "forms/new", … }`, `"forms/:slug/edit"`, `"workflows/new"` and `"workflows/:slug/edit"` (with `adminOnly: true`) plus their imports.
  - `src/nav.ts`: `export type NavItem = { to: string; label: string; adminOnly?: boolean }`, `export const navItems: NavItem[]`.
  - `src/App.tsx`: `export function buildRoutes(): RouteObject[]` (the login route, plus the authenticated shell that maps `routes`; `adminOnly` wraps in `<RequireAdmin>`), `export function routerBasename(): string`, `export function App()`.
  - `src/lib/session.ts`: `useSession()` (a TanStack query of `client.me()`) and `isAdmin(p?: Principal): boolean`.
  - `RequireSession({children})`, `RequireAdmin({children})`, `Shell()` (renders `<Outlet/>`).
  - `src/pages/LoginPage.tsx`: `LoginPage` and `safeNext(next: string | null): string`.
  - `src/test/render.tsx`: `renderApp(route: string)`, which renders the full app route table in a memory router.

- [ ] **Step 1: Write the failing tests**

`web/apps/admin/src/pages/LoginPage.test.tsx`:
```tsx
import { describe, expect, it } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { server } from "../test/server";
import { api, apiError } from "../test/handlers";
import { ids, makeDetail, makePrincipal, makeUser } from "../test/fixtures";
import { renderApp } from "../test/render";
import { safeNext } from "./LoginPage";

function signedOutUntilLogin(opts: { reject?: boolean } = {}) {
  let signedIn = false;
  const logins: unknown[] = [];
  server.use(
    http.get(api("/auth/me"), () =>
      signedIn ? HttpResponse.json({ principal: makePrincipal() }) : apiError(401, "unauthenticated", "Sign in required")),
    http.post(api("/auth/login"), async ({ request }) => {
      logins.push(await request.json());
      if (opts.reject) return apiError(401, "invalid_credentials", "invalid email or password");
      signedIn = true;
      return HttpResponse.json({ user: makeUser({ id: ids.admin, email: "admin@example.com", name: "Ada Admin", roles: ["admin"] }) });
    }),
  );
  return { logins };
}

describe("LoginPage", () => {
  it("redirects signed-out visitors to login and returns them to the deep link", async () => {
    const { logins } = signedOutUntilLogin();
    server.use(http.get(api("/submissions/:id"), () => HttpResponse.json(makeDetail())));
    const { user, router } = renderApp(`/submissions/${ids.sub1}`);

    await screen.findByRole("heading", { name: "Sign in to openforms" });
    expect(router.state.location.pathname).toBe("/login");
    expect(router.state.location.search).toBe(`?next=${encodeURIComponent(`/submissions/${ids.sub1}`)}`);

    await user.type(screen.getByLabelText("Email"), "admin@example.com");
    await user.type(screen.getByLabelText("Password"), "correct-horse");
    await user.click(screen.getByRole("button", { name: "Sign in" }));

    await waitFor(() => expect(router.state.location.pathname).toBe(`/submissions/${ids.sub1}`));
    expect(logins).toEqual([{ email: "admin@example.com", password: "correct-horse" }]);
  });

  it("shows a friendly message for wrong credentials", async () => {
    signedOutUntilLogin({ reject: true });
    const { user } = renderApp("/login");
    await user.type(await screen.findByLabelText("Email"), "admin@example.com");
    await user.type(screen.getByLabelText("Password"), "wrong-password");
    await user.click(screen.getByRole("button", { name: "Sign in" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Email or password is incorrect.");
  });

  it("never redirects off-site after login", () => {
    expect(safeNext(null)).toBe("/submissions");
    expect(safeNext("")).toBe("/submissions");
    expect(safeNext("//evil.example/x")).toBe("/submissions");
    expect(safeNext("https://evil.example")).toBe("/submissions");
    expect(safeNext("/\\evil.example")).toBe("/submissions");
    expect(safeNext("/forms?x=1")).toBe("/forms?x=1");
  });
});
```

`web/apps/admin/src/components/Shell.test.tsx`:
```tsx
import { describe, expect, it } from "vitest";
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { server } from "../test/server";
import { api } from "../test/handlers";
import { makeReviewerPrincipal } from "../test/fixtures";
import { renderApp } from "../test/render";

describe("Shell", () => {
  it("redirects / to the inbox and shows admin navigation to admins", async () => {
    const { router } = renderApp("/");
    const nav = await screen.findByRole("navigation", { name: "Main" });
    await waitFor(() => expect(router.state.location.pathname).toBe("/submissions"));
    for (const label of ["Inbox", "Forms", "Workflows", "Users", "API keys", "Jobs"]) {
      expect(within(nav).getByRole("link", { name: label })).toBeInTheDocument();
    }
    expect(screen.getByText("Ada Admin")).toBeInTheDocument();
  });

  it("hides admin-only navigation and blocks admin pages for reviewers", async () => {
    server.use(http.get(api("/auth/me"), () => HttpResponse.json({ principal: makeReviewerPrincipal() })));
    renderApp("/users");
    const nav = await screen.findByRole("navigation", { name: "Main" });
    expect(within(nav).queryByRole("link", { name: "Users" })).not.toBeInTheDocument();
    expect(within(nav).getByRole("link", { name: "Inbox" })).toBeInTheDocument();
    expect(await screen.findByRole("heading", { name: "Admins only" })).toBeInTheDocument();
  });

  it("signs out and returns to the login page", async () => {
    let loggedOut = false;
    server.use(http.post(api("/auth/logout"), () => {
      loggedOut = true;
      return new HttpResponse(null, { status: 204 });
    }));
    const { user, router } = renderApp("/submissions");
    await user.click(await screen.findByRole("button", { name: "Sign out" }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/login"));
    expect(loggedOut).toBe(true);
  });

  it("shows a not-found page for unknown admin paths", async () => {
    renderApp("/nope");
    expect(await screen.findByRole("heading", { name: "Page not found" })).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `pnpm -C web/apps/admin exec vitest run src/pages/LoginPage.test.tsx src/components/Shell.test.tsx`
Expected: FAIL with `Failed to resolve import "./LoginPage"` and `SyntaxError: The requested module '../test/render' does not provide an export named 'renderApp'`.

- [ ] **Step 3: Write the session hook, guards and shell**

`web/apps/admin/src/lib/session.ts`:
```ts
import { useQuery } from "@tanstack/react-query";
import type { Principal } from "@openforms/sdk";
import { client } from "../api";
import { qk } from "../queryKeys";

export function useSession() {
  return useQuery({ queryKey: qk.me, queryFn: () => client.me(), retry: false, staleTime: 5 * 60_000 });
}

export function isAdmin(p?: Principal | null): boolean {
  return !!p && p.roles.includes("admin");
}
```

`web/apps/admin/src/components/RequireSession.tsx`:
```tsx
import type { ReactNode } from "react";
import { Navigate, useLocation } from "react-router-dom";
import { isUnauthenticated } from "../lib/queryClient";
import { useSession } from "../lib/session";
import { ErrorMessage } from "./ErrorMessage";
import { Loading } from "./Loading";

export function RequireSession({ children }: { children: ReactNode }) {
  const session = useSession();
  const location = useLocation();
  if (session.isError) {
    if (isUnauthenticated(session.error)) {
      const next = location.pathname + location.search;
      return <Navigate to={`/login?next=${encodeURIComponent(next)}`} replace />;
    }
    return <main className="main"><ErrorMessage error={session.error} /></main>;
  }
  if (session.isPending) return <main className="main"><Loading /></main>;
  return <>{children}</>;
}
```

`web/apps/admin/src/components/RequireAdmin.tsx`:
```tsx
import type { ReactNode } from "react";
import { Link } from "react-router-dom";
import { isAdmin, useSession } from "../lib/session";

export function RequireAdmin({ children }: { children: ReactNode }) {
  const { data: me } = useSession();
  if (!isAdmin(me)) {
    return (
      <div className="page">
        <h1>Admins only</h1>
        <p className="muted">
          Your account doesn't have the <code>admin</code> role. <Link to="/submissions">Back to the inbox</Link>
        </p>
      </div>
    );
  }
  return <>{children}</>;
}
```

`web/apps/admin/src/nav.ts`:
```ts
export type NavItem = { to: string; label: string; adminOnly?: boolean };

export const navItems: NavItem[] = [
  { to: "/submissions", label: "Inbox" },
  { to: "/forms", label: "Forms" },
  { to: "/workflows", label: "Workflows" },
  { to: "/users", label: "Users", adminOnly: true },
  { to: "/api-keys", label: "API keys", adminOnly: true },
  { to: "/jobs", label: "Jobs", adminOnly: true },
];
```

`web/apps/admin/src/components/Shell.tsx`:
```tsx
import { useQueryClient } from "@tanstack/react-query";
import { NavLink, Outlet, useNavigate } from "react-router-dom";
import { client } from "../api";
import { navItems } from "../nav";
import { isAdmin, useSession } from "../lib/session";

export function Shell() {
  const { data: me } = useSession();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const items = navItems.filter((item) => !item.adminOnly || isAdmin(me));

  async function signOut() {
    try {
      await client.logout();
    } finally {
      navigate("/login", { replace: true });
      queryClient.clear();
    }
  }

  return (
    <div className="shell">
      <aside className="sidebar">
        <div className="brand">openforms</div>
        <nav aria-label="Main">
          <ul>
            {items.map((item) => (
              <li key={item.to}>
                <NavLink to={item.to} className={({ isActive }) => (isActive ? "nav-link active" : "nav-link")}>
                  {item.label}
                </NavLink>
              </li>
            ))}
          </ul>
        </nav>
        <div className="sidebar-footer">
          <div className="who">
            <strong>{me?.name}</strong>
            {me?.email && <span className="muted">{me.email}</span>}
          </div>
          <button type="button" className="btn" onClick={signOut}>Sign out</button>
        </div>
      </aside>
      <main className="main">
        <Outlet />
      </main>
    </div>
  );
}
```

- [ ] **Step 4: Write the login page, the not-found page and the page stubs**

`web/apps/admin/src/pages/LoginPage.tsx`:
```tsx
import { useState, type FormEvent } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useSearchParams } from "react-router-dom";
import { client } from "../api";
import { qk } from "../queryKeys";
import { errorCode, errorMessage } from "../lib/errors";

export function safeNext(next: string | null): string {
  if (!next || !next.startsWith("/") || next.startsWith("//") || next.startsWith("/\\")) return "/submissions";
  return next;
}

export function LoginPage() {
  const [params] = useSearchParams();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const login = useMutation({
    mutationFn: () => client.login(email.trim(), password),
    onSuccess: () => {
      queryClient.removeQueries({ queryKey: qk.me });
      navigate(safeNext(params.get("next")), { replace: true });
    },
  });

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    login.mutate();
  }

  return (
    <main className="login">
      <form className="card login-card" onSubmit={onSubmit}>
        <h1>Sign in to openforms</h1>
        {login.isError && (
          <p role="alert" className="error">
            {errorCode(login.error) === "invalid_credentials" ? "Email or password is incorrect." : errorMessage(login.error)}
          </p>
        )}
        <div className="field">
          <label htmlFor="login-email">Email</label>
          <input id="login-email" type="email" autoComplete="username" required value={email} onChange={(e) => setEmail(e.target.value)} />
        </div>
        <div className="field">
          <label htmlFor="login-password">Password</label>
          <input id="login-password" type="password" autoComplete="current-password" required value={password} onChange={(e) => setPassword(e.target.value)} />
        </div>
        <button type="submit" className="btn btn-primary" disabled={login.isPending}>
          {login.isPending ? "Signing in…" : "Sign in"}
        </button>
      </form>
    </main>
  );
}
```

`web/apps/admin/src/pages/NotFoundPage.tsx`:
```tsx
import { Link } from "react-router-dom";

export function NotFoundPage() {
  return (
    <div className="page">
      <h1>Page not found</h1>
      <p><Link to="/submissions">Back to the inbox</Link></p>
    </div>
  );
}
```

Page stubs let the route table compile now. Tasks 3–10 each replace their own file wholesale. Create each of these files with exactly this pattern, substituting the component name and heading:

`web/apps/admin/src/pages/InboxPage.tsx`:
```tsx
export function InboxPage() {
  return <div className="page"><h1>Inbox</h1></div>;
}
```
`web/apps/admin/src/pages/SubmissionPage.tsx`:
```tsx
export function SubmissionPage() {
  return <div className="page"><h1>Submission</h1></div>;
}
```
`web/apps/admin/src/pages/FormsPage.tsx`:
```tsx
export function FormsPage() {
  return <div className="page"><h1>Forms</h1></div>;
}
```
`web/apps/admin/src/pages/FormOverviewPage.tsx`:
```tsx
export function FormOverviewPage() {
  return <div className="page"><h1>Form</h1></div>;
}
```
`web/apps/admin/src/pages/WorkflowsPage.tsx`:
```tsx
export function WorkflowsPage() {
  return <div className="page"><h1>Workflows</h1></div>;
}
```
`web/apps/admin/src/pages/WorkflowOverviewPage.tsx`:
```tsx
export function WorkflowOverviewPage() {
  return <div className="page"><h1>Workflow</h1></div>;
}
```
`web/apps/admin/src/pages/UsersPage.tsx`:
```tsx
export function UsersPage() {
  return <div className="page"><h1>Users</h1></div>;
}
```
`web/apps/admin/src/pages/ApiKeysPage.tsx`:
```tsx
export function ApiKeysPage() {
  return <div className="page"><h1>API keys</h1></div>;
}
```
`web/apps/admin/src/pages/JobsPage.tsx`:
```tsx
export function JobsPage() {
  return <div className="page"><h1>Jobs</h1></div>;
}
```

- [ ] **Step 5: Write the route table, App and entry point**

`web/apps/admin/src/routes.tsx`:
```tsx
import type { ReactNode } from "react";
import { ApiKeysPage } from "./pages/ApiKeysPage";
import { FormOverviewPage } from "./pages/FormOverviewPage";
import { FormsPage } from "./pages/FormsPage";
import { InboxPage } from "./pages/InboxPage";
import { JobsPage } from "./pages/JobsPage";
import { SubmissionPage } from "./pages/SubmissionPage";
import { UsersPage } from "./pages/UsersPage";
import { WorkflowOverviewPage } from "./pages/WorkflowOverviewPage";
import { WorkflowsPage } from "./pages/WorkflowsPage";

/** A page inside the authenticated shell. `path` is relative to the /admin basename. */
export type AdminRoute = { path: string; element: ReactNode; adminOnly?: boolean };

/**
 * Extension point: Plan 08 appends editor routes to this array
 * ("forms/new", "forms/:slug/edit", "workflows/new", "workflows/:slug/edit", all adminOnly).
 */
export const routes: AdminRoute[] = [
  { path: "submissions", element: <InboxPage /> },
  { path: "submissions/:id", element: <SubmissionPage /> },
  { path: "forms", element: <FormsPage /> },
  { path: "forms/:slug", element: <FormOverviewPage /> },
  { path: "workflows", element: <WorkflowsPage /> },
  { path: "workflows/:slug", element: <WorkflowOverviewPage /> },
  { path: "users", element: <UsersPage />, adminOnly: true },
  { path: "api-keys", element: <ApiKeysPage />, adminOnly: true },
  { path: "jobs", element: <JobsPage />, adminOnly: true },
];
```

`web/apps/admin/src/App.tsx`:
```tsx
import { useState } from "react";
import { QueryClientProvider } from "@tanstack/react-query";
import { createBrowserRouter, Navigate, RouterProvider, type RouteObject } from "react-router-dom";
import { RequireAdmin } from "./components/RequireAdmin";
import { RequireSession } from "./components/RequireSession";
import { Shell } from "./components/Shell";
import { createQueryClient } from "./lib/queryClient";
import { LoginPage } from "./pages/LoginPage";
import { NotFoundPage } from "./pages/NotFoundPage";
import { routes } from "./routes";

export function buildRoutes(): RouteObject[] {
  return [
    { path: "/login", element: <LoginPage /> },
    {
      path: "/",
      element: (
        <RequireSession>
          <Shell />
        </RequireSession>
      ),
      children: [
        { index: true, element: <Navigate to="/submissions" replace /> },
        ...routes.map((r) => ({
          path: r.path,
          element: r.adminOnly ? <RequireAdmin>{r.element}</RequireAdmin> : r.element,
        })),
        { path: "*", element: <NotFoundPage /> },
      ],
    },
  ];
}

/** Production builds are served at /admin/*; `vite dev` serves the app under its base /_app/admin/. */
export function routerBasename(): string {
  return import.meta.env.DEV ? "/_app/admin" : "/admin";
}

export function App() {
  const [queryClient] = useState(() => createQueryClient());
  const [router] = useState(() => createBrowserRouter(buildRoutes(), { basename: routerBasename() }));
  return (
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  );
}
```

`web/apps/admin/src/main.tsx`:
```tsx
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "./App";
import "./styles/app.css";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
```

Add `renderApp` to `web/apps/admin/src/test/render.tsx`. Append this import at the top with the other imports:
```tsx
import { buildRoutes } from "../App";
```
and append this function at the end of the file:
```tsx
/** Renders the whole admin route table (login + authenticated shell) starting at `route`. */
export function renderApp(route: string) {
  const queryClient = createQueryClient({ test: true });
  const router = createMemoryRouter(buildRoutes(), { initialEntries: [route] });
  const user = userEvent.setup();
  const utils = render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
  return { ...utils, user, router, queryClient };
}
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `pnpm -C web/apps/admin exec vitest run`
Expected: PASS. All suites are green, including 3 LoginPage tests and 4 Shell tests.

Run: `pnpm -C web/apps/admin typecheck`
Expected: exits 0.

- [ ] **Step 7: Commit**

```bash
git add web/apps/admin
git commit -m "feat(admin): session guard, sign-in, shell navigation and route table"
```

---

### Task 3: Inbox

**Parallel group:** A (Lane 1). Touches only `src/pages/InboxPage.tsx`, `src/pages/InboxPage.test.tsx` and `src/lib/catalog.ts`.

**Files:**
- Create: `web/apps/admin/src/lib/catalog.ts`
- Modify (replace stub): `web/apps/admin/src/pages/InboxPage.tsx`
- Test: `web/apps/admin/src/pages/InboxPage.test.tsx`

**Interfaces:**
- Consumes: `client`, `qk`, `SubmissionFilter`, `submissionSummary`, `StateBadge`, `RelativeTime`, `Loading`, `ErrorMessage`, `renderApp`, `server`, `api`, `apiError` and fixtures.
- Produces:
  - `useCatalog(): Catalog`, where `Catalog = { forms: FormSummary[]; formTitle(slug): string; formDef(slug): FormDefinition | undefined; workflowDef(formSlug): WorkflowDefinition | undefined; stateColor(formSlug, state): string | undefined }`.
  - `InboxPage`, which reads and writes URL params `form`, `state` and `assignee` (`""` / `me` / `none`).
  - `INBOX_REFRESH_MS = 30_000`.

- [ ] **Step 1: Write the failing test**

`web/apps/admin/src/pages/InboxPage.test.tsx`:
```tsx
import { describe, expect, it } from "vitest";
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { server } from "../test/server";
import { api, apiError } from "../test/handlers";
import { ids, makePrincipal, makeSubmission } from "../test/fixtures";
import { renderApp } from "../test/render";

describe("InboxPage", () => {
  it("lists submissions with form title, summary link, state color and assignee", async () => {
    server.use(http.get(api("/submissions"), () => HttpResponse.json({
      items: [makeSubmission({ state: "screening", stateLabel: "Screening", assignee: { id: ids.reviewer, name: "Rita Reviewer", email: "reviewer@example.com" } })],
      nextCursor: null,
    })));
    renderApp("/submissions");
    const link = await screen.findByRole("link", { name: "Grace Hopper · grace@example.com" });
    expect(link).toHaveAttribute("href", `/submissions/${ids.sub1}`);
    const row = link.closest("tr")!;
    expect(within(row).getByText("Job application")).toBeInTheDocument();
    expect(within(row).getByText("Rita Reviewer")).toBeInTheDocument();
    await waitFor(() => expect(within(row).getByText("Screening")).toHaveAttribute("data-color", "blue"));
  });

  it("sends form, state and assignee filters and mirrors them in the URL", async () => {
    const seen: URLSearchParams[] = [];
    server.use(http.get(api("/submissions"), ({ request }) => {
      seen.push(new URL(request.url).searchParams);
      return HttpResponse.json({ items: [], nextCursor: null });
    }));
    const { user, router } = renderApp("/submissions");
    await screen.findByText("No submissions match these filters.");
    expect(screen.getByLabelText("State")).toBeDisabled();

    await screen.findByRole("option", { name: "Job application" });
    await user.selectOptions(screen.getByLabelText("Form"), "job-application");
    await screen.findByRole("option", { name: "Screening" });
    await user.selectOptions(screen.getByLabelText("State"), "screening");
    await user.selectOptions(screen.getByLabelText("Assignee"), "none");

    await waitFor(() => {
      const last = seen.at(-1)!;
      expect(last.get("form")).toBe("job-application");
      expect(last.get("state")).toBe("screening");
      expect(last.get("assignee")).toBe("none");
    });
    expect(router.state.location.search).toBe("?form=job-application&state=screening&assignee=none");
  });

  it("clears the state filter when the form changes", async () => {
    const { user, router } = renderApp("/submissions?form=job-application&state=screening");
    await screen.findByRole("option", { name: "Job application" });
    await user.selectOptions(screen.getByLabelText("Form"), "");
    await waitFor(() => expect(router.state.location.search).toBe(""));
  });

  it("loads more pages with the cursor", async () => {
    server.use(http.get(api("/submissions"), ({ request }) => {
      const cursor = new URL(request.url).searchParams.get("cursor");
      return cursor === "c2"
        ? HttpResponse.json({ items: [makeSubmission({ id: ids.sub2, data: { name: "Alan Turing", email: "alan@example.com" } })], nextCursor: null })
        : HttpResponse.json({ items: [makeSubmission()], nextCursor: "c2" });
    }));
    const { user } = renderApp("/submissions");
    await user.click(await screen.findByRole("button", { name: "Load more" }));
    expect(await screen.findByRole("link", { name: "Alan Turing · alan@example.com" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Grace Hopper · grace@example.com" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Load more" })).not.toBeInTheDocument();
  });

  it("renders submissions of unknown forms and color-less states with a gray badge", async () => {
    server.use(http.get(api("/submissions"), () => HttpResponse.json({
      items: [makeSubmission({ form: "contact-us", state: "submitted", stateLabel: "Submitted", terminal: true, data: { topic: "Pricing" } })],
      nextCursor: null,
    })));
    renderApp("/submissions");
    const link = await screen.findByRole("link", { name: "Pricing" });
    const row = link.closest("tr")!;
    expect(within(row).getByText("contact-us")).toBeInTheDocument();
    expect(within(row).getByText("Submitted")).toHaveAttribute("data-color", "gray");
  });

  it("sends the user to login when the session expires mid-use", async () => {
    let meCalls = 0;
    server.use(
      http.get(api("/auth/me"), () =>
        ++meCalls === 1 ? HttpResponse.json({ principal: makePrincipal() }) : apiError(401, "unauthenticated", "Sign in required")),
      http.get(api("/submissions"), () => apiError(401, "unauthenticated", "Sign in required")),
    );
    const { router } = renderApp("/submissions");
    expect(await screen.findByRole("heading", { name: "Sign in to openforms" })).toBeInTheDocument();
    expect(router.state.location.pathname).toBe("/login");
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `pnpm -C web/apps/admin exec vitest run src/pages/InboxPage.test.tsx`
Expected: FAIL. `Unable to find role="link" and name "Grace Hopper · grace@example.com"`, because the stub renders only a heading.

- [ ] **Step 3: Write the catalog hook**

`web/apps/admin/src/lib/catalog.ts`:
```ts
import { useQueries, useQuery } from "@tanstack/react-query";
import type { FormDefinition, FormSummary, WorkflowDefinition } from "@openforms/sdk";
import { client } from "../api";
import { qk } from "../queryKeys";

export type Catalog = {
  forms: FormSummary[];
  formTitle(slug: string): string;
  formDef(slug: string): FormDefinition | undefined;
  workflowDef(formSlug: string): WorkflowDefinition | undefined;
  stateColor(formSlug: string, state: string): string | undefined;
};

/** Current form and workflow definitions, used to label and color inbox rows. Definitions are cached for a minute. */
export function useCatalog(): Catalog {
  const formsQuery = useQuery({ queryKey: qk.forms, queryFn: () => client.listForms() });
  const forms = formsQuery.data ?? [];

  const formQueries = useQueries({
    queries: forms.map((f) => ({ queryKey: qk.form(f.slug), queryFn: () => client.getForm(f.slug), staleTime: 60_000 })),
  });
  const workflowSlugs = [...new Set(forms.map((f) => f.workflow).filter((s): s is string => !!s))];
  const workflowQueries = useQueries({
    queries: workflowSlugs.map((slug) => ({ queryKey: qk.workflow(slug), queryFn: () => client.getWorkflow(slug), staleTime: 60_000 })),
  });

  const formDefs = new Map<string, FormDefinition>();
  for (const q of formQueries) if (q.data) formDefs.set(q.data.slug, q.data.definition);
  const workflowDefs = new Map<string, WorkflowDefinition>();
  for (const q of workflowQueries) if (q.data) workflowDefs.set(q.data.slug, q.data.definition);

  const workflowDef = (formSlug: string) => {
    const slug = forms.find((f) => f.slug === formSlug)?.workflow;
    return slug ? workflowDefs.get(slug) : undefined;
  };

  return {
    forms,
    formTitle: (slug) => forms.find((f) => f.slug === slug)?.title ?? slug,
    formDef: (slug) => formDefs.get(slug),
    workflowDef,
    stateColor: (formSlug, state) => workflowDef(formSlug)?.states.find((s) => s.key === state)?.color,
  };
}
```

- [ ] **Step 4: Write the inbox page**

`web/apps/admin/src/pages/InboxPage.tsx`:
```tsx
import { useMemo } from "react";
import { useInfiniteQuery } from "@tanstack/react-query";
import { Link, useSearchParams } from "react-router-dom";
import { client } from "../api";
import { qk, type SubmissionFilter } from "../queryKeys";
import { useCatalog } from "../lib/catalog";
import { submissionSummary } from "../lib/format";
import { ErrorMessage } from "../components/ErrorMessage";
import { Loading } from "../components/Loading";
import { RelativeTime } from "../components/RelativeTime";
import { StateBadge } from "../components/StateBadge";

export const INBOX_REFRESH_MS = 30_000;
const PAGE_SIZE = 50;

const ASSIGNEE_OPTIONS = [
  { value: "", label: "Anyone" },
  { value: "me", label: "Assigned to me" },
  { value: "none", label: "Unassigned" },
];

export function InboxPage() {
  const catalog = useCatalog();
  const [params, setParams] = useSearchParams();
  const form = params.get("form") ?? "";
  const state = params.get("state") ?? "";
  const assignee = params.get("assignee") ?? "";

  const filter = useMemo(() => {
    const f: SubmissionFilter = {};
    if (form) f.form = form;
    if (state) f.state = state;
    if (assignee) f.assignee = assignee;
    return f;
  }, [form, state, assignee]);

  const query = useInfiniteQuery({
    queryKey: qk.submissions(filter),
    queryFn: ({ pageParam }) => client.listSubmissions({ ...filter, limit: PAGE_SIZE, ...(pageParam ? { cursor: pageParam } : {}) }),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.nextCursor ?? undefined,
    refetchInterval: INBOX_REFRESH_MS,
  });

  function update(key: "form" | "state" | "assignee", value: string) {
    const next = new URLSearchParams(params);
    if (value) next.set(key, value);
    else next.delete(key);
    if (key === "form") next.delete("state");
    setParams(next, { replace: true });
  }

  const states = form ? catalog.workflowDef(form)?.states ?? [] : [];
  const rows = query.data?.pages.flatMap((p) => p.items) ?? [];

  return (
    <div className="page">
      <header className="page-header">
        <h1>Inbox</h1>
      </header>

      <div className="toolbar" role="group" aria-label="Filters">
        <div className="field">
          <label htmlFor="filter-form">Form</label>
          <select id="filter-form" value={form} onChange={(e) => update("form", e.target.value)}>
            <option value="">All forms</option>
            {catalog.forms.map((f) => (
              <option key={f.slug} value={f.slug}>{f.title}</option>
            ))}
          </select>
        </div>
        <div className="field">
          <label htmlFor="filter-state">State</label>
          <select id="filter-state" value={state} disabled={states.length === 0} onChange={(e) => update("state", e.target.value)}>
            <option value="">Any state</option>
            {states.map((s) => (
              <option key={s.key} value={s.key}>{s.label}</option>
            ))}
          </select>
        </div>
        <div className="field">
          <label htmlFor="filter-assignee">Assignee</label>
          <select id="filter-assignee" value={assignee} onChange={(e) => update("assignee", e.target.value)}>
            {ASSIGNEE_OPTIONS.map((o) => (
              <option key={o.value} value={o.value}>{o.label}</option>
            ))}
          </select>
        </div>
      </div>

      {query.isPending ? (
        <Loading />
      ) : query.isError ? (
        <ErrorMessage error={query.error} />
      ) : rows.length === 0 ? (
        <p className="empty">No submissions match these filters.</p>
      ) : (
        <table className="table">
          <thead>
            <tr>
              <th>Received</th>
              <th>Form</th>
              <th>Summary</th>
              <th>State</th>
              <th>Assignee</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((s) => (
              <tr key={s.id}>
                <td><RelativeTime iso={s.createdAt} /></td>
                <td>{catalog.formTitle(s.form)}</td>
                <td>
                  <Link to={`/submissions/${s.id}`}>{submissionSummary(s.data, catalog.formDef(s.form))}</Link>
                </td>
                <td><StateBadge label={s.stateLabel} color={catalog.stateColor(s.form, s.state)} /></td>
                <td>{s.assignee?.name ?? <span className="muted">—</span>}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      {query.hasNextPage && (
        <div>
          <button type="button" className="btn" onClick={() => query.fetchNextPage()} disabled={query.isFetchingNextPage}>
            {query.isFetchingNextPage ? "Loading…" : "Load more"}
          </button>
        </div>
      )}
    </div>
  );
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `pnpm -C web/apps/admin exec vitest run src/pages/InboxPage.test.tsx`
Expected: PASS (6 tests).

Run: `pnpm -C web/apps/admin test && pnpm -C web/apps/admin typecheck`
Expected: all suites pass and the typecheck exits 0.

- [ ] **Step 6: Commit**

```bash
git add web/apps/admin/src/lib/catalog.ts web/apps/admin/src/pages/InboxPage.tsx web/apps/admin/src/pages/InboxPage.test.tsx
git commit -m "feat(admin): inbox with filters, cursor pagination and auto-refresh"
```

---

### Task 4: Submission detail — answers and timeline

**Parallel group:** A (Lane 2, first). Touches only `src/pages/SubmissionPage.tsx`, `src/pages/submission/AnswersPanel.tsx`, `src/pages/submission/Timeline.tsx`, `src/lib/events.ts` and their tests.

**Files:**
- Create: `web/apps/admin/src/lib/events.ts`, `src/pages/submission/AnswersPanel.tsx`, `src/pages/submission/Timeline.tsx`
- Modify (replace stub): `web/apps/admin/src/pages/SubmissionPage.tsx`
- Test: `web/apps/admin/src/lib/events.test.ts`, `web/apps/admin/src/pages/SubmissionPage.test.tsx`

**Interfaces:**
- Consumes: `client.getSubmission(id): Promise<SubmissionDetail>`, `formatValue`, `shortId`, `errorCode`, `StateBadge`, `RelativeTime` and fixtures.
- Produces:
  - `describeEvent(e: SubmissionEvent, wf?: WorkflowDefinition | null): { title: string; body?: string; tone: "default" | "danger" }` and `actorName(e: SubmissionEvent): string`.
  - `AnswersPanel({ form, data })`.
  - `Timeline({ events, workflow })`, which renders `<ol aria-label="Activity">` newest first.
  - `SubmissionPage`, which uses query key `qk.submission(id)`.

- [ ] **Step 1: Write the failing tests**

`web/apps/admin/src/lib/events.test.ts`:
```ts
import { describe, expect, it } from "vitest";
import { makeEvent, makeWorkflowDefinition } from "../test/fixtures";
import { actorName, describeEvent } from "./events";

const wf = makeWorkflowDefinition();
const rita = { type: "user", id: "u1", name: "Rita Reviewer" } as const;

describe("describeEvent", () => {
  it("describes creation by a respondent or a user", () => {
    expect(describeEvent(makeEvent(), wf)).toEqual({ title: "Submitted", tone: "default" });
    expect(describeEvent(makeEvent({ actor: rita }), wf).title).toBe("Created by Rita Reviewer");
  });

  it("describes transitions with state labels and comment", () => {
    const e = makeEvent({ id: 2, type: "transition", fromState: "new", toState: "screening", transition: "screen", actor: rita, payload: { comment: "Looks promising" } });
    expect(describeEvent(e, wf)).toEqual({ title: "Rita Reviewer: Start screening (New → Screening)", body: "Looks promising", tone: "default" });
  });

  it("falls back to raw keys without a workflow", () => {
    const e = makeEvent({ type: "transition", fromState: "a", toState: "b", transition: "go", actor: rita });
    expect(describeEvent(e, null).title).toBe("Rita Reviewer: go (a → b)");
  });

  it("describes field updates, assignment, comments and actions", () => {
    expect(describeEvent(makeEvent({ type: "fields_updated", actor: rita, payload: { fields: { score: 4 } } }), wf).title).toBe("Rita Reviewer updated Score");
    expect(describeEvent(makeEvent({ type: "assigned", actor: { type: "system", id: null, name: "" }, payload: { assigneeId: "u1", assigneeName: "Rita Reviewer" } }), wf).title).toBe("openforms assigned this to Rita Reviewer");
    expect(describeEvent(makeEvent({ type: "assigned", actor: rita, payload: { assigneeId: null } }), wf).title).toBe("Rita Reviewer removed the assignee");
    expect(describeEvent(makeEvent({ type: "comment", actor: rita, payload: { body: "Call on Monday" } }), wf)).toEqual({ title: "Rita Reviewer commented", body: "Call on Monday", tone: "default" });
    expect(describeEvent(makeEvent({ type: "action_succeeded", actor: { type: "system", id: null, name: "" }, payload: { action: { type: "webhook" } } }), wf).title).toBe("Webhook action succeeded");
    expect(describeEvent(makeEvent({ type: "action_failed", actor: { type: "system", id: null, name: "" }, payload: { action: { type: "email" }, error: "SMTP timeout" } }), wf))
      .toEqual({ title: "Email action failed", body: "SMTP timeout", tone: "danger" });
  });

  it("names actors when the event has no name", () => {
    expect(actorName(makeEvent({ actor: { type: "respondent", id: null, name: "" } }))).toBe("Respondent");
    expect(actorName(makeEvent({ actor: { type: "system", id: null, name: "" } }))).toBe("openforms");
    expect(actorName(makeEvent({ actor: { type: "api_key", id: "k", name: "" } }))).toBe("API key");
  });
});
```

`web/apps/admin/src/pages/SubmissionPage.test.tsx`:
```tsx
import { describe, expect, it } from "vitest";
import { screen, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { server } from "../test/server";
import { api, apiError } from "../test/handlers";
import { ids, makeDetail, makeEvent, makeSubmission } from "../test/fixtures";
import { renderApp } from "../test/render";

function answer(label: string): string | null {
  return screen.getByText(label, { selector: "dt" }).nextElementSibling?.textContent ?? null;
}

describe("SubmissionPage", () => {
  it("shows answers with pinned labels, including legacy keys, and a timeline newest first", async () => {
    server.use(http.get(api("/submissions/:id"), () => HttpResponse.json(makeDetail({
      submission: makeSubmission({
        state: "screening", stateLabel: "Screening",
        data: { name: "Grace Hopper", email: "grace@example.com", role: "engineer", relocate: true, legacyField: "kept" },
      }),
      events: [
        makeEvent(),
        makeEvent({ id: 2, type: "transition", fromState: "new", toState: "screening", transition: "screen",
          actor: { type: "user", id: ids.reviewer, name: "Rita Reviewer" }, payload: { comment: "Looks promising" } }),
      ],
    }))));
    renderApp(`/submissions/${ids.sub1}`);

    expect(await screen.findByRole("heading", { level: 1 })).toHaveTextContent("Job application");
    expect(answer("Full name")).toBe("Grace Hopper");
    expect(answer("Role")).toBe("Engineer");
    expect(answer("Willing to relocate")).toBe("Yes");
    expect(answer("legacyField")).toBe("kept");
    expect(screen.queryByText("Portfolio URL", { selector: "dt" })).not.toBeInTheDocument();
    expect(screen.getByText("Screening", { selector: ".badge" })).toHaveAttribute("data-color", "blue");

    const items = within(screen.getByRole("list", { name: "Activity" })).getAllByRole("listitem");
    expect(items[0]).toHaveTextContent("Rita Reviewer: Start screening (New → Screening)");
    expect(items[0]).toHaveTextContent("Looks promising");
    expect(items[1]).toHaveTextContent("Submitted");
  });

  it("shows a not-found state", async () => {
    server.use(http.get(api("/submissions/:id"), () => apiError(404, "not_found", "submission not found")));
    renderApp(`/submissions/${ids.sub2}`);
    expect(await screen.findByRole("heading", { name: "Submission not found" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Back to the inbox" })).toHaveAttribute("href", "/submissions");
  });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `pnpm -C web/apps/admin exec vitest run src/lib/events.test.ts src/pages/SubmissionPage.test.tsx`
Expected: FAIL. `Failed to resolve import "./events"`, and the page test fails because the heading "Submission" does not contain "Job application".

- [ ] **Step 3: Write the event descriptions**

`web/apps/admin/src/lib/events.ts`:
```ts
import type { SubmissionEvent, WorkflowDefinition } from "@openforms/sdk";

export type EventView = { title: string; body?: string; tone: "default" | "danger" };

export function actorName(e: SubmissionEvent): string {
  if (e.actor.name) return e.actor.name;
  switch (e.actor.type) {
    case "respondent": return "Respondent";
    case "system": return "openforms";
    case "api_key": return "API key";
    default: return "Someone";
  }
}

function actionLabel(payload: Record<string, unknown>): string {
  const type = (payload.action as { type?: string } | undefined)?.type;
  return type ? `${type.charAt(0).toUpperCase()}${type.slice(1)} action` : "Action";
}

export function describeEvent(e: SubmissionEvent, wf?: WorkflowDefinition | null): EventView {
  const who = actorName(e);
  const p = (e.payload ?? {}) as Record<string, unknown>;
  const stateLabel = (key?: string | null) => (key ? wf?.states.find((s) => s.key === key)?.label ?? key : "—");

  switch (e.type) {
    case "created":
      return { title: e.actor.type === "respondent" ? "Submitted" : `Created by ${who}`, tone: "default" };
    case "transition": {
      const label = wf?.transitions.find((t) => t.key === e.transition)?.label ?? e.transition ?? "Transition";
      const view: EventView = { title: `${who}: ${label} (${stateLabel(e.fromState)} → ${stateLabel(e.toState)})`, tone: "default" };
      if (typeof p.comment === "string" && p.comment) view.body = p.comment;
      return view;
    }
    case "fields_updated": {
      const keys = Object.keys((p.fields as Record<string, unknown> | undefined) ?? {});
      const labels = keys.map((k) => wf?.fields?.find((f) => f.key === k)?.label ?? k);
      return { title: `${who} updated ${labels.length ? labels.join(", ") : "fields"}`, tone: "default" };
    }
    case "assigned": {
      const name = typeof p.assigneeName === "string" && p.assigneeName ? p.assigneeName : null;
      return { title: name ? `${who} assigned this to ${name}` : `${who} removed the assignee`, tone: "default" };
    }
    case "comment":
      return { title: `${who} commented`, body: typeof p.body === "string" ? p.body : "", tone: "default" };
    case "action_succeeded":
      return { title: `${actionLabel(p)} succeeded`, tone: "default" };
    case "action_failed": {
      const view: EventView = { title: `${actionLabel(p)} failed`, tone: "danger" };
      if (typeof p.error === "string") view.body = p.error;
      return view;
    }
    default:
      return { title: e.type, tone: "default" };
  }
}
```

- [ ] **Step 4: Write the answers panel, the timeline and the page**

`web/apps/admin/src/pages/submission/AnswersPanel.tsx`:
```tsx
import type { FormDefinition } from "@openforms/sdk";
import { formatValue } from "../../lib/format";

export function AnswersPanel({ form, data }: { form: FormDefinition; data: Record<string, unknown> }) {
  const known = new Set(form.fields.map((f) => f.key));
  const rows = [
    ...form.fields.filter((f) => f.key in data).map((f) => ({ key: f.key, label: f.label, value: formatValue(data[f.key], f) })),
    ...Object.keys(data).filter((k) => !known.has(k)).map((k) => ({ key: k, label: k, value: formatValue(data[k]) })),
  ];
  if (rows.length === 0) return <p className="muted">No answers.</p>;
  return (
    <dl className="answers">
      {rows.map((r) => (
        <div key={r.key} className="answer">
          <dt>{r.label}</dt>
          <dd>{r.value}</dd>
        </div>
      ))}
    </dl>
  );
}
```

`web/apps/admin/src/pages/submission/Timeline.tsx`:
```tsx
import type { SubmissionEvent, WorkflowDefinition } from "@openforms/sdk";
import { describeEvent } from "../../lib/events";
import { RelativeTime } from "../../components/RelativeTime";

export function Timeline({ events, workflow }: { events: SubmissionEvent[]; workflow?: WorkflowDefinition | null }) {
  const ordered = [...events].sort((a, b) => b.id - a.id);
  if (ordered.length === 0) return <p className="muted">No activity yet.</p>;
  return (
    <ol className="timeline" aria-label="Activity">
      {ordered.map((e) => {
        const view = describeEvent(e, workflow);
        return (
          <li key={e.id} className={`timeline-item tone-${view.tone}`} data-type={e.type}>
            <div className="timeline-title">{view.title}</div>
            {view.body && <p className="timeline-body">{view.body}</p>}
            <RelativeTime iso={e.createdAt} />
          </li>
        );
      })}
    </ol>
  );
}
```

`web/apps/admin/src/pages/SubmissionPage.tsx`:
```tsx
import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "react-router-dom";
import { client } from "../api";
import { qk } from "../queryKeys";
import { errorCode } from "../lib/errors";
import { shortId } from "../lib/format";
import { ErrorMessage } from "../components/ErrorMessage";
import { Loading } from "../components/Loading";
import { RelativeTime } from "../components/RelativeTime";
import { StateBadge } from "../components/StateBadge";
import { AnswersPanel } from "./submission/AnswersPanel";
import { Timeline } from "./submission/Timeline";

export function SubmissionPage() {
  const { id = "" } = useParams();
  const query = useQuery({ queryKey: qk.submission(id), queryFn: () => client.getSubmission(id) });

  if (query.isPending) return <div className="page"><Loading /></div>;
  if (query.isError) {
    return (
      <div className="page">
        {errorCode(query.error) === "not_found" ? (
          <>
            <h1>Submission not found</h1>
            <p><Link to="/submissions">Back to the inbox</Link></p>
          </>
        ) : (
          <ErrorMessage error={query.error} />
        )}
      </div>
    );
  }

  const { submission, form, workflow, events } = query.data;
  const color = workflow?.states.find((s) => s.key === submission.state)?.color;

  return (
    <div className="page">
      <nav className="breadcrumb" aria-label="Breadcrumb">
        <Link to="/submissions">Inbox</Link> / <span>{form.title}</span>
      </nav>
      <header className="page-header">
        <div>
          <h1>
            {form.title} <span className="muted mono">#{shortId(submission.id)}</span>
          </h1>
          <p className="muted">
            Received <RelativeTime iso={submission.createdAt} /> · form v{submission.formVersion}
          </p>
        </div>
        <StateBadge label={submission.stateLabel} color={color} />
      </header>
      <div className="detail">
        <div className="detail-main">
          <section className="card" aria-label="Answers">
            <h2>Answers</h2>
            <AnswersPanel form={form} data={submission.data} />
          </section>
          <section className="card" aria-label="Activity log">
            <h2>Activity</h2>
            <Timeline events={events} workflow={workflow} />
          </section>
        </div>
        <aside className="detail-side">
          <section className="card">
            <h2>Assignee</h2>
            <p>{submission.assignee?.name ?? <span className="muted">Unassigned</span>}</p>
          </section>
        </aside>
      </div>
    </div>
  );
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `pnpm -C web/apps/admin exec vitest run src/lib/events.test.ts src/pages/SubmissionPage.test.tsx`
Expected: PASS (5 events tests, 2 page tests).

Run: `pnpm -C web/apps/admin typecheck`
Expected: exits 0.

- [ ] **Step 6: Commit**

```bash
git add web/apps/admin/src/lib/events.ts web/apps/admin/src/lib/events.test.ts web/apps/admin/src/pages/SubmissionPage.tsx web/apps/admin/src/pages/SubmissionPage.test.tsx web/apps/admin/src/pages/submission
git commit -m "feat(admin): submission detail with answers and activity timeline"
```

---

### Task 5: Submission detail — transitions, workflow fields, assignee, comments

**Parallel group:** A (Lane 2, after Task 4). Touches only `src/pages/SubmissionPage.tsx` and new files in `src/pages/submission/`.

**Files:**
- Create: `web/apps/admin/src/pages/submission/refresh.ts`, `WorkflowFieldInput.tsx`, `TransitionDialog.tsx`, `TransitionBar.tsx`, `WorkflowFieldsPanel.tsx`, `AssigneePicker.tsx`, `CommentBox.tsx`
- Modify (full replacement below): `web/apps/admin/src/pages/SubmissionPage.tsx`
- Test: `web/apps/admin/src/pages/submission/actions.test.tsx`

**Interfaces:**
- Consumes: `client.transition`, `client.updateFields`, `client.comment`, `client.assign`, `client.listUsers`; `useSession`, `isAdmin`, `problemsByPath`, `errorCode` and `Dialog`.
- Produces:
  - `useRefreshSubmission(id): () => Promise<unknown>`, which invalidates `qk.submission(id)` and every `["submissions", …]` list.
  - `WorkflowFieldInput({ field, value, onChange, error?, idPrefix })` and `isEmpty(v)`.
  - `TransitionBar({ detail })`, `TransitionDialog`, `WorkflowFieldsPanel({ detail })`, `AssigneePicker({ submission })` and `CommentBox({ submissionId })`.
  - Transition requests always send `expectedState` equal to the displayed state.

- [ ] **Step 1: Write the failing test**

`web/apps/admin/src/pages/submission/actions.test.tsx`:
```tsx
import { describe, expect, it } from "vitest";
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import type { SubmissionDetail } from "@openforms/sdk";
import { server } from "../../test/server";
import { api, apiError } from "../../test/handlers";
import { ids, makeDetail, makeReviewerPrincipal, makeSubmission, makeTransition } from "../../test/fixtures";
import { renderApp } from "../../test/render";

type Calls = { gets: number; transitions: unknown[]; fields: unknown[]; comments: unknown[]; assign: unknown[] };

function mockDetail(initial: SubmissionDetail = makeDetail(), opts: { afterTransition?: SubmissionDetail } = {}) {
  let current = initial;
  const calls: Calls = { gets: 0, transitions: [], fields: [], comments: [], assign: [] };
  const updated = (patch: Partial<SubmissionDetail["submission"]>) => {
    current = { ...current, submission: { ...current.submission, ...patch, updatedAt: new Date(Date.now() + calls.gets * 1000).toISOString() } };
    return HttpResponse.json({ submission: current.submission });
  };
  server.use(
    http.get(api("/submissions/:id"), () => {
      calls.gets++;
      return HttpResponse.json(current);
    }),
    http.post(api("/submissions/:id/transitions"), async ({ request }) => {
      calls.transitions.push(await request.json());
      current = opts.afterTransition ?? makeDetail({
        submission: makeSubmission({ state: "screening", stateLabel: "Screening", updatedAt: "2026-09-23T12:00:00Z" }),
        transitions: [],
      });
      return HttpResponse.json({ submission: current.submission });
    }),
    http.patch(api("/submissions/:id/fields"), async ({ request }) => {
      const body = (await request.json()) as { fields: Record<string, unknown> };
      calls.fields.push(body);
      return updated({ fields: { ...current.submission.fields, ...body.fields } });
    }),
    http.post(api("/submissions/:id/comments"), async ({ request }) => {
      calls.comments.push(await request.json());
      return HttpResponse.json({ event: { id: 9, type: "comment", fromState: null, toState: null, transition: null, actor: { type: "user", id: ids.admin, name: "Ada Admin" }, payload: { body: "x" }, createdAt: "2026-09-23T12:00:00Z" } }, { status: 201 });
    }),
    http.put(api("/submissions/:id/assignee"), async ({ request }) => {
      const body = (await request.json()) as { userId: string | null };
      calls.assign.push(body);
      return updated({ assignee: body.userId ? { id: body.userId, name: "Rita Reviewer", email: "reviewer@example.com" } : null });
    }),
  );
  return calls;
}

const path = `/submissions/${ids.sub1}`;

describe("submission actions", () => {
  it("performs a transition without required fields immediately, with expectedState", async () => {
    const calls = mockDetail();
    const { user } = renderApp(path);
    await user.click(await screen.findByRole("button", { name: "Start screening" }));
    await waitFor(() => expect(calls.transitions).toEqual([{ transition: "screen", expectedState: "new" }]));
    expect(await screen.findByText("Screening", { selector: ".badge" })).toBeInTheDocument();
  });

  it("asks for required fields and an optional comment before transitioning", async () => {
    const calls = mockDetail();
    const { user } = renderApp(path);
    await user.click(await screen.findByRole("button", { name: "Reject" }));
    const dialog = await screen.findByRole("dialog", { name: "Reject" });

    await user.click(within(dialog).getByRole("button", { name: "Reject" }));
    expect(within(dialog).getByText("Required")).toBeInTheDocument();
    expect(calls.transitions).toHaveLength(0);

    await user.type(within(dialog).getByLabelText("Rejection reason"), "Not enough experience");
    await user.type(within(dialog).getByLabelText("Comment (optional)"), "Discussed with Sam");
    await user.click(within(dialog).getByRole("button", { name: "Reject" }));

    await waitFor(() => expect(calls.transitions).toEqual([{
      transition: "reject", expectedState: "new",
      fields: { rejectionReason: "Not enough experience" }, comment: "Discussed with Sam",
    }]));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  });

  it("shows server validation problems next to the dialog field", async () => {
    mockDetail();
    server.use(http.post(api("/submissions/:id/transitions"), () =>
      apiError(422, "validation_failed", "Invalid fields", [{ path: "fields.rejectionReason", message: "must be at least 10 characters" }])));
    const { user } = renderApp(path);
    await user.click(await screen.findByRole("button", { name: "Reject" }));
    const dialog = await screen.findByRole("dialog", { name: "Reject" });
    await user.type(within(dialog).getByLabelText("Rejection reason"), "Too short");
    await user.click(within(dialog).getByRole("button", { name: "Reject" }));
    expect(await within(dialog).findByText("must be at least 10 characters")).toBeInTheDocument();
  });

  it("refetches and explains when someone else changed the submission (409)", async () => {
    const calls = mockDetail();
    server.use(http.post(api("/submissions/:id/transitions"), () => apiError(409, "state_conflict", "state changed")));
    const { user } = renderApp(path);
    await user.click(await screen.findByRole("button", { name: "Start screening" }));
    expect(await screen.findByText("This submission changed while you were viewing it.")).toBeInTheDocument();
    await waitFor(() => expect(calls.gets).toBeGreaterThanOrEqual(2));
  });

  it("disables transitions the user may not perform, with an explanation", async () => {
    mockDetail(makeDetail({ transitions: [makeTransition({ key: "hire", label: "Hire", to: "hired", toLabel: "Hired", allowed: false, reason: "role" })] }));
    renderApp(path);
    const button = await screen.findByRole("button", { name: "Hire" });
    expect(button).toBeDisabled();
    expect(button).toHaveAttribute("title", "You don't have a role that can perform this transition.");
  });

  it("saves changed workflow fields with PATCH", async () => {
    const calls = mockDetail();
    const { user } = renderApp(path);
    const save = await screen.findByRole("button", { name: "Save fields" });
    expect(save).toBeDisabled();
    await user.type(screen.getByLabelText("Score"), "4");
    await user.click(save);
    await waitFor(() => expect(calls.fields).toEqual([{ fields: { score: 4 } }]));
  });

  it("adds a comment", async () => {
    const calls = mockDetail();
    const { user } = renderApp(path);
    await user.type(await screen.findByLabelText("Comment"), "Call candidate on Monday");
    await user.click(screen.getByRole("button", { name: "Add comment" }));
    await waitFor(() => expect(calls.comments).toEqual([{ body: "Call candidate on Monday" }]));
    await waitFor(() => expect(screen.getByLabelText("Comment")).toHaveValue(""));
  });

  it("lets admins assign anyone from the user list", async () => {
    const calls = mockDetail();
    const { user } = renderApp(path);
    await screen.findByRole("option", { name: "Rita Reviewer (reviewer@example.com)" });
    await user.selectOptions(screen.getByLabelText("Assign to"), ids.reviewer);
    await waitFor(() => expect(calls.assign).toEqual([{ userId: ids.reviewer }]));
  });

  it("gives reviewers 'Assign to me' without calling the admin-only users endpoint", async () => {
    let usersCalled = false;
    const calls = mockDetail();
    server.use(
      http.get(api("/auth/me"), () => HttpResponse.json({ principal: makeReviewerPrincipal() })),
      http.get(api("/users"), () => {
        usersCalled = true;
        return apiError(403, "forbidden", "admin only");
      }),
    );
    const { user } = renderApp(path);
    await user.click(await screen.findByRole("button", { name: "Assign to me" }));
    await waitFor(() => expect(calls.assign).toEqual([{ userId: ids.reviewer }]));
    expect(await screen.findByRole("button", { name: "Unassign" })).toBeInTheDocument();
    expect(usersCalled).toBe(false);
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `pnpm -C web/apps/admin exec vitest run src/pages/submission/actions.test.tsx`
Expected: FAIL with `Unable to find role="button" and name "Start screening"`.

- [ ] **Step 3: Write the refresh helper and the field input**

`web/apps/admin/src/pages/submission/refresh.ts`:
```ts
import { useQueryClient } from "@tanstack/react-query";
import { qk } from "../../queryKeys";

export function useRefreshSubmission(id: string) {
  const queryClient = useQueryClient();
  return () =>
    Promise.all([
      queryClient.invalidateQueries({ queryKey: qk.submission(id) }),
      queryClient.invalidateQueries({ queryKey: ["submissions"] }),
    ]);
}
```

`web/apps/admin/src/pages/submission/WorkflowFieldInput.tsx`:
```tsx
import type { ReactNode } from "react";

export type WorkflowFieldLike = { key: string; type: string; label: string; options?: { value: string; label: string }[] };

export function isEmpty(v: unknown): boolean {
  return v === undefined || v === null || v === "";
}

const str = (v: unknown) => (v === null || v === undefined ? "" : String(v));

export function WorkflowFieldInput({ field, value, onChange, error, idPrefix }: {
  field: WorkflowFieldLike;
  value: unknown;
  onChange: (v: unknown) => void;
  error?: string;
  idPrefix: string;
}) {
  const id = `${idPrefix}-${field.key}`;
  const errorId = `${id}-error`;
  const common = { id, "aria-invalid": error ? true : undefined, "aria-describedby": error ? errorId : undefined };
  const errorNode = error ? <p id={errorId} className="field-error">{error}</p> : null;

  if (field.type === "checkbox") {
    return (
      <div className="field field-checkbox">
        <label htmlFor={id}>
          <input {...common} type="checkbox" checked={value === true} onChange={(e) => onChange(e.target.checked)} /> {field.label}
        </label>
        {errorNode}
      </div>
    );
  }

  let input: ReactNode;
  switch (field.type) {
    case "textarea":
      input = <textarea {...common} rows={3} value={str(value)} onChange={(e) => onChange(e.target.value)} />;
      break;
    case "number":
      input = (
        <input {...common} type="number" value={str(value)}
          onChange={(e) => onChange(e.target.value === "" ? null : Number(e.target.value))} />
      );
      break;
    case "select":
      input = (
        <select {...common} value={str(value)} onChange={(e) => onChange(e.target.value === "" ? null : e.target.value)}>
          <option value="">—</option>
          {field.options?.map((o) => (
            <option key={o.value} value={o.value}>{o.label}</option>
          ))}
        </select>
      );
      break;
    case "date":
      input = <input {...common} type="date" value={str(value)} onChange={(e) => onChange(e.target.value === "" ? null : e.target.value)} />;
      break;
    default:
      input = <input {...common} type="text" value={str(value)} onChange={(e) => onChange(e.target.value)} />;
  }

  return (
    <div className="field">
      <label htmlFor={id}>{field.label}</label>
      {input}
      {errorNode}
    </div>
  );
}
```

- [ ] **Step 4: Write the transition dialog and bar**

`web/apps/admin/src/pages/submission/TransitionDialog.tsx`:
```tsx
import { useState, type FormEvent } from "react";
import type { AvailableTransition, Submission, WorkflowDefinition } from "@openforms/sdk";
import { Dialog } from "../../components/Dialog";
import { ErrorMessage } from "../../components/ErrorMessage";
import { problemsByPath } from "../../lib/errors";
import { isEmpty, WorkflowFieldInput, type WorkflowFieldLike } from "./WorkflowFieldInput";

export function TransitionDialog({ transition, workflow, submission, pending, serverError, onCancel, onConfirm }: {
  transition: AvailableTransition;
  workflow: WorkflowDefinition;
  submission: Submission;
  pending: boolean;
  serverError: unknown;
  onCancel: () => void;
  onConfirm: (input: { fields: Record<string, unknown>; comment: string }) => void;
}) {
  const required: WorkflowFieldLike[] = transition.requireFields.map(
    (key) => workflow.fields?.find((f) => f.key === key) ?? { key, type: "text", label: key },
  );
  const [values, setValues] = useState<Record<string, unknown>>(() =>
    Object.fromEntries(required.map((f) => [f.key, submission.fields[f.key] ?? null])),
  );
  const [comment, setComment] = useState("");
  const [missing, setMissing] = useState<string[]>([]);
  const problems = problemsByPath(serverError);

  function submit(e: FormEvent) {
    e.preventDefault();
    const miss = required.filter((f) => isEmpty(values[f.key])).map((f) => f.key);
    setMissing(miss);
    if (miss.length) return;
    const fields = Object.fromEntries(Object.entries(values).map(([k, v]) => [k, v === "" ? null : v]));
    onConfirm({ fields, comment: comment.trim() });
  }

  return (
    <Dialog title={transition.label} onClose={onCancel}>
      <form onSubmit={submit} noValidate>
        <p className="muted">Moves this submission to <strong>{transition.toLabel}</strong>.</p>
        {required.map((f) => (
          <WorkflowFieldInput
            key={f.key}
            idPrefix="tr"
            field={f}
            value={values[f.key]}
            onChange={(v) => setValues((s) => ({ ...s, [f.key]: v }))}
            error={missing.includes(f.key) ? "Required" : problems[`fields.${f.key}`]}
          />
        ))}
        <div className="field">
          <label htmlFor="tr-comment">Comment (optional)</label>
          <textarea id="tr-comment" rows={3} value={comment} onChange={(e) => setComment(e.target.value)} />
        </div>
        {serverError && Object.keys(problems).length === 0 ? <ErrorMessage error={serverError} /> : null}
        <div className="dialog-footer">
          <button type="button" className="btn" onClick={onCancel}>Cancel</button>
          <button type="submit" className="btn btn-primary" disabled={pending}>{transition.label}</button>
        </div>
      </form>
    </Dialog>
  );
}
```

`web/apps/admin/src/pages/submission/TransitionBar.tsx`:
```tsx
import { useState } from "react";
import { useMutation } from "@tanstack/react-query";
import type { AvailableTransition, SubmissionDetail } from "@openforms/sdk";
import { client } from "../../api";
import { ErrorMessage } from "../../components/ErrorMessage";
import { errorCode } from "../../lib/errors";
import { useRefreshSubmission } from "./refresh";
import { TransitionDialog } from "./TransitionDialog";

export function reasonText(reason: string): string {
  return reason === "role" ? "You don't have a role that can perform this transition." : "This transition isn't available.";
}

type Vars = { t: AvailableTransition; fields?: Record<string, unknown>; comment?: string };

export function TransitionBar({ detail }: { detail: SubmissionDetail }) {
  const { submission, workflow, transitions } = detail;
  const refresh = useRefreshSubmission(submission.id);
  const [active, setActive] = useState<AvailableTransition | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const mutation = useMutation({
    mutationFn: ({ t, fields, comment }: Vars) =>
      client.transition(submission.id, {
        transition: t.key,
        expectedState: submission.state,
        ...(fields && Object.keys(fields).length ? { fields } : {}),
        ...(comment ? { comment } : {}),
      }),
    onSuccess: async () => {
      setActive(null);
      setNotice(null);
      await refresh();
    },
    onError: async (err) => {
      const code = errorCode(err);
      if (code === "state_conflict" || code === "invalid_state") {
        setActive(null);
        setNotice("This submission changed while you were viewing it.");
        await refresh();
      }
    },
  });

  if (!workflow) return <p className="muted">This form has no workflow.</p>;

  return (
    <div>
      {notice && <p role="alert" className="notice">{notice}</p>}
      {transitions.length === 0 ? (
        <p className="muted">No transitions from {submission.stateLabel}.</p>
      ) : (
        <div className="transition-buttons">
          {transitions.map((t) => (
            <button
              key={t.key}
              type="button"
              className="btn"
              disabled={!t.allowed || mutation.isPending}
              title={t.allowed ? `Move to ${t.toLabel}` : reasonText(t.reason)}
              onClick={() => {
                setNotice(null);
                mutation.reset();
                if (t.requireFields.length) setActive(t);
                else mutation.mutate({ t });
              }}
            >
              {t.label}
            </button>
          ))}
        </div>
      )}
      {mutation.isError && !active && !notice && <ErrorMessage error={mutation.error} />}
      {active && (
        <TransitionDialog
          transition={active}
          workflow={workflow}
          submission={submission}
          pending={mutation.isPending}
          serverError={mutation.error ?? undefined}
          onCancel={() => {
            setActive(null);
            mutation.reset();
          }}
          onConfirm={({ fields, comment }) => mutation.mutate({ t: active, fields, comment })}
        />
      )}
    </div>
  );
}
```

- [ ] **Step 5: Write the fields panel, assignee picker and comment box**

`web/apps/admin/src/pages/submission/WorkflowFieldsPanel.tsx`:
```tsx
import { useState, type FormEvent } from "react";
import { useMutation } from "@tanstack/react-query";
import type { SubmissionDetail } from "@openforms/sdk";
import { client } from "../../api";
import { ErrorMessage } from "../../components/ErrorMessage";
import { problemsByPath } from "../../lib/errors";
import { isEmpty, WorkflowFieldInput } from "./WorkflowFieldInput";
import { useRefreshSubmission } from "./refresh";

const same = (a: unknown, b: unknown) => (isEmpty(a) && isEmpty(b)) || a === b;

/** Re-mount with `key={submission.updatedAt}` so a refetch resets local edits. */
export function WorkflowFieldsPanel({ detail }: { detail: SubmissionDetail }) {
  const { submission, workflow } = detail;
  const defs = workflow?.fields ?? [];
  const refresh = useRefreshSubmission(submission.id);
  const [values, setValues] = useState<Record<string, unknown>>(() => ({ ...submission.fields }));

  const changed: Record<string, unknown> = {};
  for (const f of defs) {
    if (!same(values[f.key], submission.fields[f.key])) changed[f.key] = isEmpty(values[f.key]) ? null : values[f.key];
  }
  const dirty = Object.keys(changed).length > 0;

  const mutation = useMutation({
    mutationFn: () => client.updateFields(submission.id, changed),
    onSuccess: () => refresh(),
  });
  const problems = problemsByPath(mutation.error);

  if (defs.length === 0) return null;

  function submit(e: FormEvent) {
    e.preventDefault();
    if (dirty) mutation.mutate();
  }

  return (
    <section className="card" aria-label="Workflow fields">
      <h2>Workflow fields</h2>
      <form onSubmit={submit}>
        {defs.map((f) => (
          <WorkflowFieldInput
            key={f.key}
            idPrefix="wf"
            field={f}
            value={values[f.key]}
            onChange={(v) => setValues((s) => ({ ...s, [f.key]: v }))}
            error={problems[`fields.${f.key}`]}
          />
        ))}
        {mutation.isError && Object.keys(problems).length === 0 && <ErrorMessage error={mutation.error} />}
        <button type="submit" className="btn btn-primary" disabled={!dirty || mutation.isPending}>
          {mutation.isPending ? "Saving…" : "Save fields"}
        </button>
      </form>
    </section>
  );
}
```

`web/apps/admin/src/pages/submission/AssigneePicker.tsx`:
```tsx
import { useMutation, useQuery } from "@tanstack/react-query";
import type { Submission } from "@openforms/sdk";
import { client } from "../../api";
import { qk } from "../../queryKeys";
import { ErrorMessage } from "../../components/ErrorMessage";
import { isAdmin, useSession } from "../../lib/session";
import { useRefreshSubmission } from "./refresh";

export function AssigneePicker({ submission }: { submission: Submission }) {
  const { data: me } = useSession();
  const admin = isAdmin(me);
  const refresh = useRefreshSubmission(submission.id);
  // GET /users is admin-only; reviewers get self-assignment buttons instead.
  const users = useQuery({ queryKey: qk.users, queryFn: () => client.listUsers(), enabled: admin });
  const mutation = useMutation({
    mutationFn: (userId: string | null) => client.assign(submission.id, userId),
    onSuccess: () => refresh(),
  });
  const current = submission.assignee;

  return (
    <section className="card" aria-label="Assignee">
      <h2>Assignee</h2>
      <p>
        {current ? (
          <>
            <strong>{current.name}</strong> <span className="muted">{current.email}</span>
          </>
        ) : (
          <span className="muted">Unassigned</span>
        )}
      </p>
      {admin && users.data ? (
        <div className="field">
          <label htmlFor="assignee-select">Assign to</label>
          <select id="assignee-select" value={current?.id ?? ""} disabled={mutation.isPending}
            onChange={(e) => mutation.mutate(e.target.value || null)}>
            <option value="">Unassigned</option>
            {users.data.map((u) => (
              <option key={u.id} value={u.id}>{u.name} ({u.email})</option>
            ))}
          </select>
        </div>
      ) : (
        <div className="button-row">
          {me?.kind === "user" && current?.id !== me.id && (
            <button type="button" className="btn" disabled={mutation.isPending} onClick={() => mutation.mutate(me.id)}>
              Assign to me
            </button>
          )}
          {current && (
            <button type="button" className="btn btn-ghost" disabled={mutation.isPending} onClick={() => mutation.mutate(null)}>
              Unassign
            </button>
          )}
        </div>
      )}
      {mutation.isError && <ErrorMessage error={mutation.error} />}
    </section>
  );
}
```

`web/apps/admin/src/pages/submission/CommentBox.tsx`:
```tsx
import { useState, type FormEvent } from "react";
import { useMutation } from "@tanstack/react-query";
import { client } from "../../api";
import { ErrorMessage } from "../../components/ErrorMessage";
import { useRefreshSubmission } from "./refresh";

export function CommentBox({ submissionId }: { submissionId: string }) {
  const [body, setBody] = useState("");
  const refresh = useRefreshSubmission(submissionId);
  const mutation = useMutation({
    mutationFn: () => client.comment(submissionId, body.trim()),
    onSuccess: async () => {
      setBody("");
      await refresh();
    },
  });

  function submit(e: FormEvent) {
    e.preventDefault();
    if (body.trim()) mutation.mutate();
  }

  return (
    <form className="comment-box" onSubmit={submit}>
      <label htmlFor="comment-body" className="sr-only">Comment</label>
      <textarea id="comment-body" rows={3} placeholder="Add a note for your team" value={body} onChange={(e) => setBody(e.target.value)} />
      {mutation.isError && <ErrorMessage error={mutation.error} />}
      <button type="submit" className="btn" disabled={!body.trim() || mutation.isPending}>Add comment</button>
    </form>
  );
}
```

- [ ] **Step 6: Wire the components into the page**

Replace `web/apps/admin/src/pages/SubmissionPage.tsx` entirely with:
```tsx
import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "react-router-dom";
import { client } from "../api";
import { qk } from "../queryKeys";
import { errorCode } from "../lib/errors";
import { shortId } from "../lib/format";
import { ErrorMessage } from "../components/ErrorMessage";
import { Loading } from "../components/Loading";
import { RelativeTime } from "../components/RelativeTime";
import { StateBadge } from "../components/StateBadge";
import { AnswersPanel } from "./submission/AnswersPanel";
import { AssigneePicker } from "./submission/AssigneePicker";
import { CommentBox } from "./submission/CommentBox";
import { Timeline } from "./submission/Timeline";
import { TransitionBar } from "./submission/TransitionBar";
import { WorkflowFieldsPanel } from "./submission/WorkflowFieldsPanel";

export function SubmissionPage() {
  const { id = "" } = useParams();
  const query = useQuery({ queryKey: qk.submission(id), queryFn: () => client.getSubmission(id) });

  if (query.isPending) return <div className="page"><Loading /></div>;
  if (query.isError) {
    return (
      <div className="page">
        {errorCode(query.error) === "not_found" ? (
          <>
            <h1>Submission not found</h1>
            <p><Link to="/submissions">Back to the inbox</Link></p>
          </>
        ) : (
          <ErrorMessage error={query.error} />
        )}
      </div>
    );
  }

  const detail = query.data;
  const { submission, form, workflow, events } = detail;
  const color = workflow?.states.find((s) => s.key === submission.state)?.color;

  return (
    <div className="page">
      <nav className="breadcrumb" aria-label="Breadcrumb">
        <Link to="/submissions">Inbox</Link> / <span>{form.title}</span>
      </nav>
      <header className="page-header">
        <div>
          <h1>
            {form.title} <span className="muted mono">#{shortId(submission.id)}</span>
          </h1>
          <p className="muted">
            Received <RelativeTime iso={submission.createdAt} /> · form v{submission.formVersion}
          </p>
        </div>
        <StateBadge label={submission.stateLabel} color={color} />
      </header>
      <div className="detail">
        <div className="detail-main">
          <section className="card" aria-label="Answers">
            <h2>Answers</h2>
            <AnswersPanel form={form} data={submission.data} />
          </section>
          <section className="card" aria-label="Activity log">
            <h2>Activity</h2>
            <CommentBox submissionId={submission.id} />
            <Timeline events={events} workflow={workflow} />
          </section>
        </div>
        <aside className="detail-side">
          <section className="card" aria-label="Actions">
            <h2>Actions</h2>
            <TransitionBar detail={detail} />
          </section>
          <AssigneePicker submission={submission} />
          <WorkflowFieldsPanel key={submission.updatedAt} detail={detail} />
        </aside>
      </div>
    </div>
  );
}
```

- [ ] **Step 7: Run the tests to verify they pass**

Run: `pnpm -C web/apps/admin exec vitest run src/pages/submission/actions.test.tsx src/pages/SubmissionPage.test.tsx`
Expected: PASS (9 action tests and 2 page tests).

Run: `pnpm -C web/apps/admin test && pnpm -C web/apps/admin typecheck`
Expected: all suites pass and the typecheck exits 0.

- [ ] **Step 8: Commit**

```bash
git add web/apps/admin/src/pages/SubmissionPage.tsx web/apps/admin/src/pages/submission
git commit -m "feat(admin): transitions with required-field dialog, workflow fields, assignee and comments"
```

---

### Task 6: Forms list, form overview and the `OverviewExtras` slot

**Parallel group:** A (Lane 3, first). Touches only `src/pages/FormsPage.tsx`, `src/pages/FormOverviewPage.tsx`, `src/components/VersionTable.tsx`, `src/extensions/OverviewExtras.tsx` and their tests.

**Files:**
- Create: `web/apps/admin/src/extensions/OverviewExtras.tsx`, `web/apps/admin/src/components/VersionTable.tsx`
- Modify (replace stubs): `web/apps/admin/src/pages/FormsPage.tsx`, `web/apps/admin/src/pages/FormOverviewPage.tsx`
- Test: `web/apps/admin/src/pages/FormsPage.test.tsx`

**Interfaces:**
- Consumes: `client.listForms`, `client.getForm`, `client.formVersions`, `client.csvUrl`, `useSession`/`isAdmin`, `qk.forms`, `qk.form(slug)` and `qk.formVersions(slug)`.
- Produces:
  - `export function OverviewExtras(props: { kind: "form" | "workflow"; slug: string }): JSX.Element | null`. It returns `null` in Plan 07. **Plan 08 replaces this file's body** with the version-history panel. It is rendered at the bottom of the form overview (this task) and the workflow overview (Task 7).
  - `VersionTable({ versions })` and `SourceTag({ source })`.
  - Admin-only links: "New form" → `/forms/new` and "Edit" → `/forms/:slug/edit`. Plan 08 provides those routes, so until then they resolve to Page not found.

- [ ] **Step 1: Write the failing test**

`web/apps/admin/src/pages/FormsPage.test.tsx`:
```tsx
import { describe, expect, it } from "vitest";
import { screen, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { server } from "../test/server";
import { api } from "../test/handlers";
import { makeFormDefinition, makeFormRecord, makeFormSummary, makeReviewerPrincipal } from "../test/fixtures";
import { renderApp } from "../test/render";

describe("FormsPage", () => {
  it("lists forms with workflow, visibility, version and submission counts", async () => {
    renderApp("/forms");
    const link = await screen.findByRole("link", { name: "Job application" });
    expect(link).toHaveAttribute("href", "/forms/job-application");
    const row = link.closest("tr")!;
    expect(within(row).getByRole("link", { name: "hiring" })).toHaveAttribute("href", "/workflows/hiring");
    expect(within(row).getByText("v3")).toBeInTheDocument();
    expect(within(row).getByText("12")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "New form" })).toHaveAttribute("href", "/forms/new");
  });

  it("explains how to create the first form", async () => {
    server.use(http.get(api("/forms"), () => HttpResponse.json({ items: [] })));
    renderApp("/forms");
    expect(await screen.findByText(/No forms yet/)).toBeInTheDocument();
  });

  it("hides admin-only actions from reviewers", async () => {
    server.use(http.get(api("/auth/me"), () => HttpResponse.json({ principal: makeReviewerPrincipal() })));
    renderApp("/forms");
    await screen.findByRole("link", { name: "Job application" });
    expect(screen.queryByRole("link", { name: "New form" })).not.toBeInTheDocument();
  });
});

describe("FormOverviewPage", () => {
  it("shows metadata, links, fields and versions", async () => {
    renderApp("/forms/job-application");
    expect(await screen.findByRole("heading", { level: 1, name: "Job application" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "View submissions" })).toHaveAttribute("href", "/submissions?form=job-application");
    expect(screen.getByRole("link", { name: "Open hosted form" })).toHaveAttribute("href", "/f/job-application");
    expect(screen.getByRole("link", { name: "Download CSV" }).getAttribute("href")).toContain("/api/v1/forms/job-application/submissions.csv");
    expect(screen.getByRole("link", { name: "Edit" })).toHaveAttribute("href", "/forms/job-application/edit");

    const fields = screen.getByRole("region", { name: "Fields" });
    expect(within(fields).getByText("Full name")).toBeInTheDocument();
    expect(within(fields).getByText("role = designer")).toBeInTheDocument();

    const versions = screen.getByRole("region", { name: "Versions" });
    expect(await within(versions).findByText("v3")).toBeInTheDocument();
    expect(within(versions).getByText("v2")).toBeInTheDocument();
    expect(within(versions).getByText("Editor")).toBeInTheDocument();
  });

  it("does not link to the hosted page for private forms", async () => {
    server.use(http.get(api("/forms/:slug"), () => HttpResponse.json({
      form: makeFormRecord({ definition: makeFormDefinition({ settings: { public: false } }) }),
    })));
    renderApp("/forms/job-application");
    await screen.findByRole("heading", { level: 1, name: "Job application" });
    expect(screen.queryByRole("link", { name: "Open hosted form" })).not.toBeInTheDocument();
  });

  it("shows not found for unknown forms", async () => {
    renderApp("/forms/missing");
    expect(await screen.findByRole("heading", { name: "Form not found" })).toBeInTheDocument();
  });

  it("keeps the summary list and overview in sync with the summary fixture", () => {
    expect(makeFormSummary().slug).toBe(makeFormRecord().slug);
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `pnpm -C web/apps/admin exec vitest run src/pages/FormsPage.test.tsx`
Expected: FAIL with `Unable to find role="link" and name "Job application"`.

- [ ] **Step 3: Write the extension slot and the version table**

`web/apps/admin/src/extensions/OverviewExtras.tsx`:
```tsx
/**
 * Extension point rendered at the bottom of the form and workflow overview pages.
 * Plan 07 renders nothing; Plan 08 replaces this body with the version-history panel.
 */
export function OverviewExtras(_props: { kind: "form" | "workflow"; slug: string }): JSX.Element | null {
  return null;
}
```

`web/apps/admin/src/components/VersionTable.tsx`:
```tsx
import type { VersionInfo } from "@openforms/sdk";
import { RelativeTime } from "./RelativeTime";

const SOURCE_LABELS: Record<string, string> = { cli: "CLI", ui: "Editor", api: "API", seed: "Seed" };

export function SourceTag({ source }: { source: string }) {
  return <span className="tag" title={`Created via ${source}`}>{SOURCE_LABELS[source] ?? source}</span>;
}

export function VersionTable({ versions }: { versions: VersionInfo[] }) {
  if (versions.length === 0) return <p className="muted">No versions yet.</p>;
  return (
    <table className="table">
      <thead>
        <tr>
          <th>Version</th>
          <th>Source</th>
          <th>By</th>
          <th>Created</th>
          <th>Hash</th>
        </tr>
      </thead>
      <tbody>
        {versions.map((v) => (
          <tr key={v.version}>
            <td>v{v.version}</td>
            <td><SourceTag source={v.source} /></td>
            <td>{v.createdBy || <span className="muted">—</span>}</td>
            <td><RelativeTime iso={v.createdAt} /></td>
            <td className="mono">{v.hash.slice(0, 12)}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
```

- [ ] **Step 4: Write the forms list and overview pages**

`web/apps/admin/src/pages/FormsPage.tsx`:
```tsx
import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { client } from "../api";
import { qk } from "../queryKeys";
import { isAdmin, useSession } from "../lib/session";
import { ErrorMessage } from "../components/ErrorMessage";
import { Loading } from "../components/Loading";
import { RelativeTime } from "../components/RelativeTime";
import { SourceTag } from "../components/VersionTable";

export function FormsPage() {
  const { data: me } = useSession();
  const query = useQuery({ queryKey: qk.forms, queryFn: () => client.listForms() });

  return (
    <div className="page">
      <header className="page-header">
        <h1>Forms</h1>
        {isAdmin(me) && <Link className="btn btn-primary" to="/forms/new">New form</Link>}
      </header>
      {query.isPending ? (
        <Loading />
      ) : query.isError ? (
        <ErrorMessage error={query.error} />
      ) : query.data.length === 0 ? (
        <p className="empty">
          No forms yet. Define one in YAML and run <code>openforms push</code>, or create one here.
        </p>
      ) : (
        <table className="table">
          <thead>
            <tr>
              <th>Title</th>
              <th>Slug</th>
              <th>Workflow</th>
              <th>Public</th>
              <th>Version</th>
              <th>Submissions</th>
              <th>Updated</th>
            </tr>
          </thead>
          <tbody>
            {query.data.map((f) => (
              <tr key={f.slug}>
                <td><Link to={`/forms/${f.slug}`}>{f.title}</Link></td>
                <td className="mono">{f.slug}</td>
                <td>{f.workflow ? <Link to={`/workflows/${f.workflow}`}>{f.workflow}</Link> : <span className="muted">—</span>}</td>
                <td>{f.public ? "Yes" : "No"}</td>
                <td><span>v{f.version}</span> <SourceTag source={f.source} /></td>
                <td>{f.submissionCount}</td>
                <td><RelativeTime iso={f.updatedAt} /></td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
```

`web/apps/admin/src/pages/FormOverviewPage.tsx`:
```tsx
import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "react-router-dom";
import { client } from "../api";
import { qk } from "../queryKeys";
import { errorCode } from "../lib/errors";
import { isAdmin, useSession } from "../lib/session";
import { ErrorMessage } from "../components/ErrorMessage";
import { Loading } from "../components/Loading";
import { RelativeTime } from "../components/RelativeTime";
import { SourceTag, VersionTable } from "../components/VersionTable";
import { OverviewExtras } from "../extensions/OverviewExtras";

type ConditionLike = { field: string; equals?: unknown; notEquals?: unknown; in?: unknown[] };

export function conditionText(c?: ConditionLike | null): string {
  if (!c) return "Always";
  if (c.in !== undefined) return `${c.field} in [${c.in.map(String).join(", ")}]`;
  if (c.notEquals !== undefined) return `${c.field} ≠ ${String(c.notEquals)}`;
  return `${c.field} = ${String(c.equals)}`;
}

export function FormOverviewPage() {
  const { slug = "" } = useParams();
  const { data: me } = useSession();
  const form = useQuery({ queryKey: qk.form(slug), queryFn: () => client.getForm(slug) });
  const versions = useQuery({ queryKey: qk.formVersions(slug), queryFn: () => client.formVersions(slug) });

  if (form.isPending) return <div className="page"><Loading /></div>;
  if (form.isError) {
    return (
      <div className="page">
        {errorCode(form.error) === "not_found" ? (
          <>
            <h1>Form not found</h1>
            <p><Link to="/forms">Back to forms</Link></p>
          </>
        ) : (
          <ErrorMessage error={form.error} />
        )}
      </div>
    );
  }

  const record = form.data;
  const def = record.definition;
  const isPublic = !!def.settings?.public;

  return (
    <div className="page">
      <nav className="breadcrumb" aria-label="Breadcrumb">
        <Link to="/forms">Forms</Link> / <span>{def.title}</span>
      </nav>
      <header className="page-header">
        <div>
          <h1>{def.title}</h1>
          {def.description && <p className="muted">{def.description}</p>}
        </div>
        {isAdmin(me) && <Link className="btn btn-primary" to={`/forms/${slug}/edit`}>Edit</Link>}
      </header>

      <dl className="meta">
        <div><dt>Slug</dt><dd className="mono">{def.slug}</dd></div>
        <div><dt>Version</dt><dd>v{record.version} <SourceTag source={record.source} /></dd></div>
        <div>
          <dt>Workflow</dt>
          <dd>
            {def.workflow ? (
              <>
                <Link to={`/workflows/${def.workflow}`}>{def.workflow}</Link>
                {record.workflowVersion != null && <span className="muted"> (v{record.workflowVersion})</span>}
              </>
            ) : (
              "None"
            )}
          </dd>
        </div>
        <div><dt>Public</dt><dd>{isPublic ? "Yes" : "No"}</dd></div>
        <div><dt>Updated</dt><dd><RelativeTime iso={record.updatedAt} /></dd></div>
      </dl>

      <div className="button-row">
        <Link className="btn" to={`/submissions?form=${encodeURIComponent(slug)}`}>View submissions</Link>
        {isPublic && <a className="btn" href={`/f/${slug}`} target="_blank" rel="noreferrer">Open hosted form</a>}
        <a className="btn" href={client.csvUrl(slug)} download>Download CSV</a>
      </div>

      <section className="card" aria-label="Fields">
        <h2>Fields</h2>
        <table className="table">
          <thead>
            <tr>
              <th>Key</th>
              <th>Label</th>
              <th>Type</th>
              <th>Required</th>
              <th>Shown when</th>
            </tr>
          </thead>
          <tbody>
            {def.fields.map((f) => (
              <tr key={f.key}>
                <td className="mono">{f.key}</td>
                <td>{f.label}</td>
                <td>{f.type}</td>
                <td>{f.required ? "Yes" : "No"}</td>
                <td>{conditionText(f.showIf as ConditionLike | undefined)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>

      <section className="card" aria-label="Versions">
        <h2>Versions</h2>
        {versions.isPending ? <Loading /> : versions.isError ? <ErrorMessage error={versions.error} /> : <VersionTable versions={versions.data} />}
      </section>

      <OverviewExtras kind="form" slug={slug} />
    </div>
  );
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `pnpm -C web/apps/admin exec vitest run src/pages/FormsPage.test.tsx`
Expected: PASS (7 tests).

Run: `pnpm -C web/apps/admin typecheck`
Expected: exits 0.

- [ ] **Step 6: Commit**

```bash
git add web/apps/admin/src/extensions web/apps/admin/src/components/VersionTable.tsx web/apps/admin/src/pages/FormsPage.tsx web/apps/admin/src/pages/FormOverviewPage.tsx web/apps/admin/src/pages/FormsPage.test.tsx
git commit -m "feat(admin): forms list and overview with versions, CSV export and OverviewExtras slot"
```

---

### Task 7: Workflows list and workflow overview

**Parallel group:** A (Lane 3, after Task 6, because it reuses `VersionTable` and `OverviewExtras`). Touches only `src/pages/WorkflowsPage.tsx`, `src/pages/WorkflowOverviewPage.tsx` and their test.

**Files:**
- Modify (replace stubs): `web/apps/admin/src/pages/WorkflowsPage.tsx`, `web/apps/admin/src/pages/WorkflowOverviewPage.tsx`
- Test: `web/apps/admin/src/pages/WorkflowsPage.test.tsx`

**Interfaces:**
- Consumes: `client.listWorkflows`, `client.getWorkflow`, `client.workflowVersions`, `client.listForms`, `VersionTable`, `SourceTag`, `OverviewExtras`, `StateBadge`, `useSession` and `isAdmin`.
- Produces:
  - `WorkflowsPage` and `WorkflowOverviewPage` (route `workflows/:slug`). The overview shows the title, current version, source, states (badges, marked initial/terminal), transitions, workflow fields, on-submit actions, the forms using the workflow, versions and `<OverviewExtras kind="workflow" slug/>`.
  - Admin-only links: "New workflow" → `/workflows/new` and "Edit" → `/workflows/:slug/edit`.
  - `actionText(a): string`.

- [ ] **Step 1: Write the failing test**

`web/apps/admin/src/pages/WorkflowsPage.test.tsx`:
```tsx
import { describe, expect, it } from "vitest";
import { screen, within } from "@testing-library/react";
import { renderApp } from "../test/render";
import { actionText } from "./WorkflowOverviewPage";

describe("WorkflowsPage", () => {
  it("lists workflows with state count and version", async () => {
    renderApp("/workflows");
    const link = await screen.findByRole("link", { name: "Hiring pipeline" });
    expect(link).toHaveAttribute("href", "/workflows/hiring");
    const row = link.closest("tr")!;
    expect(within(row).getByText("5")).toBeInTheDocument();
    expect(within(row).getByText("v2")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "New workflow" })).toHaveAttribute("href", "/workflows/new");
  });
});

describe("WorkflowOverviewPage", () => {
  it("shows states, transitions, fields, actions, forms and versions", async () => {
    renderApp("/workflows/hiring");
    expect(await screen.findByRole("heading", { level: 1, name: "Hiring pipeline" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Edit" })).toHaveAttribute("href", "/workflows/hiring/edit");

    const states = screen.getByRole("region", { name: "States" });
    expect(within(states).getByText("Screening")).toHaveAttribute("data-color", "blue");
    expect(within(states).getByText("New").closest("li")).toHaveTextContent("initial");
    expect(within(states).getByText("Hired").closest("li")).toHaveTextContent("terminal");

    const transitions = screen.getByRole("region", { name: "Transitions" });
    const reject = within(transitions).getByText("Reject").closest("tr")!;
    expect(reject).toHaveTextContent("New, Screening, Interview");
    expect(reject).toHaveTextContent("Rejected");
    expect(reject).toHaveTextContent("reviewer, hiring-manager");
    expect(reject).toHaveTextContent("Rejection reason");
    expect(reject).toHaveTextContent("Email to {{submission.data.email}}");

    expect(within(screen.getByRole("region", { name: "Workflow fields" })).getByText("Score")).toBeInTheDocument();
    expect(within(screen.getByRole("region", { name: "On submit" })).getByText("Assign to role reviewer")).toBeInTheDocument();
    expect(await within(screen.getByRole("region", { name: "Used by forms" })).findByRole("link", { name: "Job application" }))
      .toHaveAttribute("href", "/forms/job-application");
    expect(await within(screen.getByRole("region", { name: "Versions" })).findByText("v2")).toBeInTheDocument();
  });

  it("shows not found for unknown workflows", async () => {
    renderApp("/workflows/missing");
    expect(await screen.findByRole("heading", { name: "Workflow not found" })).toBeInTheDocument();
  });
});

describe("actionText", () => {
  it("describes each action type", () => {
    expect(actionText({ type: "webhook", url: "https://x.test/h" })).toBe("Webhook to https://x.test/h");
    expect(actionText({ type: "email", to: "ops@x.test" })).toBe("Email to ops@x.test");
    expect(actionText({ type: "assign", user: "a@x.test" })).toBe("Assign to a@x.test");
    expect(actionText({ type: "assign", role: "reviewer" })).toBe("Assign to role reviewer");
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `pnpm -C web/apps/admin exec vitest run src/pages/WorkflowsPage.test.tsx`
Expected: FAIL with `SyntaxError: The requested module './WorkflowOverviewPage' does not provide an export named 'actionText'`.

- [ ] **Step 3: Write the workflows list page**

`web/apps/admin/src/pages/WorkflowsPage.tsx`:
```tsx
import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { client } from "../api";
import { qk } from "../queryKeys";
import { isAdmin, useSession } from "../lib/session";
import { ErrorMessage } from "../components/ErrorMessage";
import { Loading } from "../components/Loading";
import { RelativeTime } from "../components/RelativeTime";
import { SourceTag } from "../components/VersionTable";

export function WorkflowsPage() {
  const { data: me } = useSession();
  const query = useQuery({ queryKey: qk.workflows, queryFn: () => client.listWorkflows() });

  return (
    <div className="page">
      <header className="page-header">
        <h1>Workflows</h1>
        {isAdmin(me) && <Link className="btn btn-primary" to="/workflows/new">New workflow</Link>}
      </header>
      {query.isPending ? (
        <Loading />
      ) : query.isError ? (
        <ErrorMessage error={query.error} />
      ) : query.data.length === 0 ? (
        <p className="empty">No workflows yet. Forms without a workflow keep every submission in the “Submitted” state.</p>
      ) : (
        <table className="table">
          <thead>
            <tr>
              <th>Title</th>
              <th>Slug</th>
              <th>States</th>
              <th>Version</th>
              <th>Updated</th>
            </tr>
          </thead>
          <tbody>
            {query.data.map((w) => (
              <tr key={w.slug}>
                <td><Link to={`/workflows/${w.slug}`}>{w.title}</Link></td>
                <td className="mono">{w.slug}</td>
                <td>{w.stateCount}</td>
                <td><span>v{w.version}</span> <SourceTag source={w.source} /></td>
                <td><RelativeTime iso={w.updatedAt} /></td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
```

- [ ] **Step 4: Write the workflow overview page**

`web/apps/admin/src/pages/WorkflowOverviewPage.tsx`:
```tsx
import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "react-router-dom";
import { client } from "../api";
import { qk } from "../queryKeys";
import { errorCode } from "../lib/errors";
import { isAdmin, useSession } from "../lib/session";
import { ErrorMessage } from "../components/ErrorMessage";
import { Loading } from "../components/Loading";
import { RelativeTime } from "../components/RelativeTime";
import { StateBadge } from "../components/StateBadge";
import { SourceTag, VersionTable } from "../components/VersionTable";
import { OverviewExtras } from "../extensions/OverviewExtras";

type ActionLike = { type: string; url?: string; to?: string; user?: string; role?: string };

export function actionText(a: ActionLike): string {
  switch (a.type) {
    case "webhook": return `Webhook to ${a.url ?? ""}`;
    case "email": return `Email to ${a.to ?? ""}`;
    case "assign": return a.user ? `Assign to ${a.user}` : `Assign to role ${a.role ?? ""}`;
    default: return a.type;
  }
}

export function WorkflowOverviewPage() {
  const { slug = "" } = useParams();
  const { data: me } = useSession();
  const workflow = useQuery({ queryKey: qk.workflow(slug), queryFn: () => client.getWorkflow(slug) });
  const versions = useQuery({ queryKey: qk.workflowVersions(slug), queryFn: () => client.workflowVersions(slug) });
  const forms = useQuery({ queryKey: qk.forms, queryFn: () => client.listForms() });

  if (workflow.isPending) return <div className="page"><Loading /></div>;
  if (workflow.isError) {
    return (
      <div className="page">
        {errorCode(workflow.error) === "not_found" ? (
          <>
            <h1>Workflow not found</h1>
            <p><Link to="/workflows">Back to workflows</Link></p>
          </>
        ) : (
          <ErrorMessage error={workflow.error} />
        )}
      </div>
    );
  }

  const record = workflow.data;
  const def = record.definition;
  const stateLabel = (key: string) => def.states.find((s) => s.key === key)?.label ?? key;
  const fieldLabel = (key: string) => def.fields?.find((f) => f.key === key)?.label ?? key;
  const usedBy = (forms.data ?? []).filter((f) => f.workflow === slug);

  return (
    <div className="page">
      <nav className="breadcrumb" aria-label="Breadcrumb">
        <Link to="/workflows">Workflows</Link> / <span>{def.title}</span>
      </nav>
      <header className="page-header">
        <h1>{def.title}</h1>
        {isAdmin(me) && <Link className="btn btn-primary" to={`/workflows/${slug}/edit`}>Edit</Link>}
      </header>

      <dl className="meta">
        <div><dt>Slug</dt><dd className="mono">{def.slug}</dd></div>
        <div><dt>Version</dt><dd>v{record.version} <SourceTag source={record.source} /></dd></div>
        <div><dt>Updated</dt><dd><RelativeTime iso={record.updatedAt} /></dd></div>
      </dl>

      <section className="card" aria-label="States">
        <h2>States</h2>
        <ul className="button-row" style={{ listStyle: "none", margin: 0, padding: 0 }}>
          {def.states.map((s) => (
            <li key={s.key}>
              <StateBadge label={s.label} color={s.color} />{" "}
              {s.key === def.initial && <span className="muted">initial</span>}
              {s.terminal && <span className="muted">terminal</span>}
            </li>
          ))}
        </ul>
      </section>

      <section className="card" aria-label="Transitions">
        <h2>Transitions</h2>
        <table className="table">
          <thead>
            <tr>
              <th>Transition</th>
              <th>From</th>
              <th>To</th>
              <th>Roles</th>
              <th>Required fields</th>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            {def.transitions.map((t) => (
              <tr key={t.key}>
                <td>{t.label}</td>
                <td>{t.from.map(stateLabel).join(", ")}</td>
                <td>{stateLabel(t.to)}</td>
                <td>{t.guard?.roles?.length ? t.guard.roles.join(", ") : <span className="muted">Anyone</span>}</td>
                <td>{t.guard?.requireFields?.length ? t.guard.requireFields.map(fieldLabel).join(", ") : <span className="muted">—</span>}</td>
                <td>{t.actions?.length ? t.actions.map((a) => actionText(a as ActionLike)).join("; ") : <span className="muted">—</span>}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>

      <section className="card" aria-label="Workflow fields">
        <h2>Workflow fields</h2>
        {def.fields?.length ? (
          <ul>
            {def.fields.map((f) => (
              <li key={f.key}><span>{f.label}</span> <span className="muted mono">{f.key} · {f.type}</span></li>
            ))}
          </ul>
        ) : (
          <p className="muted">None.</p>
        )}
      </section>

      <section className="card" aria-label="On submit">
        <h2>On submit</h2>
        {def.onSubmit?.length ? (
          <ul>
            {def.onSubmit.map((a, i) => <li key={i}>{actionText(a as ActionLike)}</li>)}
          </ul>
        ) : (
          <p className="muted">No actions.</p>
        )}
      </section>

      <section className="card" aria-label="Used by forms">
        <h2>Used by forms</h2>
        {forms.isPending ? (
          <Loading />
        ) : usedBy.length ? (
          <ul>
            {usedBy.map((f) => <li key={f.slug}><Link to={`/forms/${f.slug}`}>{f.title}</Link></li>)}
          </ul>
        ) : (
          <p className="muted">No forms use this workflow yet.</p>
        )}
      </section>

      <section className="card" aria-label="Versions">
        <h2>Versions</h2>
        {versions.isPending ? <Loading /> : versions.isError ? <ErrorMessage error={versions.error} /> : <VersionTable versions={versions.data} />}
      </section>

      <OverviewExtras kind="workflow" slug={slug} />
    </div>
  );
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `pnpm -C web/apps/admin exec vitest run src/pages/WorkflowsPage.test.tsx`
Expected: PASS (4 tests).

Run: `pnpm -C web/apps/admin typecheck`
Expected: exits 0.

- [ ] **Step 6: Commit**

```bash
git add web/apps/admin/src/pages/WorkflowsPage.tsx web/apps/admin/src/pages/WorkflowOverviewPage.tsx web/apps/admin/src/pages/WorkflowsPage.test.tsx
git commit -m "feat(admin): workflows list and workflow overview"
```

---

### Task 8: Users (admin)

**Parallel group:** A (Lane 4). Touches only `src/pages/UsersPage.tsx`, `src/pages/users/UserDialogs.tsx` and the test.

**Files:**
- Create: `web/apps/admin/src/pages/users/UserDialogs.tsx`
- Modify (replace stub): `web/apps/admin/src/pages/UsersPage.tsx`
- Test: `web/apps/admin/src/pages/UsersPage.test.tsx`

**Interfaces:**
- Consumes: `client.listUsers`, `client.createUser`, `client.updateUser`, `client.deleteUser`, `parseRoles`, `formatRoles`, `RoleTags`, `Dialog`, `useSession` and `qk.users`.
- Produces: `UsersPage`, `UserDialog({ user?, onClose })` and `DeleteUserDialog({ user, onClose })`. Edits send only the changed properties.

- [ ] **Step 1: Write the failing test**

`web/apps/admin/src/pages/UsersPage.test.tsx`:
```tsx
import { describe, expect, it } from "vitest";
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { server } from "../test/server";
import { api, apiError } from "../test/handlers";
import { ids, makeUser } from "../test/fixtures";
import { renderApp } from "../test/render";

function capture() {
  const calls = { create: [] as unknown[], update: [] as unknown[], del: [] as string[] };
  server.use(
    http.post(api("/users"), async ({ request }) => {
      calls.create.push(await request.json());
      return HttpResponse.json({ user: makeUser({ id: ids.manager, email: "sam@example.com", name: "Sam Manager", roles: ["hiring-manager"] }) }, { status: 201 });
    }),
    http.patch(api("/users/:id"), async ({ request, params }) => {
      calls.update.push({ id: params.id, body: await request.json() });
      return HttpResponse.json({ user: makeUser() });
    }),
    http.delete(api("/users/:id"), ({ params }) => {
      calls.del.push(String(params.id));
      return new HttpResponse(null, { status: 204 });
    }),
  );
  return calls;
}

describe("UsersPage", () => {
  it("lists users with roles", async () => {
    renderApp("/users");
    const row = (await screen.findByText("Rita Reviewer")).closest("tr")!;
    expect(within(row).getByText("reviewer@example.com")).toBeInTheDocument();
    expect(within(row).getByText("reviewer")).toHaveClass("tag");
  });

  it("creates a user with parsed roles", async () => {
    const calls = capture();
    const { user } = renderApp("/users");
    await user.click(await screen.findByRole("button", { name: "Add user" }));
    const dialog = screen.getByRole("dialog", { name: "Add user" });
    await user.type(within(dialog).getByLabelText("Name"), "Sam Manager");
    await user.type(within(dialog).getByLabelText("Email"), "sam@example.com");
    await user.type(within(dialog).getByLabelText("Password"), "s3cret-pass");
    await user.type(within(dialog).getByLabelText("Roles"), "hiring-manager, reviewer");
    await user.click(within(dialog).getByRole("button", { name: "Add user" }));
    await waitFor(() => expect(calls.create).toEqual([{ email: "sam@example.com", name: "Sam Manager", password: "s3cret-pass", roles: ["hiring-manager", "reviewer"] }]));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  });

  it("explains a duplicate email", async () => {
    server.use(http.post(api("/users"), () => apiError(409, "email_taken", "email already in use")));
    const { user } = renderApp("/users");
    await user.click(await screen.findByRole("button", { name: "Add user" }));
    const dialog = screen.getByRole("dialog", { name: "Add user" });
    await user.type(within(dialog).getByLabelText("Name"), "Rita Again");
    await user.type(within(dialog).getByLabelText("Email"), "reviewer@example.com");
    await user.type(within(dialog).getByLabelText("Password"), "s3cret-pass");
    await user.click(within(dialog).getByRole("button", { name: "Add user" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent("A user with this email already exists.");
  });

  it("edits only changed properties", async () => {
    const calls = capture();
    const { user } = renderApp("/users");
    const row = (await screen.findByText("Rita Reviewer")).closest("tr")!;
    await user.click(within(row).getByRole("button", { name: "Edit" }));
    const dialog = screen.getByRole("dialog", { name: "Edit Rita Reviewer" });
    const roles = within(dialog).getByLabelText("Roles");
    await user.clear(roles);
    await user.type(roles, "reviewer, hiring-manager");
    await user.click(within(dialog).getByRole("button", { name: "Save" }));
    await waitFor(() => expect(calls.update).toEqual([{ id: ids.reviewer, body: { roles: ["reviewer", "hiring-manager"] } }]));
  });

  it("deletes a user after confirmation but not yourself", async () => {
    const calls = capture();
    const { user } = renderApp("/users");
    const selfRow = (await screen.findByText("Ada Admin", { selector: "td" })).closest("tr")!;
    expect(within(selfRow).getByRole("button", { name: "Delete" })).toBeDisabled();

    const row = screen.getByText("Rita Reviewer").closest("tr")!;
    await user.click(within(row).getByRole("button", { name: "Delete" }));
    const dialog = screen.getByRole("dialog", { name: "Delete Rita Reviewer?" });
    await user.click(within(dialog).getByRole("button", { name: "Delete user" }));
    await waitFor(() => expect(calls.del).toEqual([ids.reviewer]));
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `pnpm -C web/apps/admin exec vitest run src/pages/UsersPage.test.tsx`
Expected: FAIL with `Unable to find an element with the text: Rita Reviewer`.

- [ ] **Step 3: Write the user dialogs**

`web/apps/admin/src/pages/users/UserDialogs.tsx`:
```tsx
import { useState, type FormEvent } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import type { User } from "@openforms/sdk";
import { client } from "../../api";
import { qk } from "../../queryKeys";
import { Dialog } from "../../components/Dialog";
import { errorCode, errorMessage, problemsByPath } from "../../lib/errors";
import { formatRoles, parseRoles } from "../../lib/roles";

export function UserDialog({ user, onClose }: { user?: User; onClose: () => void }) {
  const queryClient = useQueryClient();
  const [name, setName] = useState(user?.name ?? "");
  const [email, setEmail] = useState(user?.email ?? "");
  const [password, setPassword] = useState("");
  const [roles, setRoles] = useState(formatRoles(user?.roles ?? []));

  const mutation = useMutation({
    mutationFn: async () => {
      if (!user) {
        return client.createUser({ email: email.trim(), name: name.trim(), password, roles: parseRoles(roles) });
      }
      const input: { name?: string; password?: string; roles?: string[] } = {};
      if (name.trim() !== user.name) input.name = name.trim();
      if (password) input.password = password;
      const nextRoles = parseRoles(roles);
      if (nextRoles.join(",") !== user.roles.join(",")) input.roles = nextRoles;
      return client.updateUser(user.id, input);
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: qk.users });
      onClose();
    },
  });

  const problems = problemsByPath(mutation.error);
  const alertText = !mutation.isError
    ? null
    : errorCode(mutation.error) === "email_taken"
      ? "A user with this email already exists."
      : Object.keys(problems).length === 0
        ? errorMessage(mutation.error)
        : null;

  function submit(e: FormEvent) {
    e.preventDefault();
    mutation.mutate();
  }

  return (
    <Dialog title={user ? `Edit ${user.name}` : "Add user"} onClose={onClose}>
      <form onSubmit={submit}>
        <div className="field">
          <label htmlFor="user-name">Name</label>
          <input id="user-name" required value={name} onChange={(e) => setName(e.target.value)} />
        </div>
        {!user && (
          <div className="field">
            <label htmlFor="user-email">Email</label>
            <input id="user-email" type="email" required value={email} onChange={(e) => setEmail(e.target.value)} />
          </div>
        )}
        <div className="field">
          <label htmlFor="user-password">{user ? "New password" : "Password"}</label>
          <input id="user-password" type="password" autoComplete="new-password" minLength={8} required={!user}
            value={password} onChange={(e) => setPassword(e.target.value)} aria-describedby="user-password-help" />
          <p id="user-password-help" className="field-help">
            {user ? "Leave blank to keep the current password." : "At least 8 characters."}
          </p>
        </div>
        <div className="field">
          <label htmlFor="user-roles">Roles</label>
          <input id="user-roles" value={roles} onChange={(e) => setRoles(e.target.value)} aria-describedby="user-roles-help" />
          <p id="user-roles-help" className="field-help">
            Comma-separated, e.g. <code>reviewer, hiring-manager</code>. <code>admin</code> can do everything.
          </p>
        </div>
        {Object.entries(problems).map(([path, message]) => (
          <p key={path} className="field-error">{message}</p>
        ))}
        {alertText && <p role="alert" className="error">{alertText}</p>}
        <div className="dialog-footer">
          <button type="button" className="btn" onClick={onClose}>Cancel</button>
          <button type="submit" className="btn btn-primary" disabled={mutation.isPending}>{user ? "Save" : "Add user"}</button>
        </div>
      </form>
    </Dialog>
  );
}

export function DeleteUserDialog({ user, onClose }: { user: User; onClose: () => void }) {
  const queryClient = useQueryClient();
  const mutation = useMutation({
    mutationFn: () => client.deleteUser(user.id),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: qk.users });
      onClose();
    },
  });
  return (
    <Dialog title={`Delete ${user.name}?`} onClose={onClose}>
      <p>They lose access immediately. Submissions assigned to them become unassigned; their history stays.</p>
      {mutation.isError && <p role="alert" className="error">{errorMessage(mutation.error)}</p>}
      <div className="dialog-footer">
        <button type="button" className="btn" onClick={onClose}>Cancel</button>
        <button type="button" className="btn btn-danger" disabled={mutation.isPending} onClick={() => mutation.mutate()}>Delete user</button>
      </div>
    </Dialog>
  );
}
```

- [ ] **Step 4: Write the users page**

`web/apps/admin/src/pages/UsersPage.tsx`:
```tsx
import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import type { User } from "@openforms/sdk";
import { client } from "../api";
import { qk } from "../queryKeys";
import { useSession } from "../lib/session";
import { ErrorMessage } from "../components/ErrorMessage";
import { Loading } from "../components/Loading";
import { RelativeTime } from "../components/RelativeTime";
import { RoleTags } from "../components/RoleTags";
import { DeleteUserDialog, UserDialog } from "./users/UserDialogs";

type Open = { kind: "create" } | { kind: "edit"; user: User } | { kind: "delete"; user: User } | null;

export function UsersPage() {
  const { data: me } = useSession();
  const query = useQuery({ queryKey: qk.users, queryFn: () => client.listUsers() });
  const [open, setOpen] = useState<Open>(null);
  const close = () => setOpen(null);

  return (
    <div className="page">
      <header className="page-header">
        <h1>Users</h1>
        <button type="button" className="btn btn-primary" onClick={() => setOpen({ kind: "create" })}>Add user</button>
      </header>
      {query.isPending ? (
        <Loading />
      ) : query.isError ? (
        <ErrorMessage error={query.error} />
      ) : (
        <table className="table">
          <thead>
            <tr>
              <th>Name</th>
              <th>Email</th>
              <th>Roles</th>
              <th>Created</th>
              <th><span className="sr-only">Actions</span></th>
            </tr>
          </thead>
          <tbody>
            {query.data.map((u) => (
              <tr key={u.id}>
                <td>{u.name}</td>
                <td>{u.email}</td>
                <td><RoleTags roles={u.roles} /></td>
                <td><RelativeTime iso={u.createdAt} /></td>
                <td>
                  <div className="button-row">
                    <button type="button" className="btn" onClick={() => setOpen({ kind: "edit", user: u })}>Edit</button>
                    <button type="button" className="btn" disabled={me?.id === u.id}
                      title={me?.id === u.id ? "You can't delete your own account." : undefined}
                      onClick={() => setOpen({ kind: "delete", user: u })}>
                      Delete
                    </button>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {open?.kind === "create" && <UserDialog onClose={close} />}
      {open?.kind === "edit" && <UserDialog user={open.user} onClose={close} />}
      {open?.kind === "delete" && <DeleteUserDialog user={open.user} onClose={close} />}
    </div>
  );
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `pnpm -C web/apps/admin exec vitest run src/pages/UsersPage.test.tsx`
Expected: PASS (5 tests).

Run: `pnpm -C web/apps/admin typecheck`
Expected: exits 0.

- [ ] **Step 6: Commit**

```bash
git add web/apps/admin/src/pages/UsersPage.tsx web/apps/admin/src/pages/UsersPage.test.tsx web/apps/admin/src/pages/users
git commit -m "feat(admin): users management page"
```

---

### Task 9: API keys (admin)

**Parallel group:** A (Lane 5). Touches only `src/pages/ApiKeysPage.tsx`, `src/pages/api-keys/KeyDialogs.tsx` and the test.

**Files:**
- Create: `web/apps/admin/src/pages/api-keys/KeyDialogs.tsx`
- Modify (replace stub): `web/apps/admin/src/pages/ApiKeysPage.tsx`
- Test: `web/apps/admin/src/pages/ApiKeysPage.test.tsx`

**Interfaces:**
- Consumes: `client.listApiKeys`, `client.createApiKey`, `client.revokeApiKey`, `parseRoles`, `RoleTags`, `CopyButton`, `Dialog` and `qk.apiKeys`.
- Produces: `ApiKeysPage`, `CreateKeyDialog({ onCreated(key: string), onClose })`, `ShowKeyDialog({ plaintext, onClose })` and `RevokeKeyDialog({ apiKey, onClose })`. The plaintext key is held only in component state and dropped when the dialog closes.

- [ ] **Step 1: Write the failing test**

`web/apps/admin/src/pages/ApiKeysPage.test.tsx`:
```tsx
import { describe, expect, it } from "vitest";
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { server } from "../test/server";
import { api } from "../test/handlers";
import { ids, makeApiKey } from "../test/fixtures";
import { renderApp } from "../test/render";

const PLAINTEXT = "ofk_Zx9QwErTy1234567890abcdefghijklmnopqrst";

function keysApi() {
  const calls = { created: [] as unknown[], revoked: [] as string[] };
  server.use(
    http.get(api("/api-keys"), () => HttpResponse.json({ items: [
      makeApiKey(),
      makeApiKey({ id: ids.key2, name: "Old integration", prefix: "ofk_Old1", roles: ["reviewer"], revokedAt: "2026-09-15T10:00:00Z" }),
    ] })),
    http.post(api("/api-keys"), async ({ request }) => {
      calls.created.push(await request.json());
      return HttpResponse.json({ apiKey: makeApiKey({ id: "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee", name: "Zapier", prefix: "ofk_Zx9Q" }), key: PLAINTEXT }, { status: 201 });
    }),
    http.delete(api("/api-keys/:id"), ({ params }) => {
      calls.revoked.push(String(params.id));
      return new HttpResponse(null, { status: 204 });
    }),
  );
  return calls;
}

describe("ApiKeysPage", () => {
  it("lists active and revoked keys", async () => {
    keysApi();
    renderApp("/api-keys");
    const active = (await screen.findByText("CI deploy")).closest("tr")!;
    expect(within(active).getByText("Active")).toBeInTheDocument();
    expect(within(active).getByText("ofk_Ab12…")).toBeInTheDocument();
    const revoked = screen.getByText("Old integration").closest("tr")!;
    expect(within(revoked).getByText("Revoked")).toBeInTheDocument();
    expect(within(revoked).queryByRole("button", { name: "Revoke" })).not.toBeInTheDocument();
  });

  it("creates a key and shows the plaintext exactly once", async () => {
    const calls = keysApi();
    const { user } = renderApp("/api-keys");
    await user.click(await screen.findByRole("button", { name: "Create API key" }));
    const create = screen.getByRole("dialog", { name: "Create API key" });
    await user.type(within(create).getByLabelText("Name"), "Zapier");
    await user.type(within(create).getByLabelText("Roles"), "reviewer");
    await user.click(within(create).getByRole("button", { name: "Create key" }));

    await waitFor(() => expect(calls.created).toEqual([{ name: "Zapier", roles: ["reviewer"] }]));
    const show = await screen.findByRole("dialog", { name: "Copy your API key" });
    expect(within(show).getByDisplayValue(PLAINTEXT)).toBeInTheDocument();
    expect(within(show).getByText(/won't be able to see it again/)).toBeInTheDocument();
    await user.click(within(show).getByRole("button", { name: "Copy" }));
    expect(await within(show).findByRole("button", { name: "Copied" })).toBeInTheDocument();

    await user.click(within(show).getByRole("button", { name: "Done" }));
    expect(screen.queryByDisplayValue(PLAINTEXT)).not.toBeInTheDocument();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("revokes a key after confirmation", async () => {
    const calls = keysApi();
    const { user } = renderApp("/api-keys");
    const row = (await screen.findByText("CI deploy")).closest("tr")!;
    await user.click(within(row).getByRole("button", { name: "Revoke" }));
    const dialog = screen.getByRole("dialog", { name: "Revoke CI deploy?" });
    await user.click(within(dialog).getByRole("button", { name: "Revoke key" }));
    await waitFor(() => expect(calls.revoked).toEqual([ids.key1]));
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `pnpm -C web/apps/admin exec vitest run src/pages/ApiKeysPage.test.tsx`
Expected: FAIL with `Unable to find an element with the text: CI deploy`.

- [ ] **Step 3: Write the key dialogs**

`web/apps/admin/src/pages/api-keys/KeyDialogs.tsx`:
```tsx
import { useState, type FormEvent } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import type { ApiKey } from "@openforms/sdk";
import { client } from "../../api";
import { qk } from "../../queryKeys";
import { CopyButton } from "../../components/CopyButton";
import { Dialog } from "../../components/Dialog";
import { ErrorMessage } from "../../components/ErrorMessage";
import { parseRoles } from "../../lib/roles";

export function CreateKeyDialog({ onCreated, onClose }: { onCreated: (plaintext: string) => void; onClose: () => void }) {
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const [roles, setRoles] = useState("");
  const mutation = useMutation({
    mutationFn: () => client.createApiKey({ name: name.trim(), roles: parseRoles(roles) }),
    onSuccess: async (res) => {
      await queryClient.invalidateQueries({ queryKey: qk.apiKeys });
      onCreated(res.key);
    },
  });

  function submit(e: FormEvent) {
    e.preventDefault();
    mutation.mutate();
  }

  return (
    <Dialog title="Create API key" onClose={onClose}>
      <form onSubmit={submit}>
        <div className="field">
          <label htmlFor="key-name">Name</label>
          <input id="key-name" required value={name} onChange={(e) => setName(e.target.value)} />
        </div>
        <div className="field">
          <label htmlFor="key-roles">Roles</label>
          <input id="key-roles" value={roles} onChange={(e) => setRoles(e.target.value)} aria-describedby="key-roles-help" />
          <p id="key-roles-help" className="field-help">
            Comma-separated. Use <code>admin</code> for <code>openforms push</code>; workflow roles for integrations that move submissions.
          </p>
        </div>
        {mutation.isError && <ErrorMessage error={mutation.error} />}
        <div className="dialog-footer">
          <button type="button" className="btn" onClick={onClose}>Cancel</button>
          <button type="submit" className="btn btn-primary" disabled={mutation.isPending}>Create key</button>
        </div>
      </form>
    </Dialog>
  );
}

export function ShowKeyDialog({ plaintext, onClose }: { plaintext: string; onClose: () => void }) {
  return (
    <Dialog title="Copy your API key" onClose={onClose}>
      <p className="notice">Store it somewhere safe now: you won't be able to see it again.</p>
      <div className="field">
        <label htmlFor="key-plaintext">API key</label>
        <input id="key-plaintext" className="key-display" readOnly value={plaintext} onFocus={(e) => e.currentTarget.select()} />
      </div>
      <div className="dialog-footer">
        <CopyButton text={plaintext} />
        <button type="button" className="btn btn-primary" onClick={onClose}>Done</button>
      </div>
    </Dialog>
  );
}

export function RevokeKeyDialog({ apiKey, onClose }: { apiKey: ApiKey; onClose: () => void }) {
  const queryClient = useQueryClient();
  const mutation = useMutation({
    mutationFn: () => client.revokeApiKey(apiKey.id),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: qk.apiKeys });
      onClose();
    },
  });
  return (
    <Dialog title={`Revoke ${apiKey.name}?`} onClose={onClose}>
      <p>Requests using this key will be rejected immediately. This cannot be undone.</p>
      {mutation.isError && <ErrorMessage error={mutation.error} />}
      <div className="dialog-footer">
        <button type="button" className="btn" onClick={onClose}>Cancel</button>
        <button type="button" className="btn btn-danger" disabled={mutation.isPending} onClick={() => mutation.mutate()}>Revoke key</button>
      </div>
    </Dialog>
  );
}
```

- [ ] **Step 4: Write the API keys page**

`web/apps/admin/src/pages/ApiKeysPage.tsx`:
```tsx
import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import type { ApiKey } from "@openforms/sdk";
import { client } from "../api";
import { qk } from "../queryKeys";
import { ErrorMessage } from "../components/ErrorMessage";
import { Loading } from "../components/Loading";
import { RelativeTime } from "../components/RelativeTime";
import { RoleTags } from "../components/RoleTags";
import { CreateKeyDialog, RevokeKeyDialog, ShowKeyDialog } from "./api-keys/KeyDialogs";

type Open = { kind: "create" } | { kind: "show"; plaintext: string } | { kind: "revoke"; apiKey: ApiKey } | null;

export function ApiKeysPage() {
  const query = useQuery({ queryKey: qk.apiKeys, queryFn: () => client.listApiKeys() });
  const [open, setOpen] = useState<Open>(null);
  const close = () => setOpen(null);

  return (
    <div className="page">
      <header className="page-header">
        <div>
          <h1>API keys</h1>
          <p className="muted">Keys authenticate the CLI and your integrations with <code>Authorization: Bearer ofk_…</code>.</p>
        </div>
        <button type="button" className="btn btn-primary" onClick={() => setOpen({ kind: "create" })}>Create API key</button>
      </header>
      {query.isPending ? (
        <Loading />
      ) : query.isError ? (
        <ErrorMessage error={query.error} />
      ) : query.data.length === 0 ? (
        <p className="empty">No API keys yet.</p>
      ) : (
        <table className="table">
          <thead>
            <tr>
              <th>Name</th>
              <th>Key</th>
              <th>Roles</th>
              <th>Created</th>
              <th>Last used</th>
              <th>Status</th>
              <th><span className="sr-only">Actions</span></th>
            </tr>
          </thead>
          <tbody>
            {query.data.map((k) => (
              <tr key={k.id}>
                <td>{k.name}</td>
                <td className="mono">{k.prefix}…</td>
                <td><RoleTags roles={k.roles} /></td>
                <td><RelativeTime iso={k.createdAt} /></td>
                <td>{k.lastUsedAt ? <RelativeTime iso={k.lastUsedAt} /> : <span className="muted">Never</span>}</td>
                <td>{k.revokedAt ? <span className="badge badge-red">Revoked</span> : <span className="badge badge-green">Active</span>}</td>
                <td>
                  {!k.revokedAt && (
                    <button type="button" className="btn" onClick={() => setOpen({ kind: "revoke", apiKey: k })}>Revoke</button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {open?.kind === "create" && <CreateKeyDialog onClose={close} onCreated={(plaintext) => setOpen({ kind: "show", plaintext })} />}
      {open?.kind === "show" && <ShowKeyDialog plaintext={open.plaintext} onClose={close} />}
      {open?.kind === "revoke" && <RevokeKeyDialog apiKey={open.apiKey} onClose={close} />}
    </div>
  );
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `pnpm -C web/apps/admin exec vitest run src/pages/ApiKeysPage.test.tsx`
Expected: PASS (3 tests).

Run: `pnpm -C web/apps/admin typecheck`
Expected: exits 0.

- [ ] **Step 6: Commit**

```bash
git add web/apps/admin/src/pages/ApiKeysPage.tsx web/apps/admin/src/pages/ApiKeysPage.test.tsx web/apps/admin/src/pages/api-keys
git commit -m "feat(admin): API keys page with show-once plaintext dialog"
```

---

### Task 10: Jobs (admin)

**Parallel group:** A (Lane 6). Touches only `src/pages/JobsPage.tsx` and its test.

**Files:**
- Modify (replace stub): `web/apps/admin/src/pages/JobsPage.tsx`
- Test: `web/apps/admin/src/pages/JobsPage.test.tsx`

**Interfaces:**
- Consumes: `client.listJobs(status)`, `client.retryJob(id)` and `qk.jobs(status)`.
- Produces: `JobsPage`, with status tabs (`failed` by default, plus `pending`, `running` and `done`) kept in the URL param `status`, and a Retry button on failed jobs.

- [ ] **Step 1: Write the failing test**

`web/apps/admin/src/pages/JobsPage.test.tsx`:
```tsx
import { describe, expect, it } from "vitest";
import { screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { server } from "../test/server";
import { api } from "../test/handlers";
import { ids, makeJob } from "../test/fixtures";
import { renderApp } from "../test/render";

describe("JobsPage", () => {
  it("shows failed jobs by default with error, attempts and a submission link, and retries them", async () => {
    const statuses: (string | null)[] = [];
    const retried: string[] = [];
    let failed = [makeJob()];
    server.use(
      http.get(api("/jobs"), ({ request }) => {
        const status = new URL(request.url).searchParams.get("status");
        statuses.push(status);
        return HttpResponse.json({ items: status === "failed" ? failed : [] });
      }),
      http.post(api("/jobs/:id/retry"), ({ params }) => {
        retried.push(String(params.id));
        failed = [];
        return new HttpResponse(null, { status: 204 });
      }),
    );
    const { user } = renderApp("/jobs");
    const row = (await screen.findByText("action.webhook")).closest("tr")!;
    expect(within(row).getByText("8/8")).toBeInTheDocument();
    expect(within(row).getByText(/500 Internal Server Error/)).toBeInTheDocument();
    expect(within(row).getByRole("link", { name: ids.sub1.slice(0, 8) })).toHaveAttribute("href", `/submissions/${ids.sub1}`);
    expect(statuses[0]).toBe("failed");

    await user.click(within(row).getByRole("button", { name: "Retry" }));
    await waitFor(() => expect(retried).toEqual(["7"]));
    expect(await screen.findByText("No failed jobs.")).toBeInTheDocument();
  });

  it("switches status tabs via the URL", async () => {
    const statuses: (string | null)[] = [];
    server.use(http.get(api("/jobs"), ({ request }) => {
      statuses.push(new URL(request.url).searchParams.get("status"));
      return HttpResponse.json({ items: [makeJob({ id: 8, status: "pending", attempts: 1, lastError: "" })] });
    }));
    const { user, router } = renderApp("/jobs");
    await screen.findByText("action.webhook");
    await user.click(screen.getByRole("button", { name: "Pending" }));
    await waitFor(() => expect(statuses.at(-1)).toBe("pending"));
    expect(router.state.location.search).toBe("?status=pending");
    expect(screen.getByRole("button", { name: "Pending" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.queryByRole("button", { name: "Retry" })).not.toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `pnpm -C web/apps/admin exec vitest run src/pages/JobsPage.test.tsx`
Expected: FAIL with `Unable to find an element with the text: action.webhook`.

- [ ] **Step 3: Write the jobs page**

`web/apps/admin/src/pages/JobsPage.tsx`:
```tsx
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useSearchParams } from "react-router-dom";
import { client } from "../api";
import { qk } from "../queryKeys";
import { shortId } from "../lib/format";
import { ErrorMessage } from "../components/ErrorMessage";
import { Loading } from "../components/Loading";
import { RelativeTime } from "../components/RelativeTime";

const STATUSES = [
  { value: "failed", label: "Failed" },
  { value: "pending", label: "Pending" },
  { value: "running", label: "Running" },
  { value: "done", label: "Done" },
];

export function JobsPage() {
  const [params, setParams] = useSearchParams();
  const status = STATUSES.some((s) => s.value === params.get("status")) ? params.get("status")! : "failed";
  const queryClient = useQueryClient();
  const query = useQuery({ queryKey: qk.jobs(status), queryFn: () => client.listJobs(status) });
  const retry = useMutation({
    mutationFn: (id: number) => client.retryJob(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["jobs"] }),
  });

  return (
    <div className="page">
      <header className="page-header">
        <div>
          <h1>Jobs</h1>
          <p className="muted">Webhook, email and assignment actions run in the background with retries.</p>
        </div>
        <button type="button" className="btn" onClick={() => query.refetch()}>Refresh</button>
      </header>

      <div className="tabs" role="group" aria-label="Job status">
        {STATUSES.map((s) => (
          <button key={s.value} type="button" className="btn" aria-pressed={s.value === status}
            onClick={() => setParams(s.value === "failed" ? {} : { status: s.value }, { replace: true })}>
            {s.label}
          </button>
        ))}
      </div>

      {retry.isError && <ErrorMessage error={retry.error} />}
      {query.isPending ? (
        <Loading />
      ) : query.isError ? (
        <ErrorMessage error={query.error} />
      ) : query.data.length === 0 ? (
        <p className="empty">No {status} jobs.</p>
      ) : (
        <table className="table">
          <thead>
            <tr>
              <th>ID</th>
              <th>Kind</th>
              <th>Submission</th>
              <th>Attempts</th>
              <th>Next run</th>
              <th>Last error</th>
              <th><span className="sr-only">Actions</span></th>
            </tr>
          </thead>
          <tbody>
            {query.data.map((job) => {
              const submissionId = typeof job.payload?.submissionId === "string" ? job.payload.submissionId : null;
              return (
                <tr key={job.id}>
                  <td className="mono">{job.id}</td>
                  <td className="mono">{job.kind}</td>
                  <td>{submissionId ? <Link className="mono" to={`/submissions/${submissionId}`}>{shortId(submissionId)}</Link> : <span className="muted">—</span>}</td>
                  <td>{job.attempts}/{job.maxAttempts}</td>
                  <td><RelativeTime iso={job.runAt} /></td>
                  <td title={job.lastError}>{job.lastError ? job.lastError.slice(0, 140) : <span className="muted">—</span>}</td>
                  <td>
                    {job.status === "failed" && (
                      <button type="button" className="btn" disabled={retry.isPending} onClick={() => retry.mutate(job.id)}>Retry</button>
                    )}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      )}
    </div>
  );
}
```

Note: the `Job.payload` type from the SDK is expected to be `Record<string, unknown>`. If Plan 06 typed it as `unknown`, change the `submissionId` line to `const payload = (job.payload ?? {}) as Record<string, unknown>; const submissionId = typeof payload.submissionId === "string" ? payload.submissionId : null;`.

- [ ] **Step 4: Run the test to verify it passes**

Run: `pnpm -C web/apps/admin exec vitest run src/pages/JobsPage.test.tsx`
Expected: PASS (2 tests).

Run: `pnpm -C web/apps/admin typecheck`
Expected: exits 0.

- [ ] **Step 5: Commit**

```bash
git add web/apps/admin/src/pages/JobsPage.tsx web/apps/admin/src/pages/JobsPage.test.tsx
git commit -m "feat(admin): jobs page with status tabs and retry"
```

---

### Task 11: Build integration, served from the Go binary, and the admin README

**Files:**
- Create: `web/apps/admin/README.md`
- Verify (no code change expected): `web/scripts/copy-dist.mjs` (Plan 06) copies `apps/admin/dist` → `internal/webui/dist/admin`

**Interfaces:**
- Consumes: Plan 06's `pnpm -C web build`, Plan 01's `internal/webui` handler (`/admin/*` → `dist/admin/index.html`, `/_app/*` static).
- Produces: a production admin bundle under `internal/webui/dist/admin/`, plus documentation of the Plan 08 extension points.

- [ ] **Step 1: Run the full admin suite and typecheck**

Run: `pnpm -C web/apps/admin test && pnpm -C web/apps/admin typecheck`
Expected: every suite passes (Tasks 1–10) and the typecheck exits 0.

- [ ] **Step 2: Build the workspace**

Run: `pnpm -C web build`
Expected: exits 0, and Vite prints `dist/index.html` and `dist/assets/index-*.js` for `@openforms/admin`.

Run: `ls internal/webui/dist/admin && grep -o '/_app/admin/assets/[^"]*\.js' internal/webui/dist/admin/index.html`
Expected: lists `index.html` and `assets`, and prints one path like `/_app/admin/assets/index-AbC123.js`.

- [ ] **Step 3: Check that the Go binary serves the admin app**

Start Postgres if it isn't running (`docker compose up -d postgres`), then run:
```bash
OPENFORMS_DATABASE_URL="postgres://openforms:openforms@localhost:54329/openforms?sslmode=disable" go run ./cmd/openforms serve &
SERVER_PID=$!
sleep 3
curl -s http://localhost:8080/admin/submissions | grep -c '<div id="root"></div>'
curl -s -o /dev/null -w '%{http_code}\n' "http://localhost:8080$(grep -o '/_app/admin/assets/[^"]*\.js' internal/webui/dist/admin/index.html)"
kill $SERVER_PID
```
Expected: `1`, then `200`.

- [ ] **Step 4: Write the admin README with the extension points**

`web/apps/admin/README.md`:
````markdown
# @openforms/admin

The openforms admin SPA, served by the Go binary at `/admin/*` (assets under `/_app/admin/`).

## Develop

```bash
docker compose up -d postgres mailpit
go run ./cmd/openforms serve            # API on :8080
pnpm -C web/apps/admin dev              # http://localhost:5174/_app/admin/ (proxies /api → :8080)
pnpm -C web/apps/admin test             # Vitest + Testing Library + MSW
pnpm -C web/apps/admin typecheck
```

## Extension points (used by Plan 08)

| What | Where | How to extend |
|---|---|---|
| Pages | `src/routes.tsx` → `AdminRoute = { path; element; adminOnly? }`, `routes: AdminRoute[]` | Append an entry; paths are relative to `/admin` (e.g. `"forms/:slug/edit"`). `adminOnly: true` wraps the page in `<RequireAdmin>`. |
| Sidebar | `src/nav.ts` → `NavItem = { to; label; adminOnly? }`, `navItems` | Append an entry. |
| Overview panels | `src/extensions/OverviewExtras.tsx` → `OverviewExtras({ kind: "form" \| "workflow"; slug })` | Replace the body; it renders at the bottom of `forms/:slug` and `workflows/:slug`. |
| API | `src/api.ts` → `client` (`OpenFormsClient`), `src/queryKeys.ts` → `qk` | Import directly; invalidate with `qk.*`. |
| Tests | `src/test/render.tsx` → `renderWithProviders(ui, { route?, path? })`, `renderApp(route)`; `src/test/server.ts` → MSW `server`; `src/test/handlers.ts` → `api()`, `apiError()`; `src/test/fixtures.ts` → `make*` factories | Use `server.use(...)` per test for extra endpoints. |

Existing links already point at Plan 08 routes: "New form" → `/admin/forms/new`, "Edit" → `/admin/forms/:slug/edit`, "New workflow" → `/admin/workflows/new`, "Edit" → `/admin/workflows/:slug/edit` (admins only).
````

- [ ] **Step 5: Commit**

```bash
git add web/apps/admin/README.md
git commit -m "docs(admin): document dev workflow and Plan 08 extension points"
```

Do not commit `internal/webui/dist/admin` unless Plan 01/06 decided build output is committed. By default only `internal/webui/dist/.gitkeep` is tracked.

---

## Self-review notes

- **Spec coverage (§9.5, excluding the Plan 08 editors and version history):**
  - Login: Task 2.
  - `/admin` → inbox redirect: Task 2.
  - Inbox filters, table, load-more and 30 s refresh: Task 3.
  - Detail answers (pinned labels) and timeline: Task 4.
  - Workflow fields PATCH, transitions with the requireFields dialog, disabled buttons with tooltips, 409 handling, assignee picker and comments: Task 5.
  - Forms overview (versions, links, CSV): Task 6.
  - Workflows list and overview: Task 7.
  - Users, API keys (show-once) and jobs: Tasks 8–10.
  - Build contract: Task 11.
- **Extension-point contract for Plan 08:** exact names in Global Constraints, Task 2 (`routes`, `navItems`), Task 6 (`OverviewExtras`), Task 1 (harness, `client`, `qk`) and the Task 11 README.
- **Review Focus coverage:**
  1. Session expiry: Task 3.
  2. Deep link and open redirect: Task 2.
  3. Legacy data keys: Task 4.
  4. Reviewer assignee picker without `/users`: Task 5.
  5. Unknown form or color-less state: Task 3.
````
