package server

import (
	"encoding/json"
	"net/http"
)

type healthResponse struct {
	Status string `json:"status"`
	DB     string `json:"db"`
}

type configResponse struct {
	AppName           string `json:"appName"`
	Environment       string `json:"environment"`
	StartingCashCents int64  `json:"startingCashCents"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	status := healthResponse{Status: "ok", DB: "ok"}
	if err := s.db.PingContext(r.Context()); err != nil {
		status.Status = "degraded"
		status.DB = "error"
		writeJSON(w, http.StatusServiceUnavailable, status)
		return
	}

	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleConfig(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, configResponse{
		AppName:           s.cfg.AppName,
		Environment:       s.cfg.Environment,
		StartingCashCents: s.cfg.StartingCashCents,
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
