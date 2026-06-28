package storage

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Shik3i/KoalaTrade/backend/internal/marketdata"
)

func TestSQLiteUpsertsAndReadsPortfolio(t *testing.T) {
	store, err := OpenSQLite(t.TempDir() + "/koalatrade.db")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	ctx := context.Background()
	now := time.Date(2026, 6, 29, 14, 0, 0, 0, time.UTC)
	if err := store.UpsertMarkets(ctx, []marketdata.Market{
		{
			AssetID:    "crypto:btc",
			Symbol:     "BTC",
			Name:       "Bitcoin",
			Kind:       marketdata.AssetKindCrypto,
			Source:     "mock",
			PriceCents: 6_200_000,
			UpdatedAt:  now,
		},
	}); err != nil {
		t.Fatalf("upsert markets: %v", err)
	}

	portfolio := Portfolio{
		ID:                "portfolio-1",
		ClientID:          "client-1",
		ClientPortfolioID: "local-default",
		SchemaVersion:     1,
		StartingCashCents: 1_000_000,
		CashCents:         850_000,
		CreatedAt:         now,
		UpdatedAt:         now.Add(time.Minute),
		Positions: []PortfolioPosition{
			{
				AssetID:          "crypto:btc",
				Symbol:           "BTC",
				Name:             "Bitcoin",
				Kind:             marketdata.AssetKindCrypto,
				QuantityMicro:    25_000,
				AverageCostCents: 6_000_000,
				LastPriceCents:   6_200_000,
				UpdatedAt:        now.Add(time.Minute),
			},
		},
		Transactions: []PortfolioTransaction{
			{
				ID:            "tx-old",
				AssetID:       "crypto:btc",
				Symbol:        "BTC",
				Side:          "buy",
				QuantityMicro: 10_000,
				PriceCents:    5_900_000,
				Status:        "local",
				CreatedAt:     now,
			},
			{
				ID:            "tx-new",
				AssetID:       "crypto:btc",
				Symbol:        "BTC",
				Side:          "buy",
				QuantityMicro: 15_000,
				PriceCents:    6_000_000,
				Status:        "local",
				CreatedAt:     now.Add(time.Minute),
			},
		},
	}
	if err := store.UpsertPortfolio(ctx, portfolio); err != nil {
		t.Fatalf("upsert portfolio: %v", err)
	}

	got, err := store.Portfolio(ctx, "portfolio-1")
	if err != nil {
		t.Fatalf("portfolio: %v", err)
	}
	if got.ClientPortfolioID != "local-default" || got.CashCents != 850_000 {
		t.Fatalf("unexpected portfolio header: %#v", got)
	}
	if len(got.Positions) != 1 || got.Positions[0].QuantityMicro != 25_000 {
		t.Fatalf("unexpected positions: %#v", got.Positions)
	}
	if len(got.Transactions) != 2 || got.Transactions[0].ID != "tx-new" {
		t.Fatalf("expected transactions newest first, got %#v", got.Transactions)
	}

	portfolio.CashCents = 1_000_000
	portfolio.UpdatedAt = now.Add(2 * time.Minute)
	portfolio.Positions = nil
	portfolio.Transactions = nil
	if err := store.UpsertPortfolio(ctx, portfolio); err != nil {
		t.Fatalf("replace portfolio: %v", err)
	}

	got, err = store.Portfolio(ctx, "portfolio-1")
	if err != nil {
		t.Fatalf("portfolio after replace: %v", err)
	}
	if len(got.Positions) != 0 || len(got.Transactions) != 0 {
		t.Fatalf("expected replacement to clear rows, got positions=%d transactions=%d", len(got.Positions), len(got.Transactions))
	}
}

func TestSQLitePortfolioUpsertRollsBackWhenAssetIsMissing(t *testing.T) {
	store, err := OpenSQLite(t.TempDir() + "/koalatrade.db")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	ctx := context.Background()
	now := time.Date(2026, 6, 29, 14, 30, 0, 0, time.UTC)
	err = store.UpsertPortfolio(ctx, Portfolio{
		ID:                "portfolio-1",
		ClientID:          "client-1",
		ClientPortfolioID: "local-default",
		SchemaVersion:     1,
		StartingCashCents: 1_000_000,
		CashCents:         900_000,
		CreatedAt:         now,
		UpdatedAt:         now,
		Positions: []PortfolioPosition{
			{
				AssetID:          "crypto:missing",
				Symbol:           "MISS",
				Name:             "Missing",
				Kind:             marketdata.AssetKindCrypto,
				QuantityMicro:    1_000,
				AverageCostCents: 1_000,
				LastPriceCents:   1_000,
				UpdatedAt:        now,
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "insert portfolio position") {
		t.Fatalf("expected missing asset insert error, got %v", err)
	}

	var count int
	if err := store.db.GetContext(ctx, &count, `SELECT COUNT(*) FROM portfolios WHERE id = ?`, "portfolio-1"); err != nil {
		t.Fatalf("count portfolios: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected failed upsert to roll back portfolio, got %d", count)
	}
}
