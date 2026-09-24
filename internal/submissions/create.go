package submissions

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/openforms/openforms/internal/db"
	"github.com/openforms/openforms/internal/definition"
	"github.com/openforms/openforms/internal/definitions"
)

// Create validates data against the form's current version and stores it in
// the workflow's initial state (or "submitted" when the form has no workflow).
// requirePublic rejects forms whose settings.public is false with ErrFormNotPublic.
// It returns the submission and its plaintext receipt token (shown once).
func (s *Service) Create(ctx context.Context, orgID uuid.UUID, formSlug string, data map[string]any, requirePublic bool) (Submission, string, error) {
	form, err := s.defs.GetForm(ctx, orgID, formSlug)
	if errors.Is(err, definitions.ErrNotFound) {
		return Submission{}, "", ErrNotFound
	}
	if err != nil {
		return Submission{}, "", err
	}
	if requirePublic && !form.Definition.Settings.Public {
		return Submission{}, "", ErrFormNotPublic
	}
	if data == nil {
		data = map[string]any{}
	}
	clean, err := definition.ValidateSubmission(form.Definition, data)
	if err != nil {
		return Submission{}, "", err
	}

	sub := Submission{
		ID:                uuid.New(),
		OrgID:             orgID,
		FormID:            form.ID,
		FormVersionID:     form.Current.ID,
		FormSlug:          form.Slug,
		FormVersion:       form.Current.Version,
		WorkflowVersionID: form.WorkflowVersionID,
		State:             NoWorkflowState,
		Data:              clean,
		Fields:            map[string]any{},
	}
	var wf *definition.Workflow
	if form.WorkflowVersionID != nil {
		rec, err := s.defs.GetWorkflowVersion(ctx, *form.WorkflowVersionID)
		if err != nil {
			return Submission{}, "", fmt.Errorf("load workflow version: %w", err)
		}
		wf = &rec.Definition
		sub.State = wf.Initial
	}
	sub.StateLabel, sub.Terminal = stateLabel(wf, sub.State)

	token, tokenHash, err := newReceiptToken()
	if err != nil {
		return Submission{}, "", err
	}
	dataJSON, err := json.Marshal(clean)
	if err != nil {
		return Submission{}, "", fmt.Errorf("encode data: %w", err)
	}
	actor := actorFromContext(ctx)

	err = db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `INSERT INTO submissions
			(id, org_id, form_id, form_version_id, workflow_version_id, state, data, fields, receipt_token_hash)
			VALUES ($1, $2, $3, $4, $5, $6, $7, '{}', $8)
			RETURNING created_at, updated_at`,
			sub.ID, orgID, sub.FormID, sub.FormVersionID, sub.WorkflowVersionID, sub.State, dataJSON, tokenHash,
		).Scan(&sub.CreatedAt, &sub.UpdatedAt); err != nil {
			return fmt.Errorf("insert submission: %w", err)
		}
		if _, err := InsertEvent(ctx, tx, Event{
			SubmissionID: sub.ID, OrgID: orgID, Type: EventCreated, ToState: sub.State,
			ActorType: actor.Type, ActorID: actor.ID, ActorName: actor.Name,
		}); err != nil {
			return err
		}
		if s.hook != nil {
			return s.hook(ctx, tx, sub, wf)
		}
		return nil
	})
	if err != nil {
		return Submission{}, "", err
	}
	return sub, token, nil
}

// newReceiptToken returns a 24-byte base64url token and its sha256 hex digest.
func newReceiptToken() (token, hash string, err error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("receipt token: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, hashToken(token), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
