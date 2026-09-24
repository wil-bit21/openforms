// Package definitions persists versioned form and workflow definitions.
package definitions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openforms/openforms/internal/definition"
)

var ErrNotFound = errors.New("definition not found")

type Source string

const (
	SourceCLI  Source = "cli"
	SourceUI   Source = "ui"
	SourceAPI  Source = "api"
	SourceSeed Source = "seed"
)

// ParseSource reports whether s is a known version source.
func ParseSource(s string) (Source, bool) {
	switch Source(s) {
	case SourceCLI, SourceUI, SourceAPI, SourceSeed:
		return Source(s), true
	}
	return "", false
}

type VersionInfo struct {
	ID        uuid.UUID
	Version   int
	Hash      string
	Source    Source
	CreatedBy string
	CreatedAt time.Time
}

// FormRecord is a form at one version. For records returned by version
// lookups (GetFormVersion, FormVersion) UpdatedAt equals Current.CreatedAt.
type FormRecord struct {
	ID                uuid.UUID
	Slug              string
	Current           VersionInfo
	WorkflowVersionID *uuid.UUID
	Definition        definition.Form
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type WorkflowRecord struct {
	ID         uuid.UUID
	Slug       string
	Current    VersionInfo
	Definition definition.Workflow
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type ApplyInput struct {
	Forms     []definition.Form
	Workflows []definition.Workflow
	Source    Source
	Actor     string
	DryRun    bool
}

type ApplyItem struct {
	Kind    string `json:"kind"`
	Slug    string `json:"slug"`
	Version int    `json:"version"`
	Changed bool   `json:"changed"`
	Created bool   `json:"created"`
}

type ApplyResult struct {
	Items []ApplyItem `json:"items"`
}

// Store reads and writes definitions. Versions are immutable, so records
// looked up by version id are cached for the life of the process.
// Callers must not mutate returned definitions.
type Store struct {
	pool             *pgxpool.Pool
	formVersions     sync.Map // uuid.UUID → FormRecord
	workflowVersions sync.Map // uuid.UUID → WorkflowRecord
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

const formCols = `SELECT f.id, f.slug, f.created_at, f.updated_at,
	v.id, v.version, v.hash, v.source, v.created_by, v.created_at, v.workflow_version_id, v.definition
FROM form_versions v JOIN forms f ON f.id = v.form_id`

const workflowCols = `SELECT w.id, w.slug, w.created_at, w.updated_at,
	v.id, v.version, v.hash, v.source, v.created_by, v.created_at, v.definition
FROM workflow_versions v JOIN workflows w ON w.id = v.workflow_id`

func scanForm(row pgx.Row) (FormRecord, error) {
	var (
		r   FormRecord
		src string
		raw []byte
	)
	err := row.Scan(&r.ID, &r.Slug, &r.CreatedAt, &r.UpdatedAt,
		&r.Current.ID, &r.Current.Version, &r.Current.Hash, &src, &r.Current.CreatedBy, &r.Current.CreatedAt,
		&r.WorkflowVersionID, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return FormRecord{}, ErrNotFound
	}
	if err != nil {
		return FormRecord{}, fmt.Errorf("scan form: %w", err)
	}
	r.Current.Source = Source(src)
	if err := json.Unmarshal(raw, &r.Definition); err != nil {
		return FormRecord{}, fmt.Errorf("decode form %s: %w", r.Slug, err)
	}
	return r, nil
}

func scanWorkflow(row pgx.Row) (WorkflowRecord, error) {
	var (
		r   WorkflowRecord
		src string
		raw []byte
	)
	err := row.Scan(&r.ID, &r.Slug, &r.CreatedAt, &r.UpdatedAt,
		&r.Current.ID, &r.Current.Version, &r.Current.Hash, &src, &r.Current.CreatedBy, &r.Current.CreatedAt, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return WorkflowRecord{}, ErrNotFound
	}
	if err != nil {
		return WorkflowRecord{}, fmt.Errorf("scan workflow: %w", err)
	}
	r.Current.Source = Source(src)
	if err := json.Unmarshal(raw, &r.Definition); err != nil {
		return WorkflowRecord{}, fmt.Errorf("decode workflow %s: %w", r.Slug, err)
	}
	return r, nil
}

// GetForm returns the current version of a form.
func (s *Store) GetForm(ctx context.Context, orgID uuid.UUID, slug string) (FormRecord, error) {
	return scanForm(s.pool.QueryRow(ctx, formCols+` WHERE f.org_id = $1 AND f.slug = $2 AND f.current_version_id = v.id`, orgID, slug))
}

// GetWorkflow returns the current version of a workflow.
func (s *Store) GetWorkflow(ctx context.Context, orgID uuid.UUID, slug string) (WorkflowRecord, error) {
	return scanWorkflow(s.pool.QueryRow(ctx, workflowCols+` WHERE w.org_id = $1 AND w.slug = $2 AND w.current_version_id = v.id`, orgID, slug))
}
