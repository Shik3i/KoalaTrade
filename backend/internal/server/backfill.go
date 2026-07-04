package server

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/Shik3i/KoalaTrade/backend/internal/marketdata"
)

// backfillFetchDelay spaces provider requests so the maintainer stays under the
// CoinGecko free-tier rate limit. The keyless public API is very aggressive; a
// COINGECKO_API_KEY (Demo tier, 30/min) makes this fast and reliable.
const backfillFetchDelay = 6 * time.Second

// Adaptive cadence: while gaps remain we re-run soon to converge (respecting the
// rate limit); once everything is covered we idle and only re-check occasionally
// to heal gaps from downtime.
const (
	backfillActiveInterval = 3 * time.Minute
	backfillIdleInterval   = 60 * time.Minute
)

// backfill429Backoffs are the waits between retries when CoinGecko returns 429.
var backfill429Backoffs = []time.Duration{8 * time.Second, 20 * time.Second, 45 * time.Second}

// coverageSpec describes one history tier the maintainer keeps populated for
// crypto assets: how far back CoinGecko is asked for (days — auto-granularity:
// 1 → 5-minutely, 2–90 → hourly, >90 → daily), the window it checks coverage
// over, and the minimum points expected in that window before it backfills.
type coverageSpec struct {
	timeframe string
	days      int
	window    time.Duration
	minPoints int
}

// Ordered most-valuable-first so the long 1D history is filled before finer tiers
// when the rate limit only lets a few requests through per pass.
var cryptoCoverageSpecs = []coverageSpec{
	{timeframe: "1D", days: 365, window: 365 * 24 * time.Hour, minPoints: 300},
	{timeframe: "6H", days: 30, window: 30 * 24 * time.Hour, minPoints: 90},
	{timeframe: "1H", days: 7, window: 7 * 24 * time.Hour, minPoints: 100},
	{timeframe: "5M", days: 1, window: 24 * time.Hour, minPoints: 100},
}

// StartCryptoHistoryMaintainer continuously keeps crypto chart history populated.
// Instead of a one-shot startup job, it periodically checks each asset/tier for
// missing data and backfills only the gaps from CoinGecko — so it completes the
// initial (rate-limited) backfill over several passes and self-heals gaps caused
// by downtime. Stocks/ETFs have no free historical source and fill via the poller.
func (s *Server) StartCryptoHistoryMaintainer(ctx context.Context, logger *slog.Logger) {
	go func() {
		for {
			fetched := s.runCryptoBackfillPass(ctx, logger)
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

// runCryptoBackfillPass checks every crypto asset/tier once and backfills the
// gaps. Returns how many provider fetches it performed (0 means fully covered).
func (s *Server) runCryptoBackfillPass(ctx context.Context, logger *slog.Logger) int {
	// Ensure the catalogue (and thus the assets FK targets) exists.
	if _, err := s.marketData.Markets(ctx); err != nil {
		logger.Warn("crypto backfill: catalogue unavailable, skipping pass", "error", err)
		return 0
	}

	assetIDs := s.coingecko.CryptoAssetIDs()
	now := time.Now().UTC()
	fetched := 0

	for _, assetID := range assetIDs {
		for _, spec := range cryptoCoverageSpecs {
			if ctx.Err() != nil {
				return fetched
			}

			count, err := s.db.HistoryCoverage(ctx, assetID, spec.timeframe, now.Add(-spec.window))
			if err != nil {
				logger.Warn("crypto backfill coverage check failed", "asset_id", assetID, "timeframe", spec.timeframe, "error", err)
				continue
			}
			if count >= spec.minPoints {
				continue // tier already sufficiently covered
			}

			// Space out actual provider calls to respect the rate limit.
			select {
			case <-ctx.Done():
				return fetched
			case <-time.After(backfillFetchDelay):
			}

			points, err := s.fetchHistoryWithRetry(ctx, assetID, spec.days, logger)
			if err != nil {
				if ctx.Err() != nil {
					return fetched
				}
				logger.Warn("crypto backfill fetch failed", "asset_id", assetID, "timeframe", spec.timeframe, "days", spec.days, "error", err)
				continue
			}
			fetched++
			if _, err := s.db.StoreHistory(ctx, assetID, spec.timeframe, points); err != nil {
				logger.Warn("crypto backfill store failed", "asset_id", assetID, "timeframe", spec.timeframe, "error", err)
			}
		}
	}

	if fetched > 0 {
		logger.Info("crypto history backfill pass done", "fetches", fetched)
	}
	return fetched
}

// fetchHistoryWithRetry fetches one market_chart window, retrying with backoff
// when CoinGecko rate-limits (HTTP 429). Non-429 errors are returned immediately.
func (s *Server) fetchHistoryWithRetry(ctx context.Context, assetID string, days int, logger *slog.Logger) ([]marketdata.HistoricalPoint, error) {
	points, err := s.coingecko.HistoricalPrices(ctx, assetID, days)
	if err == nil || !strings.Contains(err.Error(), "429") {
		return points, err
	}

	for attempt, wait := range backfill429Backoffs {
		logger.Info("crypto backfill rate-limited, backing off", "asset_id", assetID, "days", days, "attempt", attempt+1)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
		points, err = s.coingecko.HistoricalPrices(ctx, assetID, days)
		if err == nil || !strings.Contains(err.Error(), "429") {
			return points, err
		}
	}
	return nil, err
}
