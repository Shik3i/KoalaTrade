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

	windowSecs := s.cfg.MarketDataRefreshWindowSecs
	if windowSecs < s.cfg.MarketDataPollSeconds {
		windowSecs = s.cfg.MarketDataPollSeconds
	}
	ticksPerWindow := windowSecs / s.cfg.MarketDataPollSeconds
	if ticksPerWindow < 1 {
		ticksPerWindow = 1
	}

	go func() {
		// Warm the cache once at startup, then refresh in staggered batches.
		s.warmupMarketData(ctx, logger)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		cursor := 0
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cursor = s.pollStaggered(ctx, logger, cursor, ticksPerWindow)
			}
		}
	}()
}

// StartEsportsPoller periodically refreshes the LoL schedule so completed match
// results get captured (and persisted) even when nobody has the page open.
func (s *Server) StartEsportsPoller(ctx context.Context, logger *slog.Logger) {
	interval := time.Duration(s.cfg.EsportsCacheSeconds) * time.Second
	if interval < time.Minute {
		interval = time.Minute
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, err := s.esports.Matches(ctx); err != nil {
					logger.Warn("esports poll failed", "error", err)
				}
			}
		}
	}()
}

func (s *Server) warmupMarketData(ctx context.Context, logger *slog.Logger) {
	refreshCtx, cancel := context.WithTimeout(ctx, time.Duration(s.cfg.MarketDataHTTPTimeout+5)*time.Second)
	defer cancel()

	quotes, err := s.marketData.RefreshAll(refreshCtx)
	if err != nil {
		logger.Warn("market data warmup failed", "error", err)
		return
	}
	logger.Info("market data warmup completed", "quotes", len(quotes))
}

// pollStaggered refreshes the next slice of assets so that, over ticksPerWindow
// ticks, every asset is refreshed exactly once. This spreads provider calls
// evenly across the refresh window to respect free-tier per-minute rate limits.
func (s *Server) pollStaggered(ctx context.Context, logger *slog.Logger, cursor, ticksPerWindow int) int {
	refreshCtx, cancel := context.WithTimeout(ctx, time.Duration(s.cfg.MarketDataHTTPTimeout+5)*time.Second)
	defer cancel()

	markets, err := s.marketData.Markets(refreshCtx)
	if err != nil || len(markets) == 0 {
		if err != nil {
			logger.Warn("market data poll: markets unavailable", "error", err)
		}
		return cursor
	}

	n := len(markets)
	batch := (n + ticksPerWindow - 1) / ticksPerWindow // ceil(n / ticksPerWindow)
	if batch < 1 {
		batch = 1
	}
	if cursor >= n {
		cursor = 0
	}

	ids := make([]string, 0, batch)
	for i := 0; i < batch && i < n; i++ {
		ids = append(ids, markets[(cursor+i)%n].AssetID)
	}

	quotes, err := s.marketData.Refresh(refreshCtx, ids)
	if err != nil {
		logger.Warn("market data poll failed", "error", err, "batch", len(ids))
		return cursor
	}
	logger.Info("market data poll completed", "refreshed", len(quotes), "of", n, "cursor", cursor)

	return (cursor + batch) % n
}
