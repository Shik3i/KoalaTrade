package server

import (
	"context"
	"log/slog"
	"time"
)

// minPollInterval is the floor between single-asset refreshes. At most one
// provider request fires per interval, so 3s keeps us at ≤20 req/min — safely
// under both the Finnhub (60/min) and CoinGecko demo (30/min) free-tier limits
// no matter how many assets are in the catalogue.
const minPollInterval = 3 * time.Second

func (s *Server) StartMarketDataPoller(ctx context.Context, logger *slog.Logger) {
	go func() {
		// Prime the catalogue and warm the in-memory cache from the DB. This is
		// cheap (no provider fetch) and lets a restart with persisted quotes serve
		// prices immediately while the poller refreshes them in the background.
		s.warmupMarketData(ctx, logger)

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			markets, err := s.marketData.Markets(ctx)
			if err != nil || len(markets) == 0 {
				if err != nil {
					logger.Warn("market data poll: markets unavailable", "error", err)
				}
				select {
				case <-ctx.Done():
					return
				case <-time.After(10 * time.Second):
					continue
				}
			}

			// Spread every asset's refresh evenly across the configured window so
			// the per-minute provider rate limit is never exceeded.
			window := time.Duration(s.cfg.MarketDataRefreshWindowSecs) * time.Second
			interval := window / time.Duration(len(markets))
			if interval < minPollInterval {
				interval = minPollInterval
			}

			for _, market := range markets {
				select {
				case <-ctx.Done():
					return
				case <-time.After(interval):
					s.refreshOne(ctx, logger, market.AssetID)
				}
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
	// Populate the catalogue + warm the memory cache from persisted quotes. No
	// provider requests happen here — Markets() is a pure read path now.
	markets, err := s.marketData.Markets(ctx)
	if err != nil {
		logger.Warn("market data warmup failed", "error", err)
		return
	}
	logger.Info("market data warmup completed", "assets", len(markets))
}

// refreshOne fetches a single asset's quote from the provider chain and persists
// it. Exactly one asset is refreshed per poll tick (round-robin), which is what
// keeps the request rate under the free-tier limit.
func (s *Server) refreshOne(ctx context.Context, logger *slog.Logger, assetID string) {
	refreshCtx, cancel := context.WithTimeout(ctx, time.Duration(s.cfg.MarketDataHTTPTimeout+5)*time.Second)
	defer cancel()

	if _, err := s.marketData.Refresh(refreshCtx, []string{assetID}); err != nil {
		logger.Warn("market data poll failed", "error", err, "asset_id", assetID)
	}
}

// StartEsportsTeamsPoller refreshes the complete LoL team/logo snapshot once a
// week on Monday morning UTC. A stale snapshot is also refreshed once at
// startup so a newly deployed instance does not wait for the next Monday.
func (s *Server) StartEsportsTeamsPoller(ctx context.Context, logger *slog.Logger) {
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(15 * time.Second):
			logger.Info("esports teams initial poll started")
			if err := s.esports.SyncTeamsIfDue(ctx); err != nil {
				logger.Warn("initial esports teams poll failed", "error", err)
			} else {
				logger.Info("esports teams initial poll completed")
			}
		}

		for {
			wait := time.Until(nextEsportsTeamsSync(time.Now().UTC()))
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					<-timer.C
				}
				return
			case <-timer.C:
				logger.Info("running esports teams weekly poll")
				if err := s.esports.SyncTeamsIfDue(ctx); err != nil {
					logger.Warn("weekly esports teams poll failed", "error", err)
				} else {
					logger.Info("weekly esports teams poll completed")
				}
			}
		}
	}()
}

func nextEsportsTeamsSync(now time.Time) time.Time {
	next := time.Date(now.Year(), now.Month(), now.Day(), 3, 15, 0, 0, time.UTC)
	daysUntilMonday := (int(time.Monday) - int(now.Weekday()) + 7) % 7
	next = next.AddDate(0, 0, daysUntilMonday)
	if !next.After(now) {
		next = next.AddDate(0, 0, 7)
	}
	return next
}
