package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Shik3i/KoalaTrade/backend/internal/marketdata"
)

type healthResponse struct {
	Status string `json:"status"`
	DB     string `json:"db"`
}

type configResponse struct {
	AppName           string `json:"appName"`
	Environment       string `json:"environment"`
	StartingCashCents int64  `json:"startingCashCents"`
	MarketDataSource  string `json:"marketDataSource"`
	RegistrationOpen  bool   `json:"registrationOpen"`
}

type marketsResponse struct {
	Markets []marketdata.Market `json:"markets"`
}

type quotesResponse struct {
	Quotes []marketdata.Quote `json:"quotes"`
}

type errorResponse struct {
	Error string `json:"error"`
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

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, configResponse{
		AppName:           s.cfg.AppName,
		Environment:       s.cfg.Environment,
		StartingCashCents: s.cfg.StartingCashCents,
		MarketDataSource:  s.marketDataSource(),
		RegistrationOpen:  s.registrationOpen(r.Context()),
	})
}

// marketDataSource reports which live providers are actually wired up, so the UI
// never shows a misleading placeholder like "mock". CoinGecko is always active
// (crypto needs no key); Finnhub is active only when an API key is configured.
func (s *Server) marketDataSource() string {
	equities := "yahoo"
	if strings.TrimSpace(s.cfg.FinnhubAPIKey) != "" {
		equities = "finnhub"
	}
	return equities + "+coingecko"
}

func (s *Server) handleMarkets(w http.ResponseWriter, r *http.Request) {
	markets, err := s.marketData.Markets(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "market data unavailable"})
		return
	}

	writeJSON(w, http.StatusOK, marketsResponse{Markets: markets})
}

func (s *Server) handleQuotes(w http.ResponseWriter, r *http.Request) {
	ids := strings.Split(r.URL.Query().Get("ids"), ",")
	quotes, err := s.marketData.Quotes(r.Context(), ids)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, quotesResponse{Quotes: quotes})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
