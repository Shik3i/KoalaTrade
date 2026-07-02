package esports

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestSlugDiagnosticUsesTemporaryMapping(t *testing.T) {
	service := NewService("", "", "https://polymarket.test", time.Second, time.Minute, &slugTestStore{})
	service.http = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if !strings.Contains(r.URL.RawQuery, "lol-es1-g2-2026-07-02") {
			return jsonResponse(`[]`), nil
		}
		return jsonResponse(`[{
			"slug":"lol-es1-g2-2026-07-02",
			"markets":[{
				"question":"Who will win?",
				"groupItemTitle":"Match Winner",
				"sportsMarketType":"moneyline",
				"outcomes":"[\"Eintracht Spandau\",\"G2 Esports\"]",
				"outcomePrices":"[\"0.42\",\"0.58\"]"
			}]
		}]`), nil
	})}
	service.cache = []Match{{
		ID:        "match-1",
		StartTime: time.Date(2026, 7, 2, 18, 0, 0, 0, time.UTC),
		League:    "LEC",
		Team1:     Team{Name: "Eintracht Spandau", Code: "EINS"},
		Team2:     Team{Name: "G2 Esports", Code: "G2"},
	}}

	diag, err := service.SlugDiagnostic(context.Background(), "match-1", "EINS", "ES1", true)
	if err != nil {
		t.Fatalf("slug diagnostic: %v", err)
	}
	if !diag.Found {
		t.Fatal("expected diagnostic to find mapped Polymarket event")
	}
	if diag.EventSlug != "lol-es1-g2-2026-07-02" {
		t.Fatalf("expected mapped event slug, got %q", diag.EventSlug)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

type slugTestStore struct{}

func (s *slugTestStore) GetMeta(ctx context.Context, key string) (string, bool, error) {
	return "", false, ctx.Err()
}

func (s *slugTestStore) SetMeta(ctx context.Context, key, value string) error {
	return ctx.Err()
}

func (s *slugTestStore) TeamMappingsMap(ctx context.Context) (map[string]string, error) {
	return nil, ctx.Err()
}
