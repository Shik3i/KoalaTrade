package server

import (
	"crypto/rand"
	"net/http"
	"sync"
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
	authSecret []byte
	loginMu    sync.Mutex
	loginFails map[string]loginFailure
}

type loginFailure struct {
	Count     int
	LockedTil time.Time
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

	secret := []byte(cfg.AuthSecret)
	if len(secret) == 0 {
		secret = make([]byte, 32)
		_, _ = rand.Read(secret)
	}

	return &Server{
		cfg:        cfg,
		db:         db,
		authSecret: secret,
		loginFails: make(map[string]loginFailure),
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

		r.Post("/auth/register", s.handleRegister)
		r.Post("/auth/login", s.handleLogin)
		r.Post("/auth/logout", s.handleLogout)
		r.Get("/auth/me", s.handleMe)

		r.Route("/account", func(r chi.Router) {
			r.Use(s.requireUser)
			r.Patch("/", s.handleUpdateAccount)
			r.Put("/password", s.handleChangePassword)
			r.Get("/export", s.handleExportAccount)
			r.Delete("/portfolio-data", s.handleDeletePortfolioData)
			r.Delete("/", s.handleDeleteAccount)
		})

		r.Route("/admin", func(r chi.Router) {
			r.Use(s.requireAdmin)
			r.Get("/settings", s.handleAdminSettings)
			r.Put("/settings", s.handleUpdateAdminSettings)
			r.Get("/mappings", s.handleListMappings)
			r.Put("/mappings", s.handleUpsertMapping)
			r.Delete("/mappings/{code}", s.handleDeleteMapping)
			r.Get("/status", s.handleAdminStatus)
			r.Post("/refresh", s.handleAdminRefresh)
		})
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
