// Package webui serves the embedded single-page-app builds (admin, hosted, demo, embed.js).
package webui

import (
	"embed"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var dist embed.FS

// Handler serves the embedded web builds from dist/.
func Handler() http.Handler {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // dist/ is embedded at build time; this cannot fail
	}
	return newHandler(sub)
}

func newHandler(fsys fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		p := r.URL.Path
		switch {
		case p == "/":
			http.Redirect(w, r, "/admin", http.StatusFound)
		case strings.HasPrefix(p, "/_app/"):
			serveStatic(w, r, fsys, strings.TrimPrefix(p, "/_app/"))
		case p == "/embed.js":
			serveStatic(w, r, fsys, "embed/embed.js")
		case p == "/admin" || strings.HasPrefix(p, "/admin/"):
			serveIndex(w, r, fsys, "admin")
		case strings.HasPrefix(p, "/f/") || strings.HasPrefix(p, "/s/"):
			serveIndex(w, r, fsys, "hosted")
		case p == "/demo" || strings.HasPrefix(p, "/demo/"):
			serveIndex(w, r, fsys, "demo")
		default:
			http.NotFound(w, r)
		}
	})
}

func serveStatic(w http.ResponseWriter, r *http.Request, fsys fs.FS, name string) {
	name = strings.TrimPrefix(path.Clean("/"+name), "/")
	if name == "" {
		http.NotFound(w, r)
		return
	}
	info, err := fs.Stat(fsys, name)
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	if strings.Contains(name, "/assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	http.ServeFileFS(w, r, fsys, name)
}

func serveIndex(w http.ResponseWriter, r *http.Request, fsys fs.FS, app string) {
	data, err := fs.ReadFile(fsys, app+"/index.html")
	if err != nil {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		io.WriteString(w, "web UI not built; run `make web`\n") //nolint:errcheck
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		w.Write(data) //nolint:errcheck
	}
}
