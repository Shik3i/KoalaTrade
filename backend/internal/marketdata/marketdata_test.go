package marketdata

import (
	"context"
	"testing"
	"time"
)

func TestServiceQuotesDedupsAndNeverCallsProvider(t *testing.T) {
	now := time.Now().UTC()
	provider := &countingProvider{}
	store := &memoryStore{quotes: []Quote{{
		AssetID:     "crypto:btc",
		Symbol:      "BTC",
		PriceCents:  6_200_000,
		Source:      "sqlite",
		UpdatedAt:   now,
		CachedUntil: now.Add(time.Minute),
	}}}
	service := NewService(provider, time.Minute, store)

	first, err := service.Quotes(context.Background(), []string{"crypto:btc", "crypto:btc"})
	if err != nil {
		t.Fatalf("first quotes: %v", err)
	}
	if len(first) != 1 {
		t.Fatalf("expected duplicate asset ids to collapse, got %d quotes", len(first))
	}
	if first[0].CachedUntil.IsZero() {
		t.Fatal("expected cached until to be set")
	}

	second, err := service.Quotes(context.Background(), []string{"crypto:btc"})
	if err != nil {
		t.Fatalf("second quotes: %v", err)
	}
	if !second[0].CachedUntil.Equal(first[0].CachedUntil) {
		t.Fatal("expected cached quote to keep cached-until timestamp")
	}

	// Regression guard for the 502/poller-starvation bug: the read path must
	// serve stored quotes only and NEVER call the provider. The background
	// poller (Refresh) is the sole live fetcher, so a burst of quote reads can
	// never stampede the provider or saturate the shared rate limiter.
	if provider.quoteCalls != 0 {
		t.Fatalf("read path must not call the provider, got %d calls", provider.quoteCalls)
	}
}

func TestServiceQuotesUsesPersistentCacheBeforeProvider(t *testing.T) {
	now := time.Now().UTC()
	store := &memoryStore{
		quotes: []Quote{
			{
				AssetID:     "crypto:btc",
				Symbol:      "BTC",
				PriceCents:  6_200_000,
				ChangeBPS:   110,
				Source:      "sqlite",
				UpdatedAt:   now,
				CachedUntil: now.Add(time.Minute),
			},
		},
	}
	provider := &countingProvider{}
	service := NewService(provider, time.Minute, store)

	quotes, err := service.Quotes(context.Background(), []string{"crypto:btc"})
	if err != nil {
		t.Fatalf("quotes: %v", err)
	}
	if len(quotes) != 1 {
		t.Fatalf("expected 1 quote, got %d", len(quotes))
	}
	if quotes[0].Source != "sqlite" {
		t.Fatalf("expected sqlite cache source, got %q", quotes[0].Source)
	}
	if provider.quoteCalls != 0 {
		t.Fatalf("expected provider not to be called, got %d calls", provider.quoteCalls)
	}
}

func TestServiceRefreshAllStoresFreshQuotes(t *testing.T) {
	store := &memoryStore{}
	service := NewService(NewRegistryProvider(), time.Minute, store)

	quotes, err := service.RefreshAll(context.Background())
	if err != nil {
		t.Fatalf("refresh all: %v", err)
	}
	expectedLen := len(NewRegistryProvider().fixtureMarkets())
	if len(quotes) != expectedLen {
		t.Fatalf("expected %d quotes, got %d", expectedLen, len(quotes))
	}
	if len(store.quotes) != expectedLen {
		t.Fatalf("expected stored %d quotes, got %d", expectedLen, len(store.quotes))
	}
	for _, quote := range quotes {
		if quote.CachedUntil.IsZero() {
			t.Fatal("expected cached until to be set")
		}
	}
}

type countingProvider struct {
	quoteCalls int
}

func (p *countingProvider) Markets(ctx context.Context) ([]Market, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, nil
}

func (p *countingProvider) Quotes(ctx context.Context, assetIDs []string) ([]Quote, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p.quoteCalls++
	return nil, nil
}

type memoryStore struct {
	quotes []Quote
}

func (s *memoryStore) UpsertMarkets(ctx context.Context, markets []Market) error {
	return ctx.Err()
}

func (s *memoryStore) FreshQuotes(ctx context.Context, assetIDs []string, now time.Time) ([]Quote, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	byID := make(map[string]Quote, len(s.quotes))
	for _, quote := range s.quotes {
		if quote.CachedUntil.After(now) {
			byID[quote.AssetID] = quote
		}
	}

	quotes := make([]Quote, 0, len(assetIDs))
	for _, assetID := range assetIDs {
		if quote, ok := byID[assetID]; ok {
			quotes = append(quotes, quote)
		}
	}
	return quotes, nil
}

func (s *memoryStore) LatestQuotes(ctx context.Context, assetIDs []string) ([]Quote, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	byID := make(map[string]Quote, len(s.quotes))
	for _, quote := range s.quotes {
		byID[quote.AssetID] = quote
	}

	quotes := make([]Quote, 0, len(assetIDs))
	for _, assetID := range assetIDs {
		if quote, ok := byID[assetID]; ok {
			quotes = append(quotes, quote)
		}
	}
	return quotes, nil
}

func (s *memoryStore) StoreQuotes(ctx context.Context, quotes []Quote) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.quotes = append(s.quotes, quotes...)
	return nil
}
