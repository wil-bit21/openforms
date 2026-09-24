package httpapi

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/openforms/openforms/internal/definition"
)

const defaultConfirmationMessage = "Thanks! Your response has been recorded."

// mountPublic registers the unauthenticated /public routes.
func mountPublic(r chi.Router, d Deps) {
	h := &publicHandlers{d: d, limiter: newRateLimiter(20, time.Minute, time.Now)}
	r.Get("/public/forms/{slug}", h.getForm)
	r.Post("/public/forms/{slug}/submissions", h.submit)
	r.Get("/public/submissions/{id}", h.status)
}

type publicHandlers struct {
	d       Deps
	limiter *rateLimiter
}

func (h *publicHandlers) getForm(w http.ResponseWriter, r *http.Request) {
	rec, err := h.d.Defs.GetForm(r.Context(), h.d.OrgID, chi.URLParam(r, "slug"))
	if err != nil {
		WriteError(w, r, err)
		return
	}
	if !rec.Definition.Settings.Public {
		WriteError(w, r, errNotFound)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"form": rec.Definition})
}

type publicSubmitRequest struct {
	Data map[string]any `json:"data"`
}

func (h *publicHandlers) submit(w http.ResponseWriter, r *http.Request) {
	if !h.limiter.Allow(clientIP(r)) {
		w.Header().Set("Retry-After", "60")
		WriteError(w, r, &APIError{Status: http.StatusTooManyRequests, Code: "rate_limited", Message: "too many submissions; try again in a minute"})
		return
	}
	var req publicSubmitRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, r, err)
		return
	}
	sub, token, err := h.d.Subs.Create(r.Context(), h.d.OrgID, chi.URLParam(r, "slug"), req.Data, true)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	msg := defaultConfirmationMessage
	if form, err := h.d.Defs.GetFormVersion(r.Context(), sub.FormVersionID); err == nil && form.Definition.Settings.ConfirmationMessage != "" {
		msg = form.Definition.Settings.ConfirmationMessage
	}
	WriteJSON(w, http.StatusCreated, map[string]any{
		"id": sub.ID, "state": sub.State, "stateLabel": sub.StateLabel,
		"receiptToken": token, "confirmationMessage": msg,
	})
}

type publicHistoryJSON struct {
	State string    `json:"state"`
	Label string    `json:"label"`
	At    time.Time `json:"at"`
}

type publicStatusJSON struct {
	ID         string              `json:"id"`
	FormTitle  string              `json:"formTitle"`
	State      string              `json:"state"`
	StateLabel string              `json:"stateLabel"`
	Terminal   bool                `json:"terminal"`
	States     []definition.State  `json:"states"`
	History    []publicHistoryJSON `json:"history"`
	CreatedAt  time.Time           `json:"createdAt"`
}

func (h *publicHandlers) status(w http.ResponseWriter, r *http.Request) {
	id, err := URLParamUUID(r, "id")
	if err != nil {
		WriteError(w, r, err)
		return
	}
	st, err := h.d.Subs.PublicStatus(r.Context(), id, r.URL.Query().Get("token"))
	if err != nil {
		WriteError(w, r, err)
		return
	}
	history := make([]publicHistoryJSON, 0, len(st.History))
	for _, it := range st.History {
		history = append(history, publicHistoryJSON{State: it.State, Label: it.Label, At: it.At.UTC()})
	}
	WriteJSON(w, http.StatusOK, publicStatusJSON{
		ID: st.ID.String(), FormTitle: st.FormTitle, State: st.State, StateLabel: st.StateLabel,
		Terminal: st.Terminal, States: st.States, History: history, CreatedAt: st.CreatedAt.UTC(),
	})
}
