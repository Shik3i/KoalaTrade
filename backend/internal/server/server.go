package server

import (
	"net/http"
	"time"

	"github.com/Shik3i/KoalaTrade/backend/internal/config"
	"github.com/Shik3i/KoalaTrade/backend/internal/esports"
	"github.com/Shik3i/KoalaTrade/backend/internal/marketdata"
	"github.com/Shik3i/KoalaTrade/backend/internal/storage"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Server struct {
	cfg        config.Config
	db         *storage.SQLite
	marketData *marketdata.Service
	esports    *esports.Service
}

func New(cfg config.Config, db *storage.SQLite) *Server {
	provider := marketdata.Provider(marketdata.NewMockProvider())
	if cfg.MarketDataProvider == "coingecko" || cfg.MarketDataProvider == "live" {
		provider = marketdata.NewCoinGeckoProvider(
			cfg.CoinGeckoBaseURL,
			cfg.CoinGeckoAPIKey,
			time.Duration(cfg.MarketDataHTTPTimeout)*time.Second,
			provider,
		)
	}
	if cfg.MarketDataProvider == "finnhub" || cfg.MarketDataProvider == "live" {
		provider = marketdata.NewFinnhubProvider(
			cfg.FinnhubBaseURL,
			cfg.FinnhubAPIKey,
			time.Duration(cfg.MarketDataHTTPTimeout)*time.Second,
			provider,
		)
	}

	return &Server{
		cfg:        cfg,
		db:         db,
		marketData: marketdata.NewService(provider, time.Duration(cfg.MarketDataCacheSeconds)*time.Second, db),
		esports: esports.NewService(
			cfg.LolesportsAPIKey,
			cfg.LolesportsBaseURL,
			cfg.PolymarketBaseURL,
			time.Duration(cfg.MarketDataHTTPTimeout+10)*time.Second,
			time.Duration(cfg.EsportsCacheSeconds)*time.Second,
			db,
		),
	}
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
		r.Get("/markets", s.handleMarkets)
		r.Get("/markets/{assetId}/history", s.handleMarketHistory)
		r.Get("/quotes", s.handleQuotes)
		r.Get("/esports/matches", s.handleEsportsMatches)
		r.Get("/esports/matches/{matchId}/odds", s.handleMatchOdds)
		r.Get("/esports/teams", s.handleEsportsTeams)
		r.Get("/esports/results", s.handleEsportsResults)
		r.Get("/sync/portfolio", s.handleGetPortfolioSync)
		r.Put("/sync/portfolio", s.handlePutPortfolioSync)
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
