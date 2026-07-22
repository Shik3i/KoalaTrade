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

	// Without an explicit secret, the server creates one in the persistent SQLite
	// database. This keeps sessions valid across ordinary single-instance restarts;
	// deployments with multiple replicas should still provide one shared secret.
	if cfg.AuthSecret == "" {
		logger.Warn("AUTH_SECRET is not set: using the signing key persisted in SQLite; set AUTH_SECRET for multi-replica deployments")
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
	app.StartBetSettler(ctx, logger)

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
