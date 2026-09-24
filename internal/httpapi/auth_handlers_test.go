package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openforms/openforms/internal/testutil"
)

type principalResp struct {
	Principal struct {
		Kind  string   `json:"kind"`
		ID    string   `json:"id"`
		Name  string   `json:"name"`
		Email string   `json:"email"`
		Roles []string `json:"roles"`
	} `json:"principal"`
}

func withCookie(req *http.Request, c *http.Cookie) *http.Request {
	req.AddCookie(c)
	return req
}

func login(t *testing.T, h http.Handler, email, password string) *httptest.ResponseRecorder {
	t.Helper()
	return testutil.Do(t, h, http.MethodPost, "/api/v1/auth/login", "", map[string]string{"email": email, "password": password})
}

func sessionFrom(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == "of_session" {
			return c
		}
	}
	t.Fatalf("no of_session cookie in %v", rec.Result().Cookies())
	return nil
}

func TestLoginMeLogout(t *testing.T) {
	env, h := newRouter(t)
	env.User(t, "ada@example.com", "reviewer")

	rec := login(t, h, "ADA@example.com", "password123")
	if rec.Code != 200 {
		t.Fatalf("login: %d %s", rec.Code, rec.Body.String())
	}
	body := testutil.Decode[map[string]map[string]any](t, rec)
	if body["user"]["email"] != "ada@example.com" || body["user"]["createdAt"] == nil {
		t.Fatalf("login body = %v", body)
	}
	if strings.Contains(rec.Body.String(), "password") {
		t.Fatal("login response must not include password material")
	}
	c := sessionFrom(t, rec)
	if !c.HttpOnly || c.Path != "/" || c.SameSite != http.SameSiteLaxMode || c.MaxAge != 30*24*3600 {
		t.Fatalf("cookie attributes = %+v", c)
	}

	me := httptest.NewRecorder()
	h.ServeHTTP(me, withCookie(httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil), c))
	if me.Code != 200 {
		t.Fatalf("me: %d %s", me.Code, me.Body.String())
	}
	p := testutil.Decode[principalResp](t, me).Principal
	if p.Kind != "user" || p.Email != "ada@example.com" || len(p.Roles) != 1 || p.Roles[0] != "reviewer" {
		t.Fatalf("principal = %+v", p)
	}

	out := httptest.NewRecorder()
	h.ServeHTTP(out, withCookie(httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil), c))
	if out.Code != 204 {
		t.Fatalf("logout: %d", out.Code)
	}
	if cleared := sessionFrom(t, out); cleared.MaxAge >= 0 || cleared.Value != "" {
		t.Fatalf("logout must expire the cookie, got %+v", cleared)
	}

	again := httptest.NewRecorder()
	h.ServeHTTP(again, withCookie(httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil), c))
	if again.Code != 401 || testutil.ErrorCode(t, again) != "unauthenticated" {
		t.Fatalf("me after logout: %d %s", again.Code, again.Body.String())
	}
}

func TestLoginFailures(t *testing.T) {
	env, h := newRouter(t)
	env.User(t, "ada@example.com")
	rec := login(t, h, "ada@example.com", "wrong-password")
	if rec.Code != 401 || testutil.ErrorCode(t, rec) != "invalid_credentials" {
		t.Fatalf("wrong password: %d %s", rec.Code, rec.Body.String())
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Fatal("failed login must not set cookies")
	}
	rec = testutil.Do(t, h, http.MethodPost, "/api/v1/auth/login", "", `{"email":`)
	if rec.Code != 400 || testutil.ErrorCode(t, rec) != "bad_request" {
		t.Fatalf("malformed: %d %s", rec.Code, rec.Body.String())
	}
}

func TestMeWithAPIKey(t *testing.T) {
	env, h := newRouter(t)
	key := env.APIKey(t, "admin")
	rec := testutil.Do(t, h, http.MethodGet, "/api/v1/auth/me", key, nil)
	if rec.Code != 200 {
		t.Fatalf("me: %d %s", rec.Code, rec.Body.String())
	}
	p := testutil.Decode[principalResp](t, rec).Principal
	if p.Kind != "api_key" || p.Roles[0] != "admin" || p.Email != "" {
		t.Fatalf("principal = %+v", p)
	}
	if rec := testutil.Do(t, h, http.MethodGet, "/api/v1/auth/me", "", nil); rec.Code != 401 {
		t.Fatalf("anonymous me: %d", rec.Code)
	}
}
