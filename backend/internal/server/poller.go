package server

import (
	"context"
	"log/slog"
	"time"
)

func (s *Server) StartMarketDataPoller(ctx context.Context, logger *slog.Logger) {
	interval := time.Duration(s.cfg.MarketDataPollSeconds) * time.Second
	if interval <= 0 {
		return
	}

	go func() {
		s.pollMarketData(ctx, logger)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.pollMarketData(ctx, logger)
			}
		}
	}()
}

func (s *Server) pollMarketData(ctx context.Context, logger *slog.Logger) {
	refreshCtx, cancel := context.WithTimeout(ctx, time.Duration(s.cfg.MarketDataHTTPTimeout+5)*time.Second)
	defer cancel()

	quotes, err := s.marketData.RefreshAll(refreshCtx)
	if err != nil {
		logger.Warn("market data poll failed", "error", err)
		return
	}
	logger.Info("market data poll completed", "quotes", len(quotes))
}
