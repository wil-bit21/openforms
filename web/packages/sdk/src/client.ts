import type {
  PublicConfig,
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

  /** GET /api/v1/public/config (unauthenticated). */
  async getPublicConfig(): Promise<PublicConfig> {
    return this.request<PublicConfig>("GET", "/public/config");
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

  /** Emails a reset link if the address belongs to a user. Always resolves (202), whether or not it does. */
  async requestPasswordReset(email: string): Promise<void> {
    await this.request<void>("POST", "/auth/password-reset", { body: { email } });
  }

  /** Sets a new password with the token from the reset email; every session of the user is signed out. */
  async confirmPasswordReset(token: string, password: string): Promise<void> {
    await this.request<void>("POST", "/auth/password-reset/confirm", { body: { token, password } });
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
