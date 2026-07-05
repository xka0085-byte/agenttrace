// Package web provides embedded static file serving for the React SPA.
package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed dist/*
var dist embed.FS

// Handler returns an http.Handler that serves the embedded React build.
// Returns nil if the dist directory is empty (happens during development).
func Handler() http.Handler {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		// dist not built yet — return nil so caller can handle gracefully
		return nil
	}
	return http.FileServer(http.FS(sub))
}
