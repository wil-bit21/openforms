import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";

type StateKey = "new" | "screening" | "interview" | "hired" | "rejected";

const STATES = [
  { key: "new", label: "New", color: "gray" },
  { key: "screening", label: "Screening", color: "blue" },
  { key: "interview", label: "Interview", color: "purple" },
  { key: "hired", label: "Hired", color: "green", terminal: true },
  { key: "rejected", label: "Rejected", color: "red", terminal: true },
] as const;

const WORKFLOW = {
  slug: "hiring",
  title: "Hiring pipeline",
  initial: "new",
  states: STATES,
  fields: [
    { key: "score", type: "number", label: "Score" },
    { key: "rejectionReason", type: "textarea", label: "Rejection reason" },
  ],
  transitions: [
    { key: "screen", label: "Start screening", from: ["new"], to: "screening", guard: { roles: ["reviewer"] } },
    { key: "invite", label: "Invite to interview", from: ["screening"], to: "interview", guard: { roles: ["reviewer"], requireFields: ["score"] } },
    { key: "hire", label: "Hire", from: ["interview"], to: "hired", guard: { roles: ["hiring-manager"] } },
    { key: "reject", label: "Reject", from: ["new", "screening", "interview"], to: "rejected", guard: { roles: ["reviewer", "hiring-manager"], requireFields: ["rejectionReason"] } },
  ],
};

export const FORM = {
  slug: "job-application",
  title: "Job application",
  settings: { public: true, submitLabel: "Send application", confirmationMessage: "Thanks for applying!" },
  fields: [{ key: "name", type: "text", label: "Full name", required: true }],
};

export const demoApi = {
  state: "new" as StateKey,
  fields: {} as Record<string, unknown>,
  persona: "" as string,
  loginCalls: [] as string[],
  transitionBodies: [] as Array<Record<string, unknown>>,
};

export function resetDemoApi() {
  demoApi.state = "new";
  demoApi.fields = {};
  demoApi.persona = "";
  demoApi.loginCalls = [];
  demoApi.transitionBodies = [];
}

const label = (k: string) => STATES.find((s) => s.key === k)!.label;
const terminal = (k: string) => Boolean(STATES.find((s) => s.key === k && "terminal" in s));

function available() {
  const roles = demoApi.persona === "manager@demo.local" ? ["hiring-manager"] : ["reviewer"];
  return WORKFLOW.transitions
    .filter((t) => t.from.includes(demoApi.state))
    .map((t) => {
      const allowed = t.guard.roles.some((r) => roles.includes(r));
      return {
        key: t.key,
        label: t.label,
        to: t.to,
        toLabel: label(t.to),
        requireFields: t.guard.requireFields ?? [],
        allowed,
        reason: allowed ? "" : "role",
      };
    });
}

function submission() {
  return {
    id: "sub-1",
    form: "job-application",
    formVersion: 1,
    state: demoApi.state,
    stateLabel: label(demoApi.state),
    terminal: terminal(demoApi.state),
    data: { name: "Ada" },
    fields: demoApi.fields,
    assignee: null,
    createdAt: "2026-09-23T10:00:00Z",
    updatedAt: "2026-09-23T10:00:00Z",
  };
}

const err = (status: number, code: string, message: string) =>
  HttpResponse.json({ error: { code, message } }, { status });

export const handlers = [
  http.get("*/api/v1/public/config", () => HttpResponse.json({ demo: true })),
  http.get("*/api/v1/public/forms/job-application", () => HttpResponse.json({ form: FORM })),
  http.post("*/api/v1/public/forms/job-application/submissions", () =>
    HttpResponse.json(
      { id: "sub-1", state: "new", stateLabel: "New", receiptToken: "tok-1", confirmationMessage: "Thanks for applying!" },
      { status: 201 },
    ),
  ),
  http.get("*/api/v1/public/submissions/sub-1", () =>
    HttpResponse.json({
      id: "sub-1",
      formTitle: "Job application",
      state: demoApi.state,
      stateLabel: label(demoApi.state),
      terminal: terminal(demoApi.state),
      states: STATES,
      history: [{ state: "new", label: "New", at: "2026-09-23T10:00:00Z" }],
      createdAt: "2026-09-23T10:00:00Z",
    }),
  ),
  http.post("*/api/v1/auth/login", async ({ request }) => {
    const body = (await request.json()) as { email: string; password: string };
    demoApi.loginCalls.push(body.email);
    if (body.password !== "demo1234") return err(401, "invalid_credentials", "Invalid email or password");
    demoApi.persona = body.email;
    return HttpResponse.json({
      user: { id: "u-1", email: body.email, name: body.email, roles: [], createdAt: "2026-09-23T10:00:00Z" },
    });
  }),
  http.get("*/api/v1/submissions/sub-1", () =>
    HttpResponse.json({ submission: submission(), form: FORM, workflow: WORKFLOW, events: [], transitions: available() }),
  ),
  http.post("*/api/v1/submissions/sub-1/transitions", async ({ request }) => {
    const body = (await request.json()) as { transition: string; fields?: Record<string, unknown> };
    demoApi.transitionBodies.push(body);
    const t = available().find((x) => x.key === body.transition);
    if (!t) return err(409, "invalid_state", "Transition not available from this state");
    if (!t.allowed) return err(403, "forbidden", "You are not allowed to perform this transition");
    demoApi.fields = { ...demoApi.fields, ...(body.fields ?? {}) };
    demoApi.state = t.to as StateKey;
    return HttpResponse.json({ submission: submission() });
  }),
  http.get("*/embed.js", () => new HttpResponse("/* embed stub */", { headers: { "Content-Type": "text/javascript" } })),
];

export const server = setupServer(...handlers);
