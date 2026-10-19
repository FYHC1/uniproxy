package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:*.html all:assets
var assets embed.FS

func Handler() http.Handler {
	root, err := fs.Sub(assets, "assets/dist")
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "UniProxy frontend is not built. Run pnpm build:web and rebuild the Go binary.", http.StatusServiceUnavailable)
		})
	}
	if _, err := fs.Stat(root, "index.html"); err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "UniProxy frontend is not built. Run pnpm build:web and rebuild the Go binary.", http.StatusServiceUnavailable)
		})
	}
	files := http.FileServer(http.FS(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		if r.Method != "GET" && r.Method != "HEAD" {
			w.WriteHeader(405)
			return
		}
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "." || name == "" {
			name = "index.html"
		}
		if _, e := fs.Stat(root, name); e != nil {
			if strings.Contains(name, ".") {
				http.NotFound(w, r)
				return
			}
			r.URL.Path = "/"
		}
		if strings.HasPrefix(name, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	})
}
