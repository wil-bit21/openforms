package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/openforms/openforms/internal/jobs"
)

type jobJSON struct {
	ID          int64           `json:"id"`
	Kind        string          `json:"kind"`
	Status      string          `json:"status"`
	Attempts    int             `json:"attempts"`
	MaxAttempts int             `json:"maxAttempts"`
	RunAt       time.Time       `json:"runAt"`
	LastError   string          `json:"lastError"`
	Payload     json.RawMessage `json:"payload"`
	CreatedAt   time.Time       `json:"createdAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
}

func toJobJSON(j jobs.Job) jobJSON {
	return jobJSON{
		ID: j.ID, Kind: j.Kind, Status: j.Status, Attempts: j.Attempts, MaxAttempts: j.MaxAttempts,
		RunAt: j.RunAt.UTC(), LastError: j.LastError, Payload: j.Payload,
		CreatedAt: j.CreatedAt.UTC(), UpdatedAt: j.UpdatedAt.UTC(),
	}
}

var validJobStatus = map[string]bool{
	"": true, jobs.StatusPending: true, jobs.StatusRunning: true, jobs.StatusDone: true, jobs.StatusFailed: true,
}

func mountJobs(r chi.Router, d Deps) {
	r.With(RequireAdmin).Get("/jobs", func(w http.ResponseWriter, r *http.Request) {
		status := r.URL.Query().Get("status")
		if !validJobStatus[status] {
			WriteError(w, r, &APIError{Status: http.StatusBadRequest, Code: "bad_request", Message: "status must be one of pending, running, done, failed"})
			return
		}
		limit := 100
		if s := r.URL.Query().Get("limit"); s != "" {
			n, err := strconv.Atoi(s)
			if err != nil || n < 1 || n > 500 {
				WriteError(w, r, &APIError{Status: http.StatusBadRequest, Code: "bad_request", Message: "limit must be between 1 and 500"})
				return
			}
			limit = n
		}
		list, err := d.Queue.List(r.Context(), d.OrgID, status, limit)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		items := make([]jobJSON, 0, len(list))
		for _, j := range list {
			items = append(items, toJobJSON(j))
		}
		WriteJSON(w, http.StatusOK, map[string]any{"items": items})
	})
	r.With(RequireAdmin).Post("/jobs/{id}/retry", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
		if err != nil {
			WriteError(w, r, jobs.ErrNotFound)
			return
		}
		if err := d.Queue.Retry(r.Context(), d.OrgID, id); err != nil {
			WriteError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
