package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/Shik3i/KoalaTrade/backend/internal/auth"
	"github.com/Shik3i/KoalaTrade/backend/internal/storage"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

const (
	adminTokenTTL       = 12 * time.Hour
	sessionTTL          = 7 * 24 * time.Hour
	sessionCookieName   = "koala_session"
	registrationOpenKey = "registration_open"
)

type sessionUser struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
}

type contextKey string

const userContextKey contextKey = "user"

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
		if user, found, err := s.db.UserByUsername(ctx, s.cfg.AdminUsername); err == nil && found && user.Role != storage.RoleAdmin {
			if err := s.db.UpdateUserRole(ctx, user.ID, storage.RoleAdmin); err != nil {
				logger.Warn("admin seed: promote existing admin failed", "error", err)
			}
		}
		return
	}

	hash, err := auth.HashPassword(s.cfg.AdminPassword)
	if err != nil {
		logger.Warn("admin seed: hash failed", "error", err)
		return
	}
	if err := s.db.CreateUser(ctx, uuid.NewString(), s.cfg.AdminUsername, hash, storage.RoleAdmin); err != nil {
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
	Token     string      `json:"token,omitempty"`
	ExpiresAt time.Time   `json:"expiresAt"`
	User      sessionUser `json:"user"`
}

type registerRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type meResponse struct {
	User sessionUser `json:"user"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request"})
		return
	}

	username := normalizeUsername(req.Username)
	if lockedUntil, locked := s.loginLocked(username, r); locked {
		writeJSON(w, http.StatusTooManyRequests, errorResponse{Error: "too many attempts; try again after " + lockedUntil.Format(time.RFC3339)})
		return
	}

	user, found, err := s.db.UserByUsername(r.Context(), username)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "login failed"})
		return
	}
	if !found || user.Disabled || !auth.VerifyPassword(req.Password, user.PasswordHash) {
		s.recordLoginFailure(username, r)
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "invalid credentials"})
		return
	}
	s.clearLoginFailures(username, r)
	s.writeLoginResponse(w, user)
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if !s.registrationOpen(r.Context()) {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "registration is closed"})
		return
	}

	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request"})
		return
	}
	username := normalizeUsername(req.Username)
	if !validUsername(username) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "username must be 3-32 letters, numbers, dashes, underscores, or dots"})
		return
	}
	if err := validPassword(req.Password); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	if _, found, err := s.db.UserByUsername(r.Context(), username); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "registration failed"})
		return
	} else if found {
		writeJSON(w, http.StatusConflict, errorResponse{Error: "username already exists"})
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "registration failed"})
		return
	}
	if err := s.db.CreateUser(r.Context(), uuid.NewString(), username, hash, storage.RoleUser); err != nil {
		writeJSON(w, http.StatusConflict, errorResponse{Error: "username already exists"})
		return
	}
	user, found, err := s.db.UserByUsername(r.Context(), username)
	if err != nil || !found {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "registration failed"})
		return
	}
	s.writeLoginResponse(w, user)
}

func (s *Server) handleLogout(w http.ResponseWriter, _ *http.Request) {
	s.clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	user, ok := s.currentUser(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "not authenticated"})
		return
	}
	writeJSON(w, http.StatusOK, meResponse{User: user})
}

func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r)
		if token == "" {
			if cookie, err := r.Cookie(sessionCookieName); err == nil {
				token = cookie.Value
			}
		}
		if token == "" {
			writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing token"})
			return
		}
		claims, err := auth.VerifySessionToken(s.authSecret, token)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "invalid token"})
			return
		}
		user, ok := s.userFromClaims(r.Context(), claims)
		if !ok || user.Role != storage.RoleAdmin {
			writeJSON(w, http.StatusForbidden, errorResponse{Error: "admin role required"})
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userContextKey, user)))
	})
}

func (s *Server) currentUser(r *http.Request) (sessionUser, bool) {
	if user, ok := r.Context().Value(userContextKey).(sessionUser); ok {
		return user, true
	}
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return sessionUser{}, false
	}
	claims, err := auth.VerifySessionToken(s.authSecret, cookie.Value)
	if err != nil {
		return sessionUser{}, false
	}
	return s.userFromClaims(r.Context(), claims)
}

func (s *Server) userFromClaims(ctx context.Context, claims auth.Claims) (sessionUser, bool) {
	if claims.Subject == "" {
		return sessionUser{}, false
	}
	user, found, err := s.db.UserByID(ctx, claims.Subject)
	if err != nil || !found || user.Disabled {
		return sessionUser{}, false
	}
	return publicUser(user), true
}

func (s *Server) setSessionCookie(w http.ResponseWriter, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		MaxAge:   int(time.Until(expires).Seconds()),
		HttpOnly: true,
		Secure:   s.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) writeLoginResponse(w http.ResponseWriter, user storage.UserProfile) {
	token, expiresAt, err := auth.SignSessionToken(s.authSecret, user.ID, user.Username, user.Role, sessionTTL)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "login failed"})
		return
	}
	s.setSessionCookie(w, token, expiresAt)

	adminToken := ""
	if user.Role == storage.RoleAdmin {
		adminToken, _, _ = auth.SignSessionToken(s.authSecret, user.ID, user.Username, user.Role, adminTokenTTL)
	}
	writeJSON(w, http.StatusOK, loginResponse{Token: adminToken, ExpiresAt: expiresAt, User: publicUser(user)})
}

func bearerToken(r *http.Request) string {
	return strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
}

func publicUser(user storage.UserProfile) sessionUser {
	return sessionUser{ID: user.ID, Username: user.Username, DisplayName: user.DisplayName, Role: user.Role}
}

func normalizeUsername(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func validUsername(value string) bool {
	if len(value) < 3 || len(value) > 32 {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '-' || char == '_' || char == '.' {
			continue
		}
		return false
	}
	return true
}

func validPassword(value string) error {
	if len(value) < 10 {
		return errPasswordTooShort
	}
	return nil
}

var errPasswordTooShort = errString("password must be at least 10 characters")

type errString string

func (e errString) Error() string { return string(e) }

func (s *Server) loginKey(username string, r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return username + "|" + host
}

func (s *Server) loginLocked(username string, r *http.Request) (time.Time, bool) {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	item := s.loginFails[s.loginKey(username, r)]
	if item.LockedTil.After(time.Now()) {
		return item.LockedTil, true
	}
	return time.Time{}, false
}

func (s *Server) recordLoginFailure(username string, r *http.Request) {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	key := s.loginKey(username, r)
	item := s.loginFails[key]
	item.Count++
	if item.Count >= 5 {
		item.LockedTil = time.Now().Add(10 * time.Minute)
	}
	s.loginFails[key] = item
}

func (s *Server) clearLoginFailures(username string, r *http.Request) {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	delete(s.loginFails, s.loginKey(username, r))
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

type slugPreviewRequest struct {
	MatchID        string `json:"matchId"`
	OriginalCode   string `json:"originalCode"`
	PolymarketCode string `json:"polymarketCode"`
	LiveTest       bool   `json:"liveTest"`
}

func (s *Server) handleAdminSlugPreview(w http.ResponseWriter, r *http.Request) {
	var req slugPreviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.MatchID) == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "valid match id is required"})
		return
	}
	diagnostic, err := s.esports.SlugDiagnostic(r.Context(), req.MatchID, req.OriginalCode, req.PolymarketCode, req.LiveTest)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: "slug preview failed"})
		return
	}
	writeJSON(w, http.StatusOK, diagnostic)
}

func (s *Server) handleAdminStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"esports":          s.esports.Status(r.Context()),
		"marketDataSource": s.marketDataSource(),
	})
}

type adminSettingsResponse struct {
	RegistrationOpen bool `json:"registrationOpen"`
}

type adminSettingsRequest struct {
	RegistrationOpen *bool `json:"registrationOpen"`
}

func (s *Server) handleAdminSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, adminSettingsResponse{RegistrationOpen: s.registrationOpen(r.Context())})
}

func (s *Server) handleUpdateAdminSettings(w http.ResponseWriter, r *http.Request) {
	var req adminSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RegistrationOpen == nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid settings"})
		return
	}
	if err := s.db.SetMeta(r.Context(), registrationOpenKey, boolString(*req.RegistrationOpen)); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "save settings failed"})
		return
	}
	s.handleAdminSettings(w, r)
}

func (s *Server) registrationOpen(ctx context.Context) bool {
	value, found, err := s.db.GetMeta(ctx, registrationOpenKey)
	if err != nil || !found {
		return true
	}
	return value == "true"
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func (s *Server) handleAdminRefresh(w http.ResponseWriter, r *http.Request) {
	matches, err := s.esports.ForceRefresh(r.Context())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: "refresh failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"refreshed": len(matches)})
}
