package server

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/Shik3i/KoalaTrade/backend/internal/marketdata"
)

// backfillFetchDelay spaces provider requests so the maintainer stays under
// free-tier / anti-abuse rate limits (CoinGecko for crypto, Yahoo for equities).
const backfillFetchDelay = 6 * time.Second

// Adaptive cadence: while gaps remain we re-run soon to converge (respecting the
// rate limit); once everything is covered we idle and only re-check occasionally
// to heal gaps from downtime.
const (
	backfillActiveInterval = 3 * time.Minute
	backfillIdleInterval   = 60 * time.Minute
)

// backfillRetryBackoffs are the waits between retries when a provider rate-limits.
var backfillRetryBackoffs = []time.Duration{8 * time.Second, 20 * time.Second, 45 * time.Second}

// coverageSpec describes one history tier the maintainer keeps populated: how far
// back the provider is asked for (days), the window it checks coverage over, and
// the minimum points expected in that window before it backfills.
type coverageSpec struct {
	timeframe string
	days      int
	window    time.Duration
	minPoints int
}

// Ordered most-valuable-first so the long 1D history is filled before finer tiers
// when a rate limit only lets a few requests through per pass.
var historyCoverageSpecs = []coverageSpec{
	{timeframe: "1D", days: 365, window: 365 * 24 * time.Hour, minPoints: 200},
	{timeframe: "6H", days: 30, window: 30 * 24 * time.Hour, minPoints: 80},
	{timeframe: "1H", days: 7, window: 7 * 24 * time.Hour, minPoints: 80},
	{timeframe: "5M", days: 1, window: 24 * time.Hour, minPoints: 30},
}

// StartHistoryMaintainer continuously keeps chart history populated. Rather than a
// one-shot startup job, it periodically checks each asset/tier for missing data
// and backfills only the gaps — crypto from CoinGecko, equities from Yahoo. This
// completes the initial (rate-limited) backfill over several passes and self-heals
// gaps caused by downtime.
func (s *Server) StartHistoryMaintainer(ctx context.Context, logger *slog.Logger) {
	go func() {
		for {
			fetched := s.runHistoryBackfillPass(ctx, logger)
			if ctx.Err() != nil {
				return
			}

			interval := backfillIdleInterval
			if fetched > 0 {
				interval = backfillActiveInterval
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(interval):
			}
		}
	}()
}

// historyProviderFor returns the provider that can supply history for a kind, or
// nil if none (e.g. eSports events have no historical price series).
func (s *Server) historyProviderFor(kind marketdata.AssetKind) marketdata.HistoricalPricer {
	switch kind {
	case marketdata.AssetKindCrypto:
		return s.coingecko
	case marketdata.AssetKindStock, marketdata.AssetKindETF, marketdata.AssetKindCommodity:
		return s.yahoo
	default:
		return nil
	}
}

// runHistoryBackfillPass checks every asset/tier once and backfills the gaps.
// Returns how many provider fetches it performed (0 means fully covered).
func (s *Server) runHistoryBackfillPass(ctx context.Context, logger *slog.Logger) int {
	markets, err := s.marketData.Markets(ctx)
	if err != nil {
		logger.Warn("history backfill: catalogue unavailable, skipping pass", "error", err)
		return 0
	}

	now := time.Now().UTC()
	fetched := 0

	for _, market := range markets {
		provider := s.historyProviderFor(market.Kind)
		if provider == nil {
			continue
		}

		for _, spec := range historyCoverageSpecs {
			if ctx.Err() != nil {
				return fetched
			}

			count, err := s.db.HistoryCoverage(ctx, market.AssetID, spec.timeframe, now.Add(-spec.window))
			if err != nil {
				logger.Warn("history backfill coverage check failed", "asset_id", market.AssetID, "timeframe", spec.timeframe, "error", err)
				continue
			}
			if count >= spec.minPoints {
				continue // tier already sufficiently covered
			}

			// Space out actual provider calls to respect rate limits.
			select {
			case <-ctx.Done():
				return fetched
			case <-time.After(backfillFetchDelay):
			}

			points, err := s.fetchHistoryWithRetry(ctx, provider, market.AssetID, spec.days, logger)
			if err != nil {
				if ctx.Err() != nil {
					return fetched
				}
				logger.Warn("history backfill fetch failed", "asset_id", market.AssetID, "timeframe", spec.timeframe, "days", spec.days, "error", err)
				continue
			}
			fetched++
			if _, err := s.db.StoreHistory(ctx, market.AssetID, spec.timeframe, points); err != nil {
				logger.Warn("history backfill store failed", "asset_id", market.AssetID, "timeframe", spec.timeframe, "error", err)
			}
		}
	}

	if fetched > 0 {
		logger.Info("history backfill pass done", "fetches", fetched)
	}
	return fetched
}

// fetchHistoryWithRetry fetches one history window, retrying with backoff when the
// provider rate-limits (HTTP 429). Non-429 errors are returned immediately.
func (s *Server) fetchHistoryWithRetry(ctx context.Context, provider marketdata.HistoricalPricer, assetID string, days int, logger *slog.Logger) ([]marketdata.HistoricalPoint, error) {
	points, err := provider.HistoricalPrices(ctx, assetID, days)
	if err == nil || !strings.Contains(err.Error(), "429") {
		return points, err
	}

	for attempt, wait := range backfillRetryBackoffs {
		logger.Info("history backfill rate-limited, backing off", "asset_id", assetID, "days", days, "attempt", attempt+1)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
		points, err = provider.HistoricalPrices(ctx, assetID, days)
		if err == nil || !strings.Contains(err.Error(), "429") {
			return points, err
		}
	}
	return nil, err
}
