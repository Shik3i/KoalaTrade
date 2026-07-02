package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/Shik3i/KoalaTrade/backend/internal/auth"
	"github.com/Shik3i/KoalaTrade/backend/internal/storage"
)

type accountUpdateRequest struct {
	DisplayName string `json:"displayName"`
}

type passwordChangeRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

type passwordConfirmRequest struct {
	Password string `json:"password"`
}

type accountExport struct {
	ExportedAt time.Time              `json:"exportedAt"`
	User       sessionUser            `json:"user"`
	Portfolios []portfolioSyncRequest `json:"portfolios"`
}

func (s *Server) requireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := s.currentUser(r)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "not authenticated"})
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userContextKey, user)))
	})
}

func (s *Server) handleUpdateAccount(w http.ResponseWriter, r *http.Request) {
	user, _ := s.currentUser(r)
	var req accountUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request"})
		return
	}
	displayName := strings.TrimSpace(req.DisplayName)
	if len(displayName) < 2 || len(displayName) > 48 {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "display name must be 2-48 characters"})
		return
	}
	if err := s.db.UpdateUserDisplayName(r.Context(), user.ID, displayName); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "update account failed"})
		return
	}
	updated, found, err := s.db.UserByID(r.Context(), user.ID)
	if err != nil || !found {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "update account failed"})
		return
	}
	writeJSON(w, http.StatusOK, meResponse{User: publicUser(updated)})
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	user, _ := s.currentUser(r)
	var req passwordChangeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request"})
		return
	}
	stored, ok := s.verifyCurrentPassword(w, r, user.ID, req.CurrentPassword)
	if !ok {
		return
	}
	if err := validPassword(req.NewPassword); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	if auth.VerifyPassword(req.NewPassword, stored.PasswordHash) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "new password must be different"})
		return
	}
	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "password change failed"})
		return
	}
	if err := s.db.UpdateUserPasswordHash(r.Context(), user.ID, hash); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "password change failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleExportAccount(w http.ResponseWriter, r *http.Request) {
	user, _ := s.currentUser(r)
	portfolios, err := s.db.PortfoliosByUser(r.Context(), user.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "export failed"})
		return
	}
	payloads := make([]portfolioSyncRequest, 0, len(portfolios))
	for _, portfolio := range portfolios {
		payload, err := portfolioToSyncPayload(portfolio)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "export failed"})
			return
		}
		payloads = append(payloads, payload)
	}
	writeJSON(w, http.StatusOK, accountExport{ExportedAt: time.Now().UTC(), User: user, Portfolios: payloads})
}

func (s *Server) handleDeletePortfolioData(w http.ResponseWriter, r *http.Request) {
	user, _ := s.currentUser(r)
	var req passwordConfirmRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request"})
		return
	}
	if _, ok := s.verifyCurrentPassword(w, r, user.ID, req.Password); !ok {
		return
	}
	if err := s.db.DeletePortfoliosByUser(r.Context(), user.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "delete portfolio data failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	user, _ := s.currentUser(r)
	var req passwordConfirmRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request"})
		return
	}
	if _, ok := s.verifyCurrentPassword(w, r, user.ID, req.Password); !ok {
		return
	}
	if err := s.db.DeleteUser(r.Context(), user.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "delete account failed"})
		return
	}
	clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) verifyCurrentPassword(w http.ResponseWriter, r *http.Request, userID, password string) (storage.UserProfile, bool) {
	user, found, err := s.db.UserByID(r.Context(), userID)
	if err != nil || !found || user.Disabled {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "not authenticated"})
		return storage.UserProfile{}, false
	}
	if !auth.VerifyPassword(password, user.PasswordHash) {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "invalid password"})
		return storage.UserProfile{}, false
	}
	return user, true
}
