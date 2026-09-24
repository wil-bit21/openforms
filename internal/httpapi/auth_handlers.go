package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/openforms/openforms/internal/auth"
)

func mountAuth(r chi.Router, d Deps) {
	r.Post("/auth/login", func(w http.ResponseWriter, req *http.Request) {
		var body struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if err := DecodeJSON(req, &body); err != nil {
			WriteError(w, req, err)
			return
		}
		token, u, err := d.Auth.Login(req.Context(), body.Email, body.Password)
		if err != nil {
			WriteError(w, req, err)
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:     sessionCookie,
			Value:    token,
			Path:     "/",
			MaxAge:   int(auth.SessionTTL.Seconds()),
			HttpOnly: true,
			Secure:   d.Config.CookieSecure,
			SameSite: http.SameSiteLaxMode,
		})
		WriteJSON(w, http.StatusOK, map[string]any{"user": toUserJSON(u)})
	})

	r.With(RequireAuth).Post("/auth/logout", func(w http.ResponseWriter, req *http.Request) {
		if c, err := req.Cookie(sessionCookie); err == nil && c.Value != "" {
			if err := d.Auth.Logout(req.Context(), c.Value); err != nil {
				WriteError(w, req, err)
				return
			}
		}
		http.SetCookie(w, &http.Cookie{
			Name: sessionCookie, Value: "", Path: "/", MaxAge: -1,
			HttpOnly: true, Secure: d.Config.CookieSecure, SameSite: http.SameSiteLaxMode,
		})
		w.WriteHeader(http.StatusNoContent)
	})

	r.With(RequireAuth).Get("/auth/me", func(w http.ResponseWriter, req *http.Request) {
		WriteJSON(w, http.StatusOK, map[string]any{"principal": toPrincipalJSON(MustPrincipal(req))})
	})
}
