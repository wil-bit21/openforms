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
