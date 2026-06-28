package marketdata

import (
	"context"
	"testing"
	"time"
)

func TestServiceQuotesCachesProviderResults(t *testing.T) {
	provider := NewMockProvider()
	service := NewService(provider, time.Minute)

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
}
