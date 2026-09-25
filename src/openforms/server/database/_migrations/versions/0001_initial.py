"""initial schema (the Go server's goose migrations 00001-00003, verbatim)

Revision ID: 0001_initial
Revises:
Create Date: 2026-09-24
"""

from __future__ import annotations

import sqlalchemy as sa
from alembic import op

revision = "0001_initial"
down_revision = None
branch_labels = None
depends_on = None

# --- Go migration 00001_core.sql
CORE_SQL = """
CREATE TABLE orgs (
  id uuid PRIMARY KEY,
  name text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE users (
  id uuid PRIMARY KEY,
  org_id uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
  email text NOT NULL,
  name text NOT NULL,
  password_hash text NOT NULL,
  roles text[] NOT NULL DEFAULT '{}',
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX users_org_email ON users (org_id, lower(email));
CREATE TABLE sessions (
  token_hash text PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE api_keys (
  id uuid PRIMARY KEY,
  org_id uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
  name text NOT NULL,
  prefix text NOT NULL,
  key_hash text NOT NULL UNIQUE,
  roles text[] NOT NULL DEFAULT '{}',
  created_at timestamptz NOT NULL DEFAULT now(),
  last_used_at timestamptz,
  revoked_at timestamptz
);
"""

# --- Go migration 00002_definitions_submissions.sql
DEFINITIONS_SUBMISSIONS_SQL = """
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
"""

# --- Go migration 00003_jobs.sql
JOBS_SQL = """
CREATE TABLE jobs (
  id bigserial PRIMARY KEY,
  org_id uuid NOT NULL,
  kind text NOT NULL,
  payload jsonb NOT NULL,
  status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','running','done','failed')),
  attempts int NOT NULL DEFAULT 0,
  max_attempts int NOT NULL DEFAULT 8,
  run_at timestamptz NOT NULL DEFAULT now(),
  locked_until timestamptz,
  last_error text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX jobs_claim ON jobs (status, run_at);
"""


def upgrade() -> None:
    bind = op.get_bind()
    # A database created by the Go server already has these tables (goose
    # version 3); adopt it instead of failing.
    if bind.execute(sa.text("SELECT to_regclass('jobs') IS NOT NULL")).scalar():
        return
    for sql in (CORE_SQL, DEFINITIONS_SUBMISSIONS_SQL, JOBS_SQL):
        for stmt in sql.split(";"):
            if stmt.strip():
                op.execute(stmt)


def downgrade() -> None:
    for table in (
        "jobs",
        "submission_events",
        "submissions",
        "form_versions",
        "forms",
        "workflow_versions",
        "workflows",
        "api_keys",
        "sessions",
        "users",
        "orgs",
    ):
        op.execute(f"DROP TABLE IF EXISTS {table}")
