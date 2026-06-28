package server

import (
	"net/http"
	"time"

	"github.com/Shik3i/KoalaTrade/backend/internal/config"
	"github.com/Shik3i/KoalaTrade/backend/internal/storage"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Server struct {
	cfg config.Config
	db  *storage.SQLite
}

func New(cfg config.Config, db *storage.SQLite) *Server {
	return &Server{cfg: cfg, db: db}
}

func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(securityHeaders)

	r.Get("/healthz", s.handleHealth)

	r.Route("/api", func(r chi.Router) {
		r.Get("/config", s.handleConfig)
	})

	return r
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
		next.ServeHTTP(w, r)
	})
}
