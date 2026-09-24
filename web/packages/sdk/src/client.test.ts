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
