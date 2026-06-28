package marketdata

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestCoinGeckoProviderQuotes(t *testing.T) {
	var sawAPIKey bool
	provider := NewCoinGeckoProvider("https://coingecko.test/api/v3", "demo-key", time.Second, NewMockProvider())
	provider.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/v3/simple/price" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("ids"); got != "bitcoin" {
			t.Fatalf("unexpected ids %q", got)
		}
		if got := r.URL.Query().Get("vs_currencies"); got != "usd" {
			t.Fatalf("unexpected vs_currencies %q", got)
		}
		sawAPIKey = r.Header.Get("x-cg-demo-api-key") == "demo-key"
		return jsonResponse(http.StatusOK, `{"bitcoin":{"usd":62345.67,"usd_24h_change":1.23,"last_updated_at":1700000000}}`), nil
	})

	quotes, err := provider.Quotes(context.Background(), []string{"crypto:btc"})
	if err != nil {
		t.Fatalf("quotes: %v", err)
	}
	if !sawAPIKey {
		t.Fatal("expected demo api key header")
	}
	if len(quotes) != 1 {
		t.Fatalf("expected 1 quote, got %d", len(quotes))
	}
	if quotes[0].PriceCents != 6_234_567 {
		t.Fatalf("expected converted price cents, got %d", quotes[0].PriceCents)
	}
	if quotes[0].ChangeBPS != 123 {
		t.Fatalf("expected converted bps, got %d", quotes[0].ChangeBPS)
	}
	if quotes[0].Source != "coingecko" {
		t.Fatalf("expected coingecko source, got %q", quotes[0].Source)
	}
}

func TestCoinGeckoProviderFallsBackOnLiveError(t *testing.T) {
	provider := NewCoinGeckoProvider("https://coingecko.test/api/v3", "", time.Second, NewMockProvider())
	provider.client.Transport = roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusTooManyRequests, `{"error":"nope"}`), nil
	})
	quotes, err := provider.Quotes(context.Background(), []string{"crypto:btc"})
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

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	}
}
