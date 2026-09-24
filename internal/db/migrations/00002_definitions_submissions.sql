-- +goose Up
CREATE TABLE workflows (
  id uuid PRIMARY KEY,
  org_id uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
  slug text NOT NULL,
  current_version_id uuid,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (org_id, slug)
);
CREATE TABLE workflow_versions (
  id uuid PRIMARY KEY,
  workflow_id uuid NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
  version int NOT NULL,
  definition jsonb NOT NULL,
  hash text NOT NULL,
  source text NOT NULL CHECK (source IN ('cli','ui','api','seed')),
  created_by text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (workflow_id, version)
);
CREATE TABLE forms (
  id uuid PRIMARY KEY,
  org_id uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
  slug text NOT NULL,
  current_version_id uuid,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (org_id, slug)
);
CREATE TABLE form_versions (
  id uuid PRIMARY KEY,
  form_id uuid NOT NULL REFERENCES forms(id) ON DELETE CASCADE,
  version int NOT NULL,
  definition jsonb NOT NULL,
  hash text NOT NULL,
  workflow_version_id uuid REFERENCES workflow_versions(id),
  source text NOT NULL CHECK (source IN ('cli','ui','api','seed')),
  created_by text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (form_id, version)
);
CREATE TABLE submissions (
  id uuid PRIMARY KEY,
  org_id uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
  form_id uuid NOT NULL REFERENCES forms(id) ON DELETE CASCADE,
  form_version_id uuid NOT NULL REFERENCES form_versions(id),
  workflow_version_id uuid REFERENCES workflow_versions(id),
  state text NOT NULL,
  data jsonb NOT NULL,
  fields jsonb NOT NULL DEFAULT '{}',
  assignee_id uuid REFERENCES users(id) ON DELETE SET NULL,
  receipt_token_hash text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX submissions_list ON submissions (org_id, created_at DESC, id DESC);
CREATE INDEX submissions_form_state ON submissions (form_id, state);
CREATE INDEX submissions_assignee ON submissions (assignee_id);
CREATE TABLE submission_events (
  id bigserial PRIMARY KEY,
  submission_id uuid NOT NULL REFERENCES submissions(id) ON DELETE CASCADE,
  org_id uuid NOT NULL,
  type text NOT NULL,
  from_state text,
  to_state text,
  transition text,
  actor_type text NOT NULL CHECK (actor_type IN ('user','api_key','system','respondent')),
  actor_id uuid,
  actor_name text NOT NULL DEFAULT '',
  payload jsonb NOT NULL DEFAULT '{}',
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX submission_events_sub ON submission_events (submission_id, id);
-- +goose Down
DROP TABLE submission_events; DROP TABLE submissions; DROP TABLE form_versions;
DROP TABLE forms; DROP TABLE workflow_versions; DROP TABLE workflows;
