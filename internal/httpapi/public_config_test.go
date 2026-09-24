package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/openforms/openforms/internal/definitions"
	"github.com/openforms/openforms/internal/httpapi"
	"github.com/openforms/openforms/internal/submissions"
	"github.com/openforms/openforms/internal/testutil"
)

func TestPublicConfig(t *testing.T) {
	for _, demo := range []bool{true, false} {
		env := testutil.NewEnv(t)
		cfg := env.Config
		cfg.DemoMode = demo
		defs := definitions.NewStore(env.Pool)
		h := httpapi.NewRouter(httpapi.Deps{
			Config: cfg,
			Auth:   env.Auth,
			Defs:   defs,
			Subs:   submissions.NewService(env.Pool, defs),
			OrgID:  env.OrgID,
		})

		rec := testutil.Do(t, h, http.MethodGet, "/api/v1/public/config", "", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("demo=%v: status %d body %s", demo, rec.Code, rec.Body.String())
		}
		got := testutil.Decode[map[string]bool](t, rec)
		if got["demo"] != demo {
			t.Errorf("demo=%v: body %v", demo, got)
		}
	}
}
