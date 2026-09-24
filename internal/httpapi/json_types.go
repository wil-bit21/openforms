package httpapi

import (
	"time"

	"github.com/google/uuid"

	"github.com/openforms/openforms/internal/definition"
	"github.com/openforms/openforms/internal/definitions"
	"github.com/openforms/openforms/internal/submissions"
)

// JSON shapes from spec §7.3. Timestamps are always rendered in UTC.

type listJSON[T any] struct {
	Items      []T     `json:"items"`
	NextCursor *string `json:"nextCursor"`
}

func newListJSON[T any](items []T, next string) listJSON[T] {
	if items == nil {
		items = []T{}
	}
	l := listJSON[T]{Items: items}
	if next != "" {
		l.NextCursor = &next
	}
	return l
}

type assigneeJSON struct {
	ID    uuid.UUID `json:"id"`
	Name  string    `json:"name"`
	Email string    `json:"email"`
}

type submissionJSON struct {
	ID          uuid.UUID      `json:"id"`
	Form        string         `json:"form"`
	FormVersion int            `json:"formVersion"`
	State       string         `json:"state"`
	StateLabel  string         `json:"stateLabel"`
	Terminal    bool           `json:"terminal"`
	Data        map[string]any `json:"data"`
	Fields      map[string]any `json:"fields"`
	Assignee    *assigneeJSON  `json:"assignee"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
}

func toSubmissionJSON(s submissions.Submission) submissionJSON {
	out := submissionJSON{
		ID: s.ID, Form: s.FormSlug, FormVersion: s.FormVersion,
		State: s.State, StateLabel: s.StateLabel, Terminal: s.Terminal,
		Data: s.Data, Fields: s.Fields,
		CreatedAt: s.CreatedAt.UTC(), UpdatedAt: s.UpdatedAt.UTC(),
	}
	if out.Data == nil {
		out.Data = map[string]any{}
	}
	if out.Fields == nil {
		out.Fields = map[string]any{}
	}
	if s.AssigneeID != nil {
		out.Assignee = &assigneeJSON{ID: *s.AssigneeID, Name: s.AssigneeName, Email: s.AssigneeEmail}
	}
	return out
}

type actorJSON struct {
	Type string     `json:"type"`
	ID   *uuid.UUID `json:"id"`
	Name string     `json:"name"`
}

type eventJSON struct {
	ID         int64          `json:"id"`
	Type       string         `json:"type"`
	FromState  *string        `json:"fromState"`
	ToState    *string        `json:"toState"`
	Transition *string        `json:"transition"`
	Actor      actorJSON      `json:"actor"`
	Payload    map[string]any `json:"payload"`
	CreatedAt  time.Time      `json:"createdAt"`
}

func strOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func toEventJSON(e submissions.Event) eventJSON {
	out := eventJSON{
		ID: e.ID, Type: e.Type,
		FromState: strOrNil(e.FromState), ToState: strOrNil(e.ToState), Transition: strOrNil(e.Transition),
		Actor:     actorJSON{Type: e.ActorType, ID: e.ActorID, Name: e.ActorName},
		Payload:   e.Payload,
		CreatedAt: e.CreatedAt.UTC(),
	}
	if out.Payload == nil {
		out.Payload = map[string]any{}
	}
	return out
}

func toEventsJSON(evs []submissions.Event) []eventJSON {
	out := make([]eventJSON, 0, len(evs))
	for _, e := range evs {
		out = append(out, toEventJSON(e))
	}
	return out
}

type versionJSON struct {
	Version   int       `json:"version"`
	Hash      string    `json:"hash"`
	Source    string    `json:"source"`
	CreatedBy string    `json:"createdBy"`
	CreatedAt time.Time `json:"createdAt"`
}

func toVersionJSON(v definitions.VersionInfo) versionJSON {
	return versionJSON{Version: v.Version, Hash: v.Hash, Source: string(v.Source), CreatedBy: v.CreatedBy, CreatedAt: v.CreatedAt.UTC()}
}

type formRecordJSON struct {
	Slug            string          `json:"slug"`
	Version         int             `json:"version"`
	Source          string          `json:"source"`
	UpdatedAt       time.Time       `json:"updatedAt"`
	WorkflowVersion *int            `json:"workflowVersion"`
	Definition      definition.Form `json:"definition"`
}

type workflowRecordJSON struct {
	Slug       string              `json:"slug"`
	Version    int                 `json:"version"`
	Source     string              `json:"source"`
	UpdatedAt  time.Time           `json:"updatedAt"`
	Definition definition.Workflow `json:"definition"`
}

func toWorkflowRecordJSON(r definitions.WorkflowRecord) workflowRecordJSON {
	return workflowRecordJSON{Slug: r.Slug, Version: r.Current.Version, Source: string(r.Current.Source), UpdatedAt: r.UpdatedAt.UTC(), Definition: r.Definition}
}
