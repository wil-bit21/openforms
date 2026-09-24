package webui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func builtFS() fstest.MapFS {
	return fstest.MapFS{
		"admin/index.html":        {Data: []byte("<html>admin</html>")},
		"admin/assets/app-123.js": {Data: []byte("console.log('admin')")},
		"admin/favicon.svg":       {Data: []byte("<svg/>")},
		"hosted/index.html":       {Data: []byte("<html>hosted</html>")},
		"demo/index.html":         {Data: []byte("<html>demo</html>")},
		"embed/embed.js":          {Data: []byte("/*embed*/")},
		".gitkeep":                {Data: []byte{}},
	}
}

func get(h http.Handler, method, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func TestSPAFallbacks(t *testing.T) {
	h := newHandler(builtFS())
	cases := map[string]string{
		"/admin":                 "admin",
		"/admin/":                "admin",
		"/admin/submissions/abc": "admin",
		"/f/contact":             "hosted",
		"/s/1234":                "hosted",
		"/demo":                  "demo",
		"/demo/anything":         "demo",
	}
	for path, want := range cases {
		rec := get(h, http.MethodGet, path)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), want) {
			t.Errorf("%s: code=%d body=%q, want 200 containing %q", path, rec.Code, rec.Body.String(), want)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
			t.Errorf("%s: content-type %q", path, ct)
		}
		if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
			t.Errorf("%s: cache-control %q", path, cc)
		}
	}
}

func TestStaticAssets(t *testing.T) {
	h := newHandler(builtFS())
	rec := get(h, http.MethodGet, "/_app/admin/assets/app-123.js")
	if rec.Code != http.StatusOK || rec.Body.String() != "console.log('admin')" {
		t.Fatalf("asset: code=%d body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "javascript") {
		t.Errorf("content-type %q", rec.Header().Get("Content-Type"))
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Errorf("assets cache-control %q", cc)
	}
	rec = get(h, http.MethodGet, "/_app/admin/favicon.svg")
	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "no-cache" {
		t.Errorf("favicon: code=%d cc=%q", rec.Code, rec.Header().Get("Cache-Control"))
	}
}

func TestEmbedScript(t *testing.T) {
	rec := get(newHandler(builtFS()), http.MethodGet, "/embed.js")
	if rec.Code != http.StatusOK || rec.Body.String() != "/*embed*/" {
		t.Fatalf("code=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestNotFoundAndTraversal(t *testing.T) {
	h := newHandler(builtFS())
	for _, p := range []string{"/nope", "/_app/admin/missing.js", "/_app/admin/", "/_app/", "/_app/../../etc/passwd"} {
		if rec := get(h, http.MethodGet, p); rec.Code == http.StatusOK {
			t.Errorf("%s: got 200, want non-200", p)
		}
	}
}

func TestRootRedirectsToAdmin(t *testing.T) {
	rec := get(newHandler(builtFS()), http.MethodGet, "/")
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/admin" {
		t.Fatalf("code=%d location=%q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestMethodNotAllowed(t *testing.T) {
	if rec := get(newHandler(builtFS()), http.MethodPost, "/admin"); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("code=%d, want 405", rec.Code)
	}
}

func TestNotBuiltReturns503(t *testing.T) {
	h := newHandler(fstest.MapFS{".gitkeep": {Data: []byte{}}})
	rec := get(h, http.MethodGet, "/admin")
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "make web") {
		t.Fatalf("code=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestEmbeddedHandlerWorksWithoutBuild(t *testing.T) {
	// The committed dist/ only has .gitkeep: the real Handler must still construct and respond.
	rec := get(Handler(), http.MethodGet, "/nope")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code=%d", rec.Code)
	}
}
