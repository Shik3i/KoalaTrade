package server

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Shik3i/KoalaTrade/backend/internal/esports"
	"github.com/go-chi/chi/v5"
)

type esportsResponse struct {
	Matches   []esports.Match `json:"matches"`
	UpdatedAt time.Time       `json:"updatedAt"`
}

type teamsResponse struct {
	Teams []esports.TeamInfo `json:"teams"`
}

func (s *Server) handleEsportsMatches(w http.ResponseWriter, r *http.Request) {
	matches, err := s.esports.Matches(r.Context())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: "esports feed unavailable"})
		return
	}

	writeJSON(w, http.StatusOK, esportsResponse{
		Matches:   matches,
		UpdatedAt: time.Now().UTC(),
	})
}

func (s *Server) handleEsportsTeams(w http.ResponseWriter, r *http.Request) {
	teams, err := s.esports.Teams(r.Context())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: "esports teams unavailable"})
		return
	}

	writeJSON(w, http.StatusOK, teamsResponse{Teams: teams})
}

type resultsResponse struct {
	Results []esports.Result `json:"results"`
}

func (s *Server) handleEsportsResults(w http.ResponseWriter, r *http.Request) {
	ids := make([]string, 0)
	for _, id := range strings.Split(r.URL.Query().Get("ids"), ",") {
		if trimmed := strings.TrimSpace(id); trimmed != "" {
			ids = append(ids, trimmed)
		}
	}
	if len(ids) == 0 {
		writeJSON(w, http.StatusOK, resultsResponse{Results: []esports.Result{}})
		return
	}

	writeJSON(w, http.StatusOK, resultsResponse{Results: s.esports.Results(r.Context(), ids)})
}

// handleMatchOdds force-refreshes a single match's Polymarket odds on demand,
// used right before a bet is placed (Polymarket is not rate limited).
func (s *Server) handleMatchOdds(w http.ResponseWriter, r *http.Request) {
	match, err := s.esports.RefreshMatchOdds(r.Context(), chi.URLParam(r, "matchId"))
	if err != nil {
		if errors.Is(err, esports.ErrMatchNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "match not found"})
			return
		}
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: "odds refresh failed"})
		return
	}

	writeJSON(w, http.StatusOK, match)
}
