package server

import (
	"testing"
	"time"

	"github.com/Shik3i/KoalaTrade/backend/internal/esports"
	"github.com/Shik3i/KoalaTrade/backend/internal/marketdata"
	"github.com/Shik3i/KoalaTrade/backend/internal/storage"
)

func eventPosition(matchID, team string, contracts, avgCents int64) storage.PortfolioPosition {
	return storage.PortfolioPosition{
		AssetID:          esportsAssetID(matchID, team),
		Symbol:           team,
		Name:             team + " win",
		Kind:             marketdata.AssetKindEvent,
		QuantityMicro:    contracts * quantityScale,
		AverageCostCents: avgCents,
		LastPriceCents:   avgCents,
	}
}

func TestSettleEventPositionsPaysWinnerAndExpiresLoser(t *testing.T) {
	now := time.Now().UTC()
	p := storage.Portfolio{
		CashCents: 500,
		Positions: []storage.PortfolioPosition{
			eventPosition("m1", "AAA", 10, 40), // winner → pays 100¢ x10 = 1000
			eventPosition("m1", "BBB", 5, 60),  // loser  → 0
			{AssetID: "stock:aapl", Symbol: "AAPL", Kind: marketdata.AssetKindStock, QuantityMicro: quantityScale, AverageCostCents: 10_000, LastPriceCents: 10_000},
		},
	}
	results := map[string]esports.Result{
		"m1": {MatchID: "m1", WinnerCode: "aaa"}, // case-insensitive
	}

	out, changed := settleEventPositions(p, results, now)
	if !changed {
		t.Fatal("expected settlement to change the portfolio")
	}
	if out.CashCents != 500+1000 {
		t.Fatalf("cash = %d, want %d", out.CashCents, 1500)
	}
	// Both event positions removed; the stock position stays.
	if len(out.Positions) != 1 || out.Positions[0].AssetID != "stock:aapl" {
		t.Fatalf("positions = %+v, want only the stock", out.Positions)
	}
	if len(out.Transactions) != 2 {
		t.Fatalf("expected 2 settlement transactions, got %d", len(out.Transactions))
	}
}

func TestSettleEventPositionsNoResultsNoChange(t *testing.T) {
	now := time.Now().UTC()
	p := storage.Portfolio{Positions: []storage.PortfolioPosition{eventPosition("m9", "AAA", 1, 50)}}
	if _, changed := settleEventPositions(p, map[string]esports.Result{}, now); changed {
		t.Fatal("expected no change when the match is unresolved")
	}
}

func TestParseEventAsset(t *testing.T) {
	m, tc, ok := parseEventAsset("event:lol:123:T1")
	if !ok || m != "123" || tc != "T1" {
		t.Fatalf("parse = %q %q %v", m, tc, ok)
	}
	if _, _, ok := parseEventAsset("stock:aapl"); ok {
		t.Fatal("expected non-event asset to be rejected")
	}
}

func TestEsportsBettingClosedAtFullPolymarketPrice(t *testing.T) {
	if !esportsBettingClosed("buy", esports.Team{PriceCents: 100}, esports.Team{PriceCents: 0}) {
		t.Fatal("expected a buy to be blocked when the selected outcome is at 100 cents")
	}
	if !esportsBettingClosed("buy", esports.Team{PriceCents: 99}, esports.Team{PriceCents: 100}) {
		t.Fatal("expected all buys for the match to be blocked when either outcome is at 100 cents")
	}
	if esportsBettingClosed("sell", esports.Team{PriceCents: 100}, esports.Team{PriceCents: 0}) {
		t.Fatal("expected sells to remain available at 100 cents")
	}
}
