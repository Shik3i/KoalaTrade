package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Shik3i/KoalaTrade/backend/internal/config"
	"github.com/Shik3i/KoalaTrade/backend/internal/storage"
)

func TestHealthz(t *testing.T) {
	db, err := storage.OpenSQLite(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	app := New(config.Config{
		AppName:                "KoalaTrade",
		DatabasePath:           "test.db",
		Port:                   8080,
		Environment:            "test",
		StartingCashCents:      1_000_000,
		MarketDataProvider:     "mock",
		MarketDataCacheSeconds: 60,
	}, db)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	res := httptest.NewRecorder()
	app.Routes().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}
	if got := res.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("expected security header, got %q", got)
	}
}

func TestMarkets(t *testing.T) {
	db, err := storage.OpenSQLite(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	app := New(config.Config{
		AppName:                "KoalaTrade",
		DatabasePath:           "test.db",
		Port:                   8080,
		Environment:            "test",
		StartingCashCents:      1_000_000,
		MarketDataProvider:     "mock",
		MarketDataCacheSeconds: 60,
	}, db)

	req := httptest.NewRequest(http.MethodGet, "/api/markets", nil)
	res := httptest.NewRecorder()
	app.Routes().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}
	if body := res.Body.String(); !strings.Contains(body, `"assetId":"crypto:btc"`) {
		t.Fatalf("expected mock BTC market, got %s", body)
	}
}
