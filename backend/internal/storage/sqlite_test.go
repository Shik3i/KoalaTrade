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
