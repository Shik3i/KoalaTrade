package server

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/Shik3i/KoalaTrade/backend/internal/marketdata"
)

// cryptoBackfillFlag marks that the one-time crypto history backfill has run.
// Bump the version suffix to force a re-backfill after a tier/schema change.
const cryptoBackfillFlag = "crypto_history_backfilled_v1"

// backfillFetchDelay spaces provider requests so the one-time backfill stays
// under the CoinGecko free-tier rate limit. The keyless public API is very
// aggressive; a COINGECKO_API_KEY (Demo tier, 30/min) makes this reliable.
const backfillFetchDelay = 6 * time.Second

// backfill429Backoffs are the waits between retries when CoinGecko returns 429.
var backfill429Backoffs = []time.Duration{8 * time.Second, 20 * time.Second, 45 * time.Second}

// backfillSpec maps a CoinGecko market_chart window to the history tiers it seeds.
// CoinGecko auto-granularity: days=1 → 5-minutely, 2–90 → hourly, >90 → daily.
type backfillSpec struct {
	days       int
	timeframes []string
}

var cryptoBackfillSpecs = []backfillSpec{
	{days: 1, timeframes: []string{"5M"}},
	{days: 30, timeframes: []string{"1H", "6H"}},
	{days: 365, timeframes: []string{"1D"}},
}

// StartCryptoBackfill seeds historical chart data for crypto assets from
// CoinGecko once, so long-range charts aren't empty on a fresh deployment.
// Stocks/ETFs have no free historical source, so they accumulate live via the
// poller instead. Runs in the background and is a no-op once completed.
func (s *Server) StartCryptoBackfill(ctx context.Context, logger *slog.Logger) {
	go func() {
		if done, ok, _ := s.db.GetMeta(ctx, cryptoBackfillFlag); ok && done == "1" {
			return
		}

		// Ensure the catalogue (and thus the assets FK targets) exists.
		if _, err := s.marketData.Markets(ctx); err != nil {
			logger.Warn("crypto backfill: catalogue unavailable, skipping", "error", err)
			return
		}

		assetIDs := s.coingecko.CryptoAssetIDs()
		if len(assetIDs) == 0 {
			return
		}

		logger.Info("crypto history backfill started", "assets", len(assetIDs))
		success, failures := 0, 0

		for _, assetID := range assetIDs {
			for _, spec := range cryptoBackfillSpecs {
				select {
				case <-ctx.Done():
					return
				case <-time.After(backfillFetchDelay):
				}

				points, err := s.fetchHistoryWithRetry(ctx, assetID, spec.days, logger)
				if err != nil {
					if ctx.Err() != nil {
						return
					}
					failures++
					logger.Warn("crypto backfill fetch failed", "asset_id", assetID, "days", spec.days, "error", err)
					continue
				}
				success++
				for _, tf := range spec.timeframes {
					if _, err := s.db.StoreHistory(ctx, assetID, tf, points); err != nil {
						logger.Warn("crypto backfill store failed", "asset_id", assetID, "timeframe", tf, "error", err)
					}
				}
			}
		}

		// Only mark done on a fully successful pass so partial/rate-limited runs
		// retry on the next boot (StoreHistory upserts, so re-running is safe).
		if failures == 0 {
			_ = s.db.SetMeta(ctx, cryptoBackfillFlag, "1")
			logger.Info("crypto history backfill completed", "fetched", success)
		} else {
			logger.Warn("crypto history backfill incomplete, will retry next start",
				"fetched", success, "failed", failures,
				"hint", "set COINGECKO_API_KEY (free Demo tier) for reliable backfill")
		}
	}()
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
