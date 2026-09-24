package httpapi_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/openforms/openforms/internal/testutil"
)

type createKeyResp struct {
	APIKey struct {
		ID        string   `json:"id"`
		Name      string   `json:"name"`
		Prefix    string   `json:"prefix"`
		Roles     []string `json:"roles"`
		RevokedAt *string  `json:"revokedAt"`
	} `json:"apiKey"`
	Key string `json:"key"`
}

func TestAPIKeysLifecycle(t *testing.T) {
	env, h := newRouter(t)
	admin := env.APIKey(t, "admin")

	rec := testutil.Do(t, h, http.MethodPost, "/api/v1/api-keys", admin, map[string]any{"name": "ci", "roles": []string{"reviewer"}})
	if rec.Code != 201 {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	created := testutil.Decode[createKeyResp](t, rec)
	if !strings.HasPrefix(created.Key, "ofk_") || created.APIKey.Prefix != created.Key[:8] || created.APIKey.Roles[0] != "reviewer" {
		t.Fatalf("created = %+v", created)
	}

	// the new key authenticates
	if rec := testutil.Do(t, h, http.MethodGet, "/api/v1/auth/me", created.Key, nil); rec.Code != 200 {
		t.Fatalf("me with new key: %d", rec.Code)
	}

	list := testutil.Decode[struct {
		Items []map[string]any `json:"items"`
	}](t, testutil.Do(t, h, http.MethodGet, "/api/v1/api-keys", admin, nil))
	if len(list.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(list.Items))
	}
	for _, it := range list.Items {
		if _, leaked := it["key"]; leaked {
			t.Fatal("list must never include plaintext keys")
		}
	}

	if rec := testutil.Do(t, h, http.MethodDelete, "/api/v1/api-keys/"+created.APIKey.ID, admin, nil); rec.Code != 204 {
		t.Fatalf("revoke: %d %s", rec.Code, rec.Body.String())
	}
	if rec := testutil.Do(t, h, http.MethodGet, "/api/v1/auth/me", created.Key, nil); rec.Code != 401 {
		t.Fatalf("revoked key must be 401, got %d", rec.Code)
	}
	if rec := testutil.Do(t, h, http.MethodPost, "/api/v1/api-keys", admin, map[string]any{"name": ""}); rec.Code != 422 {
		t.Fatalf("invalid create: %d", rec.Code)
	}
	if rec := testutil.Do(t, h, http.MethodGet, "/api/v1/api-keys", created.Key, nil); rec.Code != 401 {
		t.Fatalf("revoked key listing: %d", rec.Code)
	}
}

func TestAPIKeysRequireAdmin(t *testing.T) {
	env, h := newRouter(t)
	if rec := testutil.Do(t, h, http.MethodPost, "/api/v1/api-keys", env.APIKey(t, "reviewer"), map[string]any{"name": "x"}); rec.Code != 403 {
		t.Fatalf("reviewer: %d", rec.Code)
	}
}
