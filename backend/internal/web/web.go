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
	"form-action 'self'; frame-ancestors 'none'; img-src 'self' data:; " +
	"manifest-src 'self'; script-src 'self'; style-src 'self'; worker-src 'self'"

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
			shell := r.Clone(r.Context())
			shell.URL.Path = "/"
			fileServer.ServeHTTP(w, shell)
			return
		}

		fileServer.ServeHTTP(w, r)
	})
}
