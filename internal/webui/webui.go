// Package webui serves the embedded single-page application.
package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var dist embed.FS

// Handler serves the SPA with history-API fallback.
func Handler() http.Handler {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(sub))
	index, err := fs.ReadFile(sub, "index.html")
	if err != nil || len(index) == 0 {
		index = []byte(placeholder)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "" {
			p = "index.html"
		}
		if f, err := sub.Open(p); err == nil {
			f.Close()
			if strings.HasPrefix(p, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				w.Header().Set("Cache-Control", "no-cache")
			}
			fileServer.ServeHTTP(w, r)
			return
		}
		switch {
		case strings.HasPrefix(p, "api/") || strings.HasPrefix(p, "agent/"):
			// An unknown API route must not look like success (the app page with 200).
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error":"not found"}`))
			return
		case strings.HasPrefix(p, "assets/"):
			// A missing hashed asset (an old page after a deploy) must fail as a 404, not
			// load the HTML page as a script.
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(index)
	})
}

const placeholder = `<!doctype html><html><head><meta charset="utf-8"><title>VibeMDM</title></head>
<body style="font-family:system-ui;padding:3rem;color:#333"><h1>VibeMDM</h1>
<p>The web UI has not been built into this binary. Run <code>make web build</code> (or use the Docker image) to include it.</p>
<p>The API is available at <code>/api/v1/</code>.</p></body></html>`
