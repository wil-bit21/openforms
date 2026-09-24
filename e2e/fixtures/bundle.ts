/** E2E-only definitions: the webhook points at the sink on the host (see docker-compose extra_hosts). */
export const E2E_WORKFLOW = {
  slug: "e2e-flow",
  title: "E2E flow",
  initial: "new",
  states: [
    { key: "new", label: "New", color: "gray" },
    { key: "approved", label: "Approved", color: "green", terminal: true },
    { key: "rejected", label: "Rejected", color: "red", terminal: true },
  ],
  fields: [{ key: "note", type: "textarea", label: "Note" }],
  transitions: [
    {
      key: "approve",
      label: "Approve",
      from: ["new"],
      to: "approved",
      guard: { roles: ["reviewer"], requireFields: ["note"] },
      actions: [
        { type: "webhook", url: "http://host.docker.internal:9911/hook" },
        {
          type: "email",
          to: "{{submission.data.email}}",
          subject: "E2E approved {{submission.id}}",
          body: "Note: {{submission.fields.note}}",
        },
      ],
    },
    { key: "reject", label: "Reject", from: ["new"], to: "rejected", guard: { roles: ["reviewer"] } },
  ],
};

export const E2E_FORM = {
  slug: "e2e-apply",
  title: "E2E application",
  workflow: "e2e-flow",
  settings: { public: true, submitLabel: "Send", confirmationMessage: "Thanks, e2e!" },
  fields: [
    { key: "name", type: "text", label: "Name", required: true },
    { key: "email", type: "email", label: "Email", required: true },
  ],
};
