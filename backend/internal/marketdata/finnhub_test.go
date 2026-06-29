package marketdata

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestFinnhubProviderQuotes(t *testing.T) {
	var sawToken bool
	provider := NewFinnhubProvider("https://finnhub.test/api/v1", "demo-key", time.Second, NewMockProvider())
	provider.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/v1/quote" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("symbol"); got != "SPY" {
			t.Fatalf("unexpected symbol %q", got)
		}
		sawToken = r.URL.Query().Get("token") == "demo-key"
		return jsonResponse(http.StatusOK, `{"c":612.34,"dp":0.45,"t":1700000000}`), nil
	})

	quotes, err := provider.Quotes(context.Background(), []string{"etf:spy"})
	if err != nil {
		t.Fatalf("quotes: %v", err)
	}
	if !sawToken {
		t.Fatal("expected finnhub token query")
	}
	if len(quotes) != 1 {
		t.Fatalf("expected 1 quote, got %d", len(quotes))
	}
	if quotes[0].PriceCents != 61_234 {
		t.Fatalf("expected converted price cents, got %d", quotes[0].PriceCents)
	}
	if quotes[0].ChangeBPS != 45 {
		t.Fatalf("expected converted bps, got %d", quotes[0].ChangeBPS)
	}
	if quotes[0].Source != "finnhub" {
		t.Fatalf("expected finnhub source, got %q", quotes[0].Source)
	}
}

func TestFinnhubProviderFallsBackWithoutKey(t *testing.T) {
	provider := NewFinnhubProvider("https://finnhub.test/api/v1", "", time.Second, NewMockProvider())
	quotes, err := provider.Quotes(context.Background(), []string{"etf:spy"})
	if err != nil {
		t.Fatalf("quotes: %v", err)
	}
	if len(quotes) != 1 {
		t.Fatalf("expected 1 fallback quote, got %d", len(quotes))
	}
	if quotes[0].Source != "mock" {
		t.Fatalf("expected mock fallback source, got %q", quotes[0].Source)
	}
}
