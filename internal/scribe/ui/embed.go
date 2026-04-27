// Package ui serves the embedded Vue SPA at "/" via go:embed. Built assets
// live under frontend/dist; run `make ui-build` to populate them before
// `go build`.
package ui

import (
	"embed"
	"io"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:frontend/dist
var distFS embed.FS

// dist returns the embedded dist/ subtree as an fs.FS rooted at the SPA root.
func dist() fs.FS {
	sub, err := fs.Sub(distFS, "frontend/dist")
	if err != nil {
		// Compile-time invariant: fs.Sub on a known prefix can't fail in practice.
		panic(err)
	}
	return sub
}

// Handler returns an http.Handler that serves the SPA. Paths that resolve to
// a real embedded file (index.html, /assets/*) are served as-is; any other
// path returns index.html so that client-side routing works (SPA fallback).
func Handler() http.Handler {
	root := dist()
	fileServer := http.FileServer(http.FS(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || strings.HasPrefix(r.URL.Path, "/assets/") {
			fileServer.ServeHTTP(w, r)
			return
		}
		// SPA fallback: any unknown path hits index.html so the Vue router
		// (when we add one — v1 has none) can take over.
		f, err := root.Open("index.html")
		if err != nil {
			http.Error(w, "index.html not found", http.StatusNotFound)
			return
		}
		defer f.Close()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = io.Copy(w, f)
	})
}
