"""password reset tokens

Revision ID: 0002_password_resets
Revises: 0001_initial
Create Date: 2026-09-25
"""

from __future__ import annotations

from alembic import op

revision = "0002_password_resets"
down_revision = "0001_initial"
branch_labels = None
depends_on = None


def upgrade() -> None:
    op.execute(
        """
CREATE TABLE password_resets (
  token_hash text PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
)"""
    )
    op.execute("CREATE INDEX password_resets_user ON password_resets (user_id)")


def downgrade() -> None:
    op.execute("DROP TABLE IF EXISTS password_resets")
