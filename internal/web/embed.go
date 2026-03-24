package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static
var staticFiles embed.FS

// Handler returns an http.Handler that serves the embedded web UI.
// Requests to "/" serve index.html; all other static assets are served from the static/ directory.
func Handler() http.Handler {
	sub, _ := fs.Sub(staticFiles, "static")
	return http.FileServer(http.FS(sub))
}
