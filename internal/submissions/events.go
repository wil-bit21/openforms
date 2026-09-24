package submissions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/openforms/openforms/internal/auth"
	"github.com/openforms/openforms/internal/db"
)

// Event types (spec §4).
const (
	EventCreated         = "created"
	EventTransition      = "transition"
	EventFieldsUpdated   = "fields_updated"
	EventAssigned        = "assigned"
	EventComment         = "comment"
	EventActionSucceeded = "action_succeeded"
	EventActionFailed    = "action_failed"
)

type Event struct {
	ID                             int64
	SubmissionID, OrgID            uuid.UUID
	Type                           string
	FromState, ToState, Transition string // "" when not applicable (NULL in DB)
	ActorType                      string
	ActorID                        *uuid.UUID
	ActorName                      string
	Payload                        map[string]any
	CreatedAt                      time.Time
}

type Actor struct {
	Type string // user|api_key|system|respondent
	ID   *uuid.UUID
	Name string
}

func ActorFromPrincipal(p auth.Principal) Actor {
	id := p.ID
	typ := "user"
	if p.Kind == auth.PrincipalAPIKey {
		typ = "api_key"
	}
	name := p.Name
	if name == "" {
		name = p.Email
	}
	return Actor{Type: typ, ID: &id, Name: name}
}

// actorFromContext returns the authenticated principal, or the anonymous respondent.
func actorFromContext(ctx context.Context) Actor {
	if p, ok := auth.PrincipalFrom(ctx); ok {
		return ActorFromPrincipal(p)
	}
	return Actor{Type: "respondent", Name: "Respondent"}
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// InsertEvent appends an event and returns it with ID and CreatedAt set.
func InsertEvent(ctx context.Context, q db.DBTX, e Event) (Event, error) {
	if e.Payload == nil {
		e.Payload = map[string]any{}
	}
	payload, err := json.Marshal(e.Payload)
	if err != nil {
		return Event{}, fmt.Errorf("encode event payload: %w", err)
	}
	err = q.QueryRow(ctx, `INSERT INTO submission_events
		(submission_id, org_id, type, from_state, to_state, transition, actor_type, actor_id, actor_name, payload)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, created_at`,
		e.SubmissionID, e.OrgID, e.Type, nullable(e.FromState), nullable(e.ToState), nullable(e.Transition),
		e.ActorType, e.ActorID, e.ActorName, payload).Scan(&e.ID, &e.CreatedAt)
	if err != nil {
		return Event{}, fmt.Errorf("insert event: %w", err)
	}
	return e, nil
}

// Events returns the submission's events, oldest first.
func (s *Service) Events(ctx context.Context, orgID, id uuid.UUID) ([]Event, error) {
	var exists bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM submissions WHERE org_id = $1 AND id = $2)`, orgID, id).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrNotFound
	}
	rows, err := s.pool.Query(ctx, `SELECT id, submission_id, org_id, type,
		COALESCE(from_state, ''), COALESCE(to_state, ''), COALESCE(transition, ''),
		actor_type, actor_id, actor_name, payload, created_at
		FROM submission_events WHERE submission_id = $1 ORDER BY id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var (
			e   Event
			raw []byte
		)
		if err := rows.Scan(&e.ID, &e.SubmissionID, &e.OrgID, &e.Type, &e.FromState, &e.ToState, &e.Transition,
			&e.ActorType, &e.ActorID, &e.ActorName, &raw, &e.CreatedAt); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, ErrNotFound
			}
			return nil, err
		}
		if err := json.Unmarshal(raw, &e.Payload); err != nil {
			return nil, fmt.Errorf("decode event payload: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
