package server

import (
	"testing"
	"time"

	"github.com/Shik3i/KoalaTrade/backend/internal/marketdata"
	"github.com/Shik3i/KoalaTrade/backend/internal/storage"
)

var testAsset = marketdata.Market{
	AssetID: "stock:aapl",
	Symbol:  "AAPL",
	Name:    "Apple Inc.",
	Kind:    marketdata.AssetKindStock,
}

func TestApplyMarketTradeBuyDeductsCashAndOpensPosition(t *testing.T) {
	now := time.Now().UTC()
	p := storage.Portfolio{StartingCashCents: 1_000_000, CashCents: 1_000_000}

	// Buy 2 @ $100.00 (10_000¢). gross = 20_000¢, fee = 20_000*8/10000 = 16¢.
	out, txn, err := applyMarketTrade(p, testAsset, "buy", 2*quantityScale, 10_000, now)
	if err != nil {
		t.Fatalf("buy: %v", err)
	}
	if out.CashCents != 1_000_000-20_000-16 {
		t.Fatalf("cash = %d, want %d", out.CashCents, 1_000_000-20_000-16)
	}
	if len(out.Positions) != 1 {
		t.Fatalf("positions = %d, want 1", len(out.Positions))
	}
	pos := out.Positions[0]
	if pos.QuantityMicro != 2*quantityScale || pos.AverageCostCents != 10_000 {
		t.Fatalf("position qty=%d avg=%d", pos.QuantityMicro, pos.AverageCostCents)
	}
	if txn.FeeCents != 16 || txn.Side != "buy" || txn.Status != "synced" {
		t.Fatalf("txn = %+v", txn)
	}
	if len(out.Transactions) != 1 {
		t.Fatalf("transactions = %d, want 1", len(out.Transactions))
	}
}

func TestApplyMarketTradeBuyRejectsInsufficientCash(t *testing.T) {
	now := time.Now().UTC()
	p := storage.Portfolio{StartingCashCents: 1_000, CashCents: 1_000}
	if _, _, err := applyMarketTrade(p, testAsset, "buy", 1*quantityScale, 10_000, now); err == nil {
		t.Fatal("expected insufficient-cash error")
	}
}

func TestApplyMarketTradeSellAveragesAndClosesPosition(t *testing.T) {
	now := time.Now().UTC()
	p := storage.Portfolio{StartingCashCents: 1_000_000, CashCents: 1_000_000}

	// Buy 1 @ 10_000, then buy 1 @ 20_000 → avg = 15_000, qty = 2.
	p, _, _ = applyMarketTrade(p, testAsset, "buy", 1*quantityScale, 10_000, now)
	p, _, _ = applyMarketTrade(p, testAsset, "buy", 1*quantityScale, 20_000, now)
	if p.Positions[0].AverageCostCents != 15_000 || p.Positions[0].QuantityMicro != 2*quantityScale {
		t.Fatalf("avg=%d qty=%d, want 15000 / 2e6", p.Positions[0].AverageCostCents, p.Positions[0].QuantityMicro)
	}

	// Sell all 2 @ 25_000 → position removed, cash credited (minus fee).
	out, _, err := applyMarketTrade(p, testAsset, "sell", 2*quantityScale, 25_000, now)
	if err != nil {
		t.Fatalf("sell: %v", err)
	}
	if len(out.Positions) != 0 {
		t.Fatalf("expected position closed, got %d", len(out.Positions))
	}
}

func TestApplyMarketTradeSellRejectsOversell(t *testing.T) {
	now := time.Now().UTC()
	p := storage.Portfolio{StartingCashCents: 1_000_000, CashCents: 1_000_000}
	p, _, _ = applyMarketTrade(p, testAsset, "buy", 1*quantityScale, 10_000, now)
	if _, _, err := applyMarketTrade(p, testAsset, "sell", 2*quantityScale, 10_000, now); err == nil {
		t.Fatal("expected oversell error")
	}
}

func TestApplyMarketTradeRejectsZeroPrice(t *testing.T) {
	now := time.Now().UTC()
	p := storage.Portfolio{StartingCashCents: 1_000_000, CashCents: 1_000_000}
	if _, _, err := applyMarketTrade(p, testAsset, "buy", 1*quantityScale, 0, now); err == nil {
		t.Fatal("expected zero-price rejection")
	}
}

func TestShouldTriggerOrder(t *testing.T) {
	cases := []struct {
		name      string
		side      string
		orderType string
		trigger   int64
		price     int64
		want      bool
	}{
		{"limit buy fills at/below", "buy", "limit", 10_000, 9_999, true},
		{"limit buy waits above", "buy", "limit", 10_000, 10_001, false},
		{"limit sell fills at/above", "sell", "limit", 10_000, 10_001, true},
		{"limit sell waits below", "sell", "limit", 10_000, 9_999, false},
		{"stop buy fires at/above", "buy", "stop", 10_000, 10_000, true},
		{"stop buy waits below", "buy", "stop", 10_000, 9_999, false},
		{"stop sell fires at/below", "sell", "stop", 10_000, 10_000, true},
		{"stop sell waits above", "sell", "stop", 10_000, 10_001, false},
		{"no price never triggers", "sell", "stop", 10_000, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			order := storage.OpenOrder{Side: c.side, OrderType: c.orderType, TriggerPriceCents: c.trigger}
			if got := shouldTriggerOrder(order, c.price); got != c.want {
				t.Fatalf("shouldTriggerOrder = %v, want %v", got, c.want)
			}
		})
	}
}
