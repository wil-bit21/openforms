package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/openforms/openforms/internal/auth"
	"github.com/openforms/openforms/internal/definition"
	"github.com/openforms/openforms/internal/definitions"
)

// mountDefinitions registers /forms, /workflows and /definitions routes (authenticated group).
func mountDefinitions(r chi.Router, d Deps) {
	h := &definitionHandlers{d: d}
	r.Get("/forms", h.listForms)
	r.Get("/forms/{slug}", h.getForm)
	r.With(RequireAdmin).Put("/forms/{slug}", h.putForm)
	r.Get("/forms/{slug}/versions", h.formVersions)
	r.Get("/forms/{slug}/versions/{version}", h.formVersion)

	r.Get("/workflows", h.listWorkflows)
	r.Get("/workflows/{slug}", h.getWorkflow)
	r.With(RequireAdmin).Put("/workflows/{slug}", h.putWorkflow)
	r.Get("/workflows/{slug}/versions", h.workflowVersions)
	r.Get("/workflows/{slug}/versions/{version}", h.workflowVersion)

	r.With(RequireAdmin).Post("/definitions/validate", h.validate)
	r.With(RequireAdmin).Post("/definitions/apply", h.apply)
	r.With(RequireAdmin).Get("/definitions", h.export)
}

type definitionHandlers struct{ d Deps }

var errNotFound = &APIError{Status: http.StatusNotFound, Code: "not_found", Message: "not found"}

// readBody reads a request body of at most 1 MiB.
func readBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		return nil, &APIError{Status: http.StatusBadRequest, Code: "bad_request", Message: "request body too large or unreadable"}
	}
	return raw, nil
}

// actorLabel is recorded as created_by on definition versions.
func actorLabel(p auth.Principal) string {
	if p.Email != "" {
		return p.Email
	}
	return p.Name
}

// sourceParam parses ?source=; "seed" is reserved for the Go seeder.
func sourceParam(r *http.Request) (definitions.Source, error) {
	q := r.URL.Query().Get("source")
	if q == "" {
		return definitions.SourceAPI, nil
	}
	src, ok := definitions.ParseSource(q)
	if !ok || src == definitions.SourceSeed {
		return "", &APIError{Status: http.StatusBadRequest, Code: "bad_request", Message: "source must be one of cli, ui, api"}
	}
	return src, nil
}

func versionParam(r *http.Request) (int, error) {
	n, err := strconv.Atoi(chi.URLParam(r, "version"))
	if err != nil || n < 1 {
		return 0, errNotFound
	}
	return n, nil
}

func slugMismatch(bodySlug, urlSlug string) error {
	return &definition.ValidationError{Problems: []definition.Problem{{
		Path: "slug", Message: fmt.Sprintf("slug %q does not match URL slug %q", bodySlug, urlSlug),
	}}}
}

func findItem(items []definitions.ApplyItem, kind, slug string) definitions.ApplyItem {
	for _, it := range items {
		if it.Kind == kind && it.Slug == slug {
			return it
		}
	}
	return definitions.ApplyItem{Kind: kind, Slug: slug}
}

// ---- forms ----

type formSummaryJSON struct {
	Slug            string    `json:"slug"`
	Title           string    `json:"title"`
	Workflow        *string   `json:"workflow"`
	Public          bool      `json:"public"`
	Version         int       `json:"version"`
	Source          string    `json:"source"`
	UpdatedAt       time.Time `json:"updatedAt"`
	SubmissionCount int       `json:"submissionCount"`
}

func (h *definitionHandlers) listForms(w http.ResponseWriter, r *http.Request) {
	forms, err := h.d.Defs.ListForms(r.Context(), h.d.OrgID)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	counts, err := h.d.Subs.CountByForm(r.Context(), h.d.OrgID)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	items := make([]formSummaryJSON, 0, len(forms))
	for _, f := range forms {
		items = append(items, formSummaryJSON{
			Slug: f.Slug, Title: f.Definition.Title, Workflow: strOrNil(f.Definition.Workflow),
			Public: f.Definition.Settings.Public, Version: f.Current.Version, Source: string(f.Current.Source),
			UpdatedAt: f.UpdatedAt.UTC(), SubmissionCount: counts[f.ID],
		})
	}
	WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *definitionHandlers) formJSON(r *http.Request, rec definitions.FormRecord) (formRecordJSON, error) {
	out := formRecordJSON{Slug: rec.Slug, Version: rec.Current.Version, Source: string(rec.Current.Source), UpdatedAt: rec.UpdatedAt.UTC(), Definition: rec.Definition}
	if rec.WorkflowVersionID != nil {
		wf, err := h.d.Defs.GetWorkflowVersion(r.Context(), *rec.WorkflowVersionID)
		if err != nil {
			return formRecordJSON{}, err
		}
		v := wf.Current.Version
		out.WorkflowVersion = &v
	}
	return out, nil
}

func (h *definitionHandlers) writeForm(w http.ResponseWriter, r *http.Request, rec definitions.FormRecord, err error) {
	if err != nil {
		WriteError(w, r, err)
		return
	}
	out, err := h.formJSON(r, rec)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"form": out})
}

func (h *definitionHandlers) getForm(w http.ResponseWriter, r *http.Request) {
	rec, err := h.d.Defs.GetForm(r.Context(), h.d.OrgID, chi.URLParam(r, "slug"))
	h.writeForm(w, r, rec, err)
}

func (h *definitionHandlers) putForm(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	src, err := sourceParam(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	raw, err := readBody(w, r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	f, err := definition.ParseForm(raw)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	if f.Slug != slug {
		WriteError(w, r, slugMismatch(f.Slug, slug))
		return
	}
	res, err := h.d.Defs.Apply(r.Context(), h.d.OrgID, definitions.ApplyInput{
		Forms: []definition.Form{f}, Source: src, Actor: actorLabel(MustPrincipal(r)),
	})
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"item": findItem(res.Items, "form", slug)})
}

func (h *definitionHandlers) formVersions(w http.ResponseWriter, r *http.Request) {
	vs, err := h.d.Defs.FormVersions(r.Context(), h.d.OrgID, chi.URLParam(r, "slug"))
	if err != nil {
		WriteError(w, r, err)
		return
	}
	writeVersions(w, vs)
}

func (h *definitionHandlers) formVersion(w http.ResponseWriter, r *http.Request) {
	n, err := versionParam(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	rec, err := h.d.Defs.FormVersion(r.Context(), h.d.OrgID, chi.URLParam(r, "slug"), n)
	h.writeForm(w, r, rec, err)
}

func writeVersions(w http.ResponseWriter, vs []definitions.VersionInfo) {
	items := make([]versionJSON, 0, len(vs))
	for _, v := range vs {
		items = append(items, toVersionJSON(v))
	}
	WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

// ---- workflows ----

type workflowSummaryJSON struct {
	Slug       string    `json:"slug"`
	Title      string    `json:"title"`
	Version    int       `json:"version"`
	Source     string    `json:"source"`
	UpdatedAt  time.Time `json:"updatedAt"`
	StateCount int       `json:"stateCount"`
}

func (h *definitionHandlers) listWorkflows(w http.ResponseWriter, r *http.Request) {
	wfs, err := h.d.Defs.ListWorkflows(r.Context(), h.d.OrgID)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	items := make([]workflowSummaryJSON, 0, len(wfs))
	for _, wf := range wfs {
		items = append(items, workflowSummaryJSON{
			Slug: wf.Slug, Title: wf.Definition.Title, Version: wf.Current.Version, Source: string(wf.Current.Source),
			UpdatedAt: wf.UpdatedAt.UTC(), StateCount: len(wf.Definition.States),
		})
	}
	WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func writeWorkflow(w http.ResponseWriter, r *http.Request, rec definitions.WorkflowRecord, err error) {
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"workflow": toWorkflowRecordJSON(rec)})
}

func (h *definitionHandlers) getWorkflow(w http.ResponseWriter, r *http.Request) {
	rec, err := h.d.Defs.GetWorkflow(r.Context(), h.d.OrgID, chi.URLParam(r, "slug"))
	writeWorkflow(w, r, rec, err)
}

func (h *definitionHandlers) putWorkflow(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	src, err := sourceParam(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	raw, err := readBody(w, r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	wf, err := definition.ParseWorkflow(raw)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	if wf.Slug != slug {
		WriteError(w, r, slugMismatch(wf.Slug, slug))
		return
	}
	res, err := h.d.Defs.Apply(r.Context(), h.d.OrgID, definitions.ApplyInput{
		Workflows: []definition.Workflow{wf}, Source: src, Actor: actorLabel(MustPrincipal(r)),
	})
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"item": findItem(res.Items, "workflow", slug)})
}

func (h *definitionHandlers) workflowVersions(w http.ResponseWriter, r *http.Request) {
	vs, err := h.d.Defs.WorkflowVersions(r.Context(), h.d.OrgID, chi.URLParam(r, "slug"))
	if err != nil {
		WriteError(w, r, err)
		return
	}
	writeVersions(w, vs)
}

func (h *definitionHandlers) workflowVersion(w http.ResponseWriter, r *http.Request) {
	n, err := versionParam(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	rec, err := h.d.Defs.WorkflowVersion(r.Context(), h.d.OrgID, chi.URLParam(r, "slug"), n)
	writeWorkflow(w, r, rec, err)
}

// ---- bundles ----

type bundleRequest struct {
	Forms     []json.RawMessage `json:"forms"`
	Workflows []json.RawMessage `json:"workflows"`
}

// parseBundle parses every document, collecting problems prefixed with the
// document's position ("forms[1].fields[0].key").
func parseBundle(r *http.Request) ([]definition.Form, []definition.Workflow, error) {
	var req bundleRequest
	if err := DecodeJSON(r, &req); err != nil {
		return nil, nil, err
	}
	var problems []definition.Problem
	wfs := make([]definition.Workflow, 0, len(req.Workflows))
	for i, raw := range req.Workflows {
		wf, err := definition.ParseWorkflow(raw)
		if err != nil {
			problems = append(problems, definitions.PrefixProblems(fmt.Sprintf("workflows[%d]", i), err)...)
			continue
		}
		wfs = append(wfs, wf)
	}
	forms := make([]definition.Form, 0, len(req.Forms))
	for i, raw := range req.Forms {
		f, err := definition.ParseForm(raw)
		if err != nil {
			problems = append(problems, definitions.PrefixProblems(fmt.Sprintf("forms[%d]", i), err)...)
			continue
		}
		forms = append(forms, f)
	}
	if len(problems) > 0 {
		return nil, nil, &definition.ValidationError{Problems: problems}
	}
	return forms, wfs, nil
}

func (h *definitionHandlers) validate(w http.ResponseWriter, r *http.Request) {
	forms, wfs, err := parseBundle(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	// A dry-run apply runs the same bundle validation (incl. existing workflows) and rolls back.
	if _, err := h.d.Defs.Apply(r.Context(), h.d.OrgID, definitions.ApplyInput{Forms: forms, Workflows: wfs, DryRun: true}); err != nil {
		WriteError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"valid": true})
}

func (h *definitionHandlers) apply(w http.ResponseWriter, r *http.Request) {
	src, err := sourceParam(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	forms, wfs, err := parseBundle(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	res, err := h.d.Defs.Apply(r.Context(), h.d.OrgID, definitions.ApplyInput{
		Forms: forms, Workflows: wfs, Source: src, Actor: actorLabel(MustPrincipal(r)),
		DryRun: r.URL.Query().Get("dryRun") == "true",
	})
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"items": res.Items})
}

func (h *definitionHandlers) export(w http.ResponseWriter, r *http.Request) {
	forms, wfs, err := h.d.Defs.Export(r.Context(), h.d.OrgID)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"forms": forms, "workflows": wfs})
}
