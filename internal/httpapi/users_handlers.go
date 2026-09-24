package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/openforms/openforms/internal/auth"
)

func mountAdminUsers(r chi.Router, d Deps) {
	r.Route("/users", func(r chi.Router) {
		r.Use(RequireAdmin)

		r.Get("/", func(w http.ResponseWriter, req *http.Request) {
			users, err := d.Auth.ListUsers(req.Context(), MustPrincipal(req).OrgID)
			if err != nil {
				WriteError(w, req, err)
				return
			}
			items := make([]userJSON, len(users))
			for i, u := range users {
				items[i] = toUserJSON(u)
			}
			WriteJSON(w, http.StatusOK, map[string]any{"items": items})
		})

		r.Post("/", func(w http.ResponseWriter, req *http.Request) {
			var body struct {
				Email    string   `json:"email"`
				Name     string   `json:"name"`
				Password string   `json:"password"`
				Roles    []string `json:"roles"`
			}
			if err := DecodeJSON(req, &body); err != nil {
				WriteError(w, req, err)
				return
			}
			u, err := d.Auth.CreateUser(req.Context(), MustPrincipal(req).OrgID, body.Email, body.Name, body.Password, body.Roles)
			if err != nil {
				WriteError(w, req, err)
				return
			}
			WriteJSON(w, http.StatusCreated, map[string]any{"user": toUserJSON(u)})
		})

		r.Patch("/{id}", func(w http.ResponseWriter, req *http.Request) {
			id, err := URLParamUUID(req, "id")
			if err != nil {
				WriteError(w, req, err)
				return
			}
			var body struct {
				Name     *string   `json:"name"`
				Password *string   `json:"password"`
				Roles    *[]string `json:"roles"`
			}
			if err := DecodeJSON(req, &body); err != nil {
				WriteError(w, req, err)
				return
			}
			in := auth.UpdateUserInput{Name: body.Name, Password: body.Password}
			if body.Roles != nil {
				in.Roles = nonNil(*body.Roles)
			}
			u, err := d.Auth.UpdateUser(req.Context(), MustPrincipal(req).OrgID, id, in)
			if err != nil {
				WriteError(w, req, err)
				return
			}
			WriteJSON(w, http.StatusOK, map[string]any{"user": toUserJSON(u)})
		})

		r.Delete("/{id}", func(w http.ResponseWriter, req *http.Request) {
			id, err := URLParamUUID(req, "id")
			if err != nil {
				WriteError(w, req, err)
				return
			}
			if err := d.Auth.DeleteUser(req.Context(), MustPrincipal(req).OrgID, id); err != nil {
				WriteError(w, req, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})
	})
}
