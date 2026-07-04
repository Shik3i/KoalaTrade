package storage

import (
	"context"
	"testing"
	"time"

	"github.com/Shik3i/KoalaTrade/backend/internal/marketdata"
)

func TestOpenSQLiteCreatesFoundationTables(t *testing.T) {
	store, err := OpenSQLite(t.TempDir() + "/koalatrade.db")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	ctx := context.Background()
	for _, table := range []string{
		"app_meta",
		"user_profiles",
		"assets",
		"asset_quotes",
		"portfolios",
		"portfolio_positions",
		"portfolio_transactions",
		"portfolio_snapshots",
		"leaderboard_snapshots",
	} {
		exists, err := store.TableExists(ctx, table)
		if err != nil {
			t.Fatalf("check table %s: %v", table, err)
		}
		if !exists {
			t.Fatalf("expected table %s to exist", table)
		}
	}
}

func TestSQLiteStoresFreshMarketQuotes(t *testing.T) {
	store, err := OpenSQLite(t.TempDir() + "/koalatrade.db")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	ctx := context.Background()
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	markets := []marketdata.Market{
		{
			AssetID:    "crypto:btc",
			Symbol:     "BTC",
			Name:       "Bitcoin",
			Kind:       marketdata.AssetKindCrypto,
			Source:     "mock",
			PriceCents: 6_100_000,
			ChangeBPS:  120,
			UpdatedAt:  now,
		},
	}
	if err := store.UpsertMarkets(ctx, markets); err != nil {
		t.Fatalf("upsert markets: %v", err)
	}

	quote := marketdata.Quote{
		AssetID:     "crypto:btc",
		Symbol:      "BTC",
		PriceCents:  6_123_456,
		ChangeBPS:   140,
		Source:      "mock",
		UpdatedAt:   now,
		CachedUntil: now.Add(time.Minute),
	}
	if err := store.StoreQuotes(ctx, []marketdata.Quote{quote}); err != nil {
		t.Fatalf("store quotes: %v", err)
	}

	fresh, err := store.FreshQuotes(ctx, []string{"crypto:btc"}, now)
	if err != nil {
		t.Fatalf("fresh quotes: %v", err)
	}
	if len(fresh) != 1 {
		t.Fatalf("expected 1 fresh quote, got %d", len(fresh))
	}
	if fresh[0].PriceCents != quote.PriceCents {
		t.Fatalf("expected price %d, got %d", quote.PriceCents, fresh[0].PriceCents)
	}

	stale, err := store.FreshQuotes(ctx, []string{"crypto:btc"}, now.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("stale quotes: %v", err)
	}
	if len(stale) != 0 {
		t.Fatalf("expected stale quote to be filtered, got %d", len(stale))
	}
}

// Regression: reopening the database (i.e. a server restart) must NOT wipe
// accumulated asset history. The legacy-schema migration is idempotent.
func TestSQLiteHistorySurvivesReopen(t *testing.T) {
	path := t.TempDir() + "/koalatrade.db"
	ctx := context.Background()
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)

	store, err := OpenSQLite(path)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	markets := []marketdata.Market{{
		AssetID: "crypto:btc", Symbol: "BTC", Name: "Bitcoin",
		Kind: marketdata.AssetKindCrypto, Source: "mock", UpdatedAt: now,
	}}
	if err := store.UpsertMarkets(ctx, markets); err != nil {
		t.Fatalf("upsert markets: %v", err)
	}
	if err := store.StoreQuotes(ctx, []marketdata.Quote{{
		AssetID: "crypto:btc", Symbol: "BTC", PriceCents: 6_123_456, ChangeBPS: 140,
		Source: "mock", UpdatedAt: now, CachedUntil: now.Add(time.Minute),
	}}); err != nil {
		t.Fatalf("store quotes: %v", err)
	}
	_ = store.Close()

	// Reopen the same file — configure() runs again.
	reopened, err := OpenSQLite(path)
	if err != nil {
		t.Fatalf("reopen sqlite: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })

	// Use the 1D tier (≈1000-day retention) so the assertion doesn't depend on
	// the wall clock the way the short-lived 5M tier would.
	history, err := reopened.GetHistory(ctx, "crypto:btc", "1D", now.Add(-72*time.Hour))
	if err != nil {
		t.Fatalf("get history: %v", err)
	}
	if len(history) == 0 {
		t.Fatal("expected history to survive reopen, got none")
	}
}

// Backfilled 1D history must persist forever, while a fine tier (5M) is pruned
// to its bounded retention window when new quotes are stored.
func TestSQLiteHistoryRetention(t *testing.T) {
	store, err := OpenSQLite(t.TempDir() + "/koalatrade.db")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx := context.Background()
	now := time.Now().UTC()

	if err := store.UpsertMarkets(ctx, []marketdata.Market{{
		AssetID: "crypto:btc", Symbol: "BTC", Name: "Bitcoin",
		Kind: marketdata.AssetKindCrypto, Source: "coingecko", UpdatedAt: now,
	}}); err != nil {
		t.Fatalf("upsert markets: %v", err)
	}

	// Backfill: an old daily point (300 days ago) and an old 5-minute point.
	if _, err := store.StoreHistory(ctx, "crypto:btc", "1D", []marketdata.HistoricalPoint{
		{Timestamp: now.AddDate(0, 0, -300), PriceCents: 5_000_000},
	}); err != nil {
		t.Fatalf("store 1D history: %v", err)
	}
	if _, err := store.StoreHistory(ctx, "crypto:btc", "5M", []marketdata.HistoricalPoint{
		{Timestamp: now.Add(-10 * 24 * time.Hour), PriceCents: 5_100_000},
	}); err != nil {
		t.Fatalf("store 5M history: %v", err)
	}

	// A fresh quote triggers the tier prune.
	if err := store.StoreQuotes(ctx, []marketdata.Quote{{
		AssetID: "crypto:btc", Symbol: "BTC", PriceCents: 6_000_000, ChangeBPS: 100,
		Source: "coingecko", UpdatedAt: now, CachedUntil: now.Add(time.Minute),
	}}); err != nil {
		t.Fatalf("store quotes: %v", err)
	}

	// 1D backfill 300 days old must survive (retention == forever).
	daily, err := store.GetHistory(ctx, "crypto:btc", "1D", now.AddDate(0, 0, -400))
	if err != nil {
		t.Fatalf("get 1D history: %v", err)
	}
	if len(daily) == 0 {
		t.Fatal("expected 300-day-old 1D history to survive pruning")
	}

	// 5M point 10 days old must be pruned (retention is 48h).
	fine, err := store.GetHistory(ctx, "crypto:btc", "5M", now.AddDate(0, 0, -30))
	if err != nil {
		t.Fatalf("get 5M history: %v", err)
	}
	for _, q := range fine {
		if q.PriceCents == 5_100_000 {
			t.Fatal("expected 10-day-old 5M point to be pruned")
		}
	}
}

func TestSQLitePortfolioTablesAcceptNormalizedHoldings(t *testing.T) {
	store, err := OpenSQLite(t.TempDir() + "/koalatrade.db")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	ctx := context.Background()
	now := time.Date(2026, 6, 29, 13, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	_, err = store.db.ExecContext(ctx, `INSERT INTO assets (
		id, kind, symbol, name, provider, provider_ref, updated_at
	) VALUES ('crypto:btc', 'crypto', 'BTC', 'Bitcoin', 'mock', 'crypto:btc', ?)`, now)
	if err != nil {
		t.Fatalf("insert asset: %v", err)
	}

	_, err = store.db.ExecContext(ctx, `INSERT INTO portfolios (
		id, client_id, client_portfolio_id, schema_version,
		starting_cash_cents, cash_cents, created_at, updated_at
	) VALUES ('portfolio-1', 'client-1', 'local-default', 1, 1000000, 750000, ?, ?)`, now, now)
	if err != nil {
		t.Fatalf("insert portfolio: %v", err)
	}

	_, err = store.db.ExecContext(ctx, `INSERT INTO portfolio_positions (
		portfolio_id, asset_id, symbol, name, kind, quantity_micro,
		average_cost_cents, last_price_cents, updated_at
	) VALUES ('portfolio-1', 'crypto:btc', 'BTC', 'Bitcoin', 'crypto', 250000, 6000000, 6200000, ?)`, now)
	if err != nil {
		t.Fatalf("insert position: %v", err)
	}

	_, err = store.db.ExecContext(ctx, `INSERT INTO portfolio_transactions (
		id, portfolio_id, asset_id, symbol, side, quantity_micro, price_cents,
		fee_cents, status, created_at
	) VALUES ('tx-1', 'portfolio-1', 'crypto:btc', 'BTC', 'buy', 250000, 6000000, 0, 'local', ?)`, now)
	if err != nil {
		t.Fatalf("insert transaction: %v", err)
	}

	var positionCount int
	if err := store.db.GetContext(ctx, &positionCount, `SELECT COUNT(*) FROM portfolio_positions WHERE portfolio_id = ?`, "portfolio-1"); err != nil {
		t.Fatalf("count positions: %v", err)
	}
	if positionCount != 1 {
		t.Fatalf("expected 1 position, got %d", positionCount)
	}

	_, err = store.db.ExecContext(ctx, `INSERT INTO portfolio_transactions (
		id, portfolio_id, asset_id, symbol, side, quantity_micro, price_cents,
		fee_cents, status, created_at
	) VALUES ('tx-bad', 'portfolio-1', 'crypto:btc', 'BTC', 'hold', 250000, 6000000, 0, 'local', ?)`, now)
	if err == nil {
		t.Fatal("expected invalid transaction side to fail")
	}
}
