package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Shik3i/KoalaTrade/backend/internal/auth"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

const adminTokenTTL = 12 * time.Hour

// SeedAdmin creates the admin user once from ADMIN_USERNAME/ADMIN_PASSWORD when
// no user exists yet. A blank ADMIN_PASSWORD leaves the admin area disabled.
func (s *Server) SeedAdmin(ctx context.Context, logger *slog.Logger) {
	if s.cfg.AdminPassword == "" {
		logger.Info("admin seeding skipped: ADMIN_PASSWORD not set")
		return
	}

	count, err := s.db.CountUsers(ctx)
	if err != nil {
		logger.Warn("admin seed: count users failed", "error", err)
		return
	}
	if count > 0 {
		return
	}

	hash, err := auth.HashPassword(s.cfg.AdminPassword)
	if err != nil {
		logger.Warn("admin seed: hash failed", "error", err)
		return
	}
	if err := s.db.CreateUser(ctx, uuid.NewString(), s.cfg.AdminUsername, hash); err != nil {
		logger.Warn("admin seed: create user failed", "error", err)
		return
	}
	logger.Info("admin user seeded", "username", s.cfg.AdminUsername)
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request"})
		return
	}

	hash, found, err := s.db.GetUserPasswordHash(r.Context(), strings.TrimSpace(req.Username))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "login failed"})
		return
	}
	if !found || !auth.VerifyPassword(req.Password, hash) {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "invalid credentials"})
		return
	}

	token, expiresAt, err := auth.SignToken(s.authSecret, req.Username, adminTokenTTL)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "login failed"})
		return
	}
	writeJSON(w, http.StatusOK, loginResponse{Token: token, ExpiresAt: expiresAt})
}

func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if token == "" {
			writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing token"})
			return
		}
		if _, err := auth.VerifyToken(s.authSecret, token); err != nil {
			writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "invalid token"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleListMappings(w http.ResponseWriter, r *http.Request) {
	mappings, err := s.db.ListTeamMappings(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "list mappings failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"mappings": mappings})
}

type mappingRequest struct {
	OriginalCode   string `json:"originalCode"`
	PolymarketCode string `json:"polymarketCode"`
}

func (s *Server) handleUpsertMapping(w http.ResponseWriter, r *http.Request) {
	var req mappingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request"})
		return
	}
	if strings.TrimSpace(req.OriginalCode) == "" || strings.TrimSpace(req.PolymarketCode) == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "both codes are required"})
		return
	}
	if err := s.db.UpsertTeamMapping(r.Context(), req.OriginalCode, req.PolymarketCode); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "save mapping failed"})
		return
	}
	s.handleListMappings(w, r)
}

func (s *Server) handleDeleteMapping(w http.ResponseWriter, r *http.Request) {
	if err := s.db.DeleteTeamMapping(r.Context(), chi.URLParam(r, "code")); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "delete mapping failed"})
		return
	}
	s.handleListMappings(w, r)
}

func (s *Server) handleAdminStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"esports":          s.esports.Status(r.Context()),
		"marketDataSource": s.cfg.MarketDataProvider,
	})
}

func (s *Server) handleAdminRefresh(w http.ResponseWriter, r *http.Request) {
	matches, err := s.esports.ForceRefresh(r.Context())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: "refresh failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"refreshed": len(matches)})
}
