// Package web embeds the built single-page frontend and serves it from the same
// origin as the API. The production Docker build overwrites the placeholder
// dist/ directory with the real Vite output before compiling.
package web

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// frontendCSP is the Content-Security-Policy for HTML/asset responses. It is
// looser than the API's `default-src 'none'` because the SPA needs to load its
// own scripts, styles and data-URI images, but still same-origin only. This
// mirrors the policy the previous standalone nginx frontend served.
const frontendCSP = "default-src 'self'; base-uri 'self'; connect-src 'self'; " +
	"font-src 'self'; form-action 'self'; frame-ancestors 'none'; frame-src 'none'; " +
	"img-src 'self' data:; manifest-src 'self'; object-src 'none'; " +
	"script-src 'self'; style-src 'self'; worker-src 'self'"

// Handler serves the embedded SPA: real assets when the path exists, otherwise
// the index.html shell so client-side routes resolve. Mount it as the router's
// catch-all after the /api and /healthz routes.
func Handler() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		// distFS always contains a dist/ directory (placeholder or real build),
		// so fs.Sub cannot fail at runtime; treat it as a programming error.
		panic(err)
	}

	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", frontendCSP)

		reqPath := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if reqPath == "" {
			reqPath = "index.html"
		}

		// Unknown path (a client-side route, not a file): serve the SPA shell.
		if _, statErr := fs.Stat(sub, reqPath); statErr != nil {
			// The shell must always be revalidated so a new deploy's index.html
			// (which references the new hashed asset names) is picked up.
			w.Header().Set("Cache-Control", "no-cache")
			shell := r.Clone(r.Context())
			shell.URL.Path = "/"
			fileServer.ServeHTTP(w, shell)
			return
		}

		// Vite emits content-hashed filenames under assets/, so they can be
		// cached forever; index.html and other root files stay revalidated.
		if strings.HasPrefix(reqPath, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}

		fileServer.ServeHTTP(w, r)
	})
}
