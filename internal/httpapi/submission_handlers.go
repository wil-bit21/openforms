package httpapi

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/openforms/openforms/internal/definition"
	"github.com/openforms/openforms/internal/submissions"
)

// mountSubmissions registers /submissions routes and the CSV export (authenticated group).
func mountSubmissions(r chi.Router, d Deps) {
	h := &submissionHandlers{d: d}
	r.Get("/submissions", h.list)
	r.Post("/submissions", h.create)
	r.Get("/submissions/{id}", h.get)
	r.Get("/forms/{slug}/submissions.csv", h.csv)
}

type submissionHandlers struct{ d Deps }

func (h *submissionHandlers) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := submissions.ListFilter{FormSlug: q.Get("form"), State: q.Get("state"), Cursor: q.Get("cursor")}
	switch a := q.Get("assignee"); a {
	case "":
	case "none":
		f.Unassigned = true
	case "me":
		id := MustPrincipal(r).ID
		f.AssigneeID = &id
	default:
		id, err := uuid.Parse(a)
		if err != nil {
			WriteError(w, r, badRequest(http.StatusBadRequest, `assignee must be a user id, "me" or "none"`))
			return
		}
		f.AssigneeID = &id
	}
	if l := q.Get("limit"); l != "" {
		n, err := strconv.Atoi(l)
		if err != nil || n < 1 {
			WriteError(w, r, badRequest(http.StatusBadRequest, "limit must be a positive integer"))
			return
		}
		f.Limit = n
	}
	items, next, err := h.d.Subs.List(r.Context(), h.d.OrgID, f)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	out := make([]submissionJSON, 0, len(items))
	for _, s := range items {
		out = append(out, toSubmissionJSON(s))
	}
	WriteJSON(w, http.StatusOK, newListJSON(out, next))
}

type createSubmissionRequest struct {
	Form string         `json:"form"`
	Data map[string]any `json:"data"`
}

func (h *submissionHandlers) create(w http.ResponseWriter, r *http.Request) {
	var req createSubmissionRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, r, err)
		return
	}
	sub, token, err := h.d.Subs.Create(r.Context(), h.d.OrgID, req.Form, req.Data, false)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusCreated, map[string]any{"submission": toSubmissionJSON(sub), "receiptToken": token})
}

func (h *submissionHandlers) get(w http.ResponseWriter, r *http.Request) {
	id, err := URLParamUUID(r, "id")
	if err != nil {
		WriteError(w, r, err)
		return
	}
	ctx := r.Context()
	sub, err := h.d.Subs.Get(ctx, h.d.OrgID, id)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	form, err := h.d.Defs.GetFormVersion(ctx, sub.FormVersionID)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	var wf *definition.Workflow
	if sub.WorkflowVersionID != nil {
		rec, err := h.d.Defs.GetWorkflowVersion(ctx, *sub.WorkflowVersionID)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		wf = &rec.Definition
	}
	events, err := h.d.Subs.Events(ctx, h.d.OrgID, id)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	transitions, err := h.transitionsFor(r, sub)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"submission":  toSubmissionJSON(sub),
		"form":        form.Definition,
		"workflow":    wf,
		"events":      toEventsJSON(events),
		"transitions": transitions,
	})
}

// transitionsFor lists the transitions available to the caller (Engine.Available).
func (h *submissionHandlers) transitionsFor(r *http.Request, sub submissions.Submission) (any, error) {
	return availableTransitions(r, h.d, sub)
}

// lazyCSVWriter sets the download headers on the first write, so a failure
// before any output can still be reported as a JSON error.
type lazyCSVWriter struct {
	w        http.ResponseWriter
	filename string
	started  bool
}

func (l *lazyCSVWriter) Write(p []byte) (int, error) {
	if !l.started {
		l.started = true
		l.w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		l.w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, l.filename))
		l.w.WriteHeader(http.StatusOK)
	}
	return l.w.Write(p)
}

func (h *submissionHandlers) csv(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	lw := &lazyCSVWriter{w: w, filename: slug + "-submissions.csv"}
	if err := h.d.Subs.ExportCSV(r.Context(), h.d.OrgID, slug, lw); err != nil && !lw.started {
		WriteError(w, r, err)
	}
}
