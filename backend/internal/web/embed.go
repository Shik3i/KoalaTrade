package web

import "embed"

// distFS holds the built frontend. The committed dist/ contains only a
// placeholder shell so the package compiles for local `go build`/`go test`;
// the Docker image copies the real Vite output over it before compiling.
//
//go:embed all:dist
var distFS embed.FS
