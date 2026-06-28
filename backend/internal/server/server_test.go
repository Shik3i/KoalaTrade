package server

import (
	"bytes"
	"encoding/json"
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
		MarketDataHTTPTimeout:  5,
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
		MarketDataHTTPTimeout:  5,
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

func TestPortfolioSyncRoundTrip(t *testing.T) {
	app := newTestServer(t)
	body := `{
		"id":"local-default",
		"schemaVersion":1,
		"startingCashCents":1000000,
		"cashCents":500000,
		"positions":[{
			"assetId":"crypto:btc",
			"symbol":"BTC",
			"name":"Bitcoin",
			"kind":"crypto",
			"quantity":0.5,
			"averageCostCents":6000000,
			"lastPriceCents":6200000,
			"updatedAt":"2026-06-29T14:00:00Z"
		}],
		"transactions":[{
			"id":"tx-12345678",
			"assetId":"crypto:btc",
			"symbol":"BTC",
			"side":"buy",
			"quantity":0.5,
			"priceCents":6000000,
			"feeCents":0,
			"status":"local",
			"createdAt":"2026-06-29T14:00:00Z"
		}],
		"createdAt":"2026-06-29T14:00:00Z",
		"updatedAt":"2026-06-29T14:00:00Z"
	}`

	put := httptest.NewRequest(http.MethodPut, "/api/sync/portfolio", bytes.NewBufferString(body))
	put.Header.Set("Content-Type", "application/json")
	put.Header.Set("X-Koala-Client-ID", "client-12345678")
	putRes := httptest.NewRecorder()
	app.Routes().ServeHTTP(putRes, put)

	if putRes.Code != http.StatusOK {
		t.Fatalf("expected PUT status %d, got %d body=%s", http.StatusOK, putRes.Code, putRes.Body.String())
	}
	var putPayload struct {
		Portfolio struct {
			Transactions []struct {
				Status string `json:"status"`
			} `json:"transactions"`
		} `json:"portfolio"`
	}
	if err := json.Unmarshal(putRes.Body.Bytes(), &putPayload); err != nil {
		t.Fatalf("decode put payload: %v", err)
	}
	if putPayload.Portfolio.Transactions[0].Status != "synced" {
		t.Fatalf("expected synced transaction, got %q", putPayload.Portfolio.Transactions[0].Status)
	}

	get := httptest.NewRequest(http.MethodGet, "/api/sync/portfolio?id=local-default", nil)
	get.Header.Set("X-Koala-Client-ID", "client-12345678")
	getRes := httptest.NewRecorder()
	app.Routes().ServeHTTP(getRes, get)

	if getRes.Code != http.StatusOK {
		t.Fatalf("expected GET status %d, got %d body=%s", http.StatusOK, getRes.Code, getRes.Body.String())
	}
	if got := getRes.Body.String(); !strings.Contains(got, `"cashCents":500000`) {
		t.Fatalf("expected synced portfolio body, got %s", got)
	}
}

func TestPortfolioSyncRequiresClientScope(t *testing.T) {
	app := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/sync/portfolio?id=local-default", nil)
	res := httptest.NewRecorder()
	app.Routes().ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected missing client id status %d, got %d", http.StatusBadRequest, res.Code)
	}
}

func newTestServer(t *testing.T) *Server {
	t.Helper()

	db, err := storage.OpenSQLite(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	return New(config.Config{
		AppName:                "KoalaTrade",
		DatabasePath:           "test.db",
		Port:                   8080,
		Environment:            "test",
		StartingCashCents:      1_000_000,
		MarketDataProvider:     "mock",
		MarketDataCacheSeconds: 60,
		MarketDataHTTPTimeout:  5,
	}, db)
}
