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
