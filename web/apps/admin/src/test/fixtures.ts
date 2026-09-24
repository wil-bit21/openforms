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
