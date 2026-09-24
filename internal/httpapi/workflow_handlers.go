package httpapi

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/openforms/openforms/internal/definition"
	"github.com/openforms/openforms/internal/submissions"
	"github.com/openforms/openforms/internal/workflow"
)

type availableTransitionJSON struct {
	Key           string   `json:"key"`
	Label         string   `json:"label"`
	To            string   `json:"to"`
	ToLabel       string   `json:"toLabel"`
	RequireFields []string `json:"requireFields"`
	Allowed       bool     `json:"allowed"`
	Reason        string   `json:"reason"`
}

// availableTransitions returns the detail endpoint's "transitions" array
// ([] when no engine is wired or the form has no workflow).
func availableTransitions(r *http.Request, d Deps, sub submissions.Submission) ([]availableTransitionJSON, error) {
	out := []availableTransitionJSON{}
	if d.Engine == nil {
		return out, nil
	}
	av, err := d.Engine.Available(r.Context(), MustPrincipal(r), sub)
	if err != nil {
		return nil, err
	}
	for _, a := range av {
		req := a.RequireFields
		if req == nil {
			req = []string{}
		}
		out = append(out, availableTransitionJSON{
			Key: a.Key, Label: a.Label, To: a.To, ToLabel: a.ToLabel,
			RequireFields: req, Allowed: a.Allowed, Reason: a.Reason,
		})
	}
	return out, nil
}

type workflowHandlers struct{ d Deps }

func mountWorkflow(r chi.Router, d Deps) {
	h := workflowHandlers{d: d}
	r.Post("/submissions/{id}/transitions", h.transition)
	r.Patch("/submissions/{id}/fields", h.updateFields)
	r.Post("/submissions/{id}/comments", h.comment)
	r.Put("/submissions/{id}/assignee", h.assign)
}

type transitionRequest struct {
	Transition    string         `json:"transition"`
	Fields        map[string]any `json:"fields"`
	Comment       string         `json:"comment"`
	ExpectedState string         `json:"expectedState"`
}

func (h workflowHandlers) transition(w http.ResponseWriter, r *http.Request) {
	id, err := URLParamUUID(r, "id")
	if err != nil {
		WriteError(w, r, err)
		return
	}
	var req transitionRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, r, err)
		return
	}
	if strings.TrimSpace(req.Transition) == "" {
		WriteError(w, r, &definition.ValidationError{Problems: []definition.Problem{{Path: "transition", Message: "transition is required"}}})
		return
	}
	sub, err := h.d.Engine.Transition(r.Context(), MustPrincipal(r), id, workflow.TransitionInput{
		Transition: req.Transition, Fields: req.Fields, Comment: req.Comment, ExpectedState: req.ExpectedState,
	})
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"submission": toSubmissionJSON(sub)})
}

func (h workflowHandlers) updateFields(w http.ResponseWriter, r *http.Request) {
	id, err := URLParamUUID(r, "id")
	if err != nil {
		WriteError(w, r, err)
		return
	}
	var req struct {
		Fields map[string]any `json:"fields"`
	}
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, r, err)
		return
	}
	sub, err := h.d.Engine.UpdateFields(r.Context(), MustPrincipal(r), id, req.Fields)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"submission": toSubmissionJSON(sub)})
}

func (h workflowHandlers) comment(w http.ResponseWriter, r *http.Request) {
	id, err := URLParamUUID(r, "id")
	if err != nil {
		WriteError(w, r, err)
		return
	}
	var req struct {
		Body string `json:"body"`
	}
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, r, err)
		return
	}
	ev, err := h.d.Engine.Comment(r.Context(), MustPrincipal(r), id, req.Body)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusCreated, map[string]any{"event": toEventJSON(ev)})
}

func (h workflowHandlers) assign(w http.ResponseWriter, r *http.Request) {
	id, err := URLParamUUID(r, "id")
	if err != nil {
		WriteError(w, r, err)
		return
	}
	var req struct {
		UserID *uuid.UUID `json:"userId"`
	}
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, r, err)
		return
	}
	sub, err := h.d.Engine.Assign(r.Context(), MustPrincipal(r), id, req.UserID)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"submission": toSubmissionJSON(sub)})
}
