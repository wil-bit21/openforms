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
