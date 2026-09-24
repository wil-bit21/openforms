package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/openforms/openforms/internal/testutil"
)

type userResp struct {
	User struct {
		ID    string   `json:"id"`
		Email string   `json:"email"`
		Name  string   `json:"name"`
		Roles []string `json:"roles"`
	} `json:"user"`
}

func TestUsersCRUD(t *testing.T) {
	env, h := newRouter(t)
	admin := env.APIKey(t, "admin")

	rec := testutil.Do(t, h, http.MethodPost, "/api/v1/users", admin,
		map[string]any{"email": "Ada@Example.com", "name": "Ada", "password": "password123", "roles": []string{"reviewer"}})
	if rec.Code != 201 {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	created := testutil.Decode[userResp](t, rec).User
	if created.Email != "ada@example.com" || created.Roles[0] != "reviewer" {
		t.Fatalf("created = %+v", created)
	}

	rec = testutil.Do(t, h, http.MethodPost, "/api/v1/users", admin,
		map[string]any{"email": "ada@example.com", "name": "Dup", "password": "password123"})
	if rec.Code != 409 || testutil.ErrorCode(t, rec) != "email_taken" {
		t.Fatalf("duplicate: %d %s", rec.Code, rec.Body.String())
	}

	rec = testutil.Do(t, h, http.MethodPost, "/api/v1/users", admin,
		map[string]any{"email": "nope", "name": "", "password": "x"})
	if rec.Code != 422 || testutil.ErrorCode(t, rec) != "validation_failed" {
		t.Fatalf("invalid: %d %s", rec.Code, rec.Body.String())
	}

	list := testutil.Decode[struct {
		Items []map[string]any `json:"items"`
	}](t, testutil.Do(t, h, http.MethodGet, "/api/v1/users", admin, nil))
	if len(list.Items) != 1 {
		t.Fatalf("list = %+v", list)
	}

	rec = testutil.Do(t, h, http.MethodPatch, "/api/v1/users/"+created.ID, admin,
		map[string]any{"name": "Ada Lovelace", "roles": []string{"reviewer", "hiring-manager"}})
	if rec.Code != 200 {
		t.Fatalf("patch: %d %s", rec.Code, rec.Body.String())
	}
	patched := testutil.Decode[userResp](t, rec).User
	if patched.Name != "Ada Lovelace" || len(patched.Roles) != 2 {
		t.Fatalf("patched = %+v", patched)
	}

	if rec := testutil.Do(t, h, http.MethodDelete, "/api/v1/users/"+created.ID, admin, nil); rec.Code != 204 {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body.String())
	}
	if rec := testutil.Do(t, h, http.MethodDelete, "/api/v1/users/"+created.ID, admin, nil); rec.Code != 404 {
		t.Fatalf("second delete: %d", rec.Code)
	}
	if rec := testutil.Do(t, h, http.MethodPatch, "/api/v1/users/not-a-uuid", admin, map[string]any{}); rec.Code != 404 {
		t.Fatalf("bad id: %d", rec.Code)
	}
}

func TestUsersRequireAdmin(t *testing.T) {
	env, h := newRouter(t)
	reviewer := env.APIKey(t, "reviewer")
	if rec := testutil.Do(t, h, http.MethodGet, "/api/v1/users", reviewer, nil); rec.Code != 403 || testutil.ErrorCode(t, rec) != "forbidden" {
		t.Fatalf("reviewer: %d %s", rec.Code, rec.Body.String())
	}
	if rec := testutil.Do(t, h, http.MethodGet, "/api/v1/users", "", nil); rec.Code != 401 {
		t.Fatalf("anonymous: %d", rec.Code)
	}
}

func TestLastAdminIsProtectedOverHTTP(t *testing.T) {
	env, h := newRouter(t)
	key := env.APIKey(t, "admin")
	root := env.User(t, "root@example.com", "admin")

	rec := testutil.Do(t, h, http.MethodPatch, "/api/v1/users/"+root.ID.String(), key, map[string]any{"roles": []string{}})
	if rec.Code != 422 || testutil.ErrorCode(t, rec) != "validation_failed" {
		t.Fatalf("demote last admin: %d %s", rec.Code, rec.Body.String())
	}
	rec = testutil.Do(t, h, http.MethodDelete, "/api/v1/users/"+root.ID.String(), key, nil)
	if rec.Code != 422 {
		t.Fatalf("delete last admin: %d %s", rec.Code, rec.Body.String())
	}

	env.User(t, "second@example.com", "admin")
	rec = testutil.Do(t, h, http.MethodPatch, "/api/v1/users/"+root.ID.String(), key, map[string]any{"roles": []string{}})
	if rec.Code != 200 {
		t.Fatalf("demote with second admin: %d %s", rec.Code, rec.Body.String())
	}
}
