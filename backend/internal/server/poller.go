package server

import (
	"context"
	"log/slog"
	"time"
)

func (s *Server) StartMarketDataPoller(ctx context.Context, logger *slog.Logger) {
	go func() {
		// Warm the cache once at startup, then refresh in 3-second intervals.
		s.warmupMarketData(ctx, logger)

		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()

		cursor := 0
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cursor = s.pollSingle(ctx, logger, cursor)
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

// pollSingle fetches a single asset quote at a time in a round-robin loop.
// Waiting exactly 3 seconds between each poll ensures we never exceed 20 requests
// per minute, staying safely under the 30 requests/minute API limit.
func (s *Server) pollSingle(ctx context.Context, logger *slog.Logger, cursor int) int {
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
	if cursor >= n {
		cursor = 0
	}

	assetID := markets[cursor].AssetID
	quotes, err := s.marketData.Refresh(refreshCtx, []string{assetID})
	if err != nil {
		logger.Warn("market data poll failed", "error", err, "asset_id", assetID)
	} else {
		logger.Info("market data poll completed", "refreshed", len(quotes), "asset_id", assetID, "cursor", cursor)
	}

	return (cursor + 1) % n
}

// StartEsportsTeamsPoller runs a background poller that updates the team list in the database once a day (24 hours).
func (s *Server) StartEsportsTeamsPoller(ctx context.Context, logger *slog.Logger) {
	go func() {
		// Stagger the first poll by 15 seconds to avoid slowing down startup warmup
		select {
		case <-ctx.Done():
			return
		case <-time.After(15 * time.Second):
			logger.Info("esports teams initial poll started")
			if _, err := s.esports.Teams(ctx); err != nil {
				logger.Warn("initial esports teams poll failed", "error", err)
			} else {
				logger.Info("esports teams initial poll completed")
			}
		}

		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				logger.Info("running esports teams daily poll")
				if _, err := s.esports.Teams(ctx); err != nil {
					logger.Warn("daily esports teams poll failed", "error", err)
				} else {
					logger.Info("daily esports teams poll completed")
				}
			}
		}
	}()
}
