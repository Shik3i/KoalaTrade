package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Shik3i/KoalaTrade/backend/internal/config"
	"github.com/Shik3i/KoalaTrade/backend/internal/server"
	"github.com/Shik3i/KoalaTrade/backend/internal/storage"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	cfg := config.Load()

	// Warn loudly about insecure production defaults. An empty AUTH_SECRET means a
	// fresh random signing key on every boot, invalidating all sessions on restart.
	if cfg.AuthSecret == "" {
		if cfg.Environment == "production" {
			logger.Error("AUTH_SECRET is not set in production: this is required to maintain user sessions across restarts. Server exiting.")
			os.Exit(1)
		}
		logger.Warn("AUTH_SECRET is not set: a random key is generated per start, so all sessions are invalidated on restart. Set AUTH_SECRET in production.")
	}
	if cfg.Environment == "production" && cfg.AdminPassword == "" {
		logger.Warn("ADMIN_PASSWORD is not set in production: the admin account cannot be seeded with a password.")
	}

	db, err := storage.OpenSQLite(cfg.DatabasePath)
	if err != nil {
		logger.Error("open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	app := server.New(cfg, db)
	httpServer := &http.Server{
		Addr:              cfg.ListenAddr(),
		Handler:           app.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app.SeedAdmin(ctx, logger)
	app.StartMarketDataPoller(ctx, logger)
	app.StartHistoryMaintainer(ctx, logger)
	app.StartEsportsPoller(ctx, logger)
	app.StartEsportsTeamsPoller(ctx, logger)
	app.StartOpenOrderEngine(ctx, logger)

	go func() {
		logger.Info("server listening", "addr", cfg.ListenAddr())
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server failed", "error", err)
			stop()
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("server shutdown", "error", err)
		os.Exit(1)
	}

	logger.Info("server stopped")
}
