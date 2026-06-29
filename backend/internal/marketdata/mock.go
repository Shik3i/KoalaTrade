package marketdata

import (
	"context"
	"fmt"
	"time"
)

type MockProvider struct {
	now func() time.Time
}

func NewMockProvider() *MockProvider {
	return &MockProvider{now: func() time.Time { return time.Now().UTC() }}
}

func (p *MockProvider) Markets(ctx context.Context) ([]Market, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	markets := p.fixtureMarkets()
	return markets, nil
}

func (p *MockProvider) Quotes(ctx context.Context, assetIDs []string) ([]Quote, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	byID := make(map[string]Market)
	for _, market := range p.fixtureMarkets() {
		byID[market.AssetID] = market
	}

	quotes := make([]Quote, 0, len(assetIDs))
	for _, assetID := range assetIDs {
		market, ok := byID[assetID]
		if !ok {
			return nil, fmt.Errorf("unknown asset id %q", assetID)
		}
		quotes = append(quotes, Quote{
			AssetID:    market.AssetID,
			Symbol:     market.Symbol,
			PriceCents: market.PriceCents,
			ChangeBPS:  market.ChangeBPS,
			Source:     market.Source,
			UpdatedAt:  market.UpdatedAt,
		})
	}

	return quotes, nil
}

func (p *MockProvider) fixtureMarkets() []Market {
	updatedAt := p.now()
	return []Market{
		{
			AssetID:    "crypto:btc",
			Symbol:     "BTC",
			Name:       "Bitcoin",
			Kind:       AssetKindCrypto,
			Source:     "mock",
			PriceCents: 6_142_020,
			ChangeBPS:  280,
			UpdatedAt:  updatedAt,
		},
		{
			AssetID:    "etf:spy",
			Symbol:     "SPY",
			Name:       "S&P 500 ETF",
			Kind:       AssetKindETF,
			Source:     "mock",
			PriceCents: 54_618,
			ChangeBPS:  40,
			UpdatedAt:  updatedAt,
		},
		{
			AssetID:    "commodity:gld",
			Symbol:     "GLD",
			Name:       "Gold Trust",
			Kind:       AssetKindCommodity,
			Source:     "mock",
			PriceCents: 21_492,
			ChangeBPS:  -20,
			UpdatedAt:  updatedAt,
		},
		{
			AssetID:    "event:pmkt",
			Symbol:     "PMKT",
			Name:       "Polymarket event markets",
			Kind:       AssetKindEvent,
			Source:     "mock",
			PriceCents: 62,
			ChangeBPS:  0,
			UpdatedAt:  updatedAt,
		},
		{
			AssetID:    "event:lolesports-t1",
			Symbol:     "LOL-T1",
			Name:       "LoL Esports: T1 match winner",
			Kind:       AssetKindEvent,
			Source:     "mock",
			PriceCents: 64,
			ChangeBPS:  180,
			UpdatedAt:  updatedAt,
		},
		{
			AssetID:    "event:lolesports-geng",
			Symbol:     "LOL-GEN",
			Name:       "LoL Esports: Gen.G match winner",
			Kind:       AssetKindEvent,
			Source:     "mock",
			PriceCents: 41,
			ChangeBPS:  -120,
			UpdatedAt:  updatedAt,
		},
	}
}
