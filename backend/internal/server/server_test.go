package server

import (
	"bytes"
	"context"
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

func TestRegisterLoginMeUsesSessionCookie(t *testing.T) {
	app := newTestServer(t)
	router := app.Routes()

	register := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewBufferString(`{"username":"TraderOne","password":"long-enough-password"}`))
	register.Header.Set("Content-Type", "application/json")
	registerRes := httptest.NewRecorder()
	router.ServeHTTP(registerRes, register)
	if registerRes.Code != http.StatusOK {
		t.Fatalf("expected register status %d, got %d body=%s", http.StatusOK, registerRes.Code, registerRes.Body.String())
	}
	cookie := sessionCookie(t, registerRes)
	if !cookie.HttpOnly {
		t.Fatal("expected session cookie to be HttpOnly")
	}

	me := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	me.AddCookie(cookie)
	meRes := httptest.NewRecorder()
	router.ServeHTTP(meRes, me)
	if meRes.Code != http.StatusOK {
		t.Fatalf("expected me status %d, got %d body=%s", http.StatusOK, meRes.Code, meRes.Body.String())
	}
	if got := meRes.Body.String(); !strings.Contains(got, `"username":"traderone"`) || !strings.Contains(got, `"role":"user"`) {
		t.Fatalf("expected normalized user in me payload, got %s", got)
	}

	login := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(`{"username":"traderone","password":"long-enough-password"}`))
	login.Header.Set("Content-Type", "application/json")
	loginRes := httptest.NewRecorder()
	router.ServeHTTP(loginRes, login)
	if loginRes.Code != http.StatusOK {
		t.Fatalf("expected login status %d, got %d body=%s", http.StatusOK, loginRes.Code, loginRes.Body.String())
	}
}

func TestRegisterHonorsRegistrationToggle(t *testing.T) {
	app := newTestServer(t)
	if err := app.db.SetMeta(context.Background(), registrationOpenKey, "false"); err != nil {
		t.Fatalf("set registration toggle: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewBufferString(`{"username":"closed","password":"long-enough-password"}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	app.Routes().ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("expected register status %d, got %d body=%s", http.StatusForbidden, res.Code, res.Body.String())
	}
}

func TestAuthenticatedPortfolioSyncCanRestoreByUser(t *testing.T) {
	app := newTestServer(t)
	router := app.Routes()

	register := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewBufferString(`{"username":"syncuser","password":"long-enough-password"}`))
	register.Header.Set("Content-Type", "application/json")
	registerRes := httptest.NewRecorder()
	router.ServeHTTP(registerRes, register)
	cookie := sessionCookie(t, registerRes)

	body := `{
		"id":"local-default",
		"schemaVersion":1,
		"startingCashCents":1000000,
		"cashCents":880000,
		"positions":[],
		"transactions":[],
		"createdAt":"2026-06-29T14:00:00Z",
		"updatedAt":"2026-06-29T14:00:00Z"
	}`
	put := httptest.NewRequest(http.MethodPut, "/api/sync/portfolio", bytes.NewBufferString(body))
	put.Header.Set("Content-Type", "application/json")
	put.Header.Set("X-Koala-Client-ID", "client-12345678")
	put.AddCookie(cookie)
	putRes := httptest.NewRecorder()
	router.ServeHTTP(putRes, put)
	if putRes.Code != http.StatusOK {
		t.Fatalf("expected PUT status %d, got %d body=%s", http.StatusOK, putRes.Code, putRes.Body.String())
	}

	get := httptest.NewRequest(http.MethodGet, "/api/sync/portfolio?id=local-default", nil)
	get.AddCookie(cookie)
	getRes := httptest.NewRecorder()
	router.ServeHTTP(getRes, get)
	if getRes.Code != http.StatusOK {
		t.Fatalf("expected account GET status %d, got %d body=%s", http.StatusOK, getRes.Code, getRes.Body.String())
	}
	if got := getRes.Body.String(); !strings.Contains(got, `"cashCents":880000`) {
		t.Fatalf("expected account portfolio body, got %s", got)
	}
}

func TestAccountManagement(t *testing.T) {
	app := newTestServer(t)
	router := app.Routes()
	cookie := registerUser(t, router, "accountuser", "long-enough-password")

	update := httptest.NewRequest(http.MethodPatch, "/api/account/", bytes.NewBufferString(`{"displayName":"Captain Paper"}`))
	update.Header.Set("Content-Type", "application/json")
	update.AddCookie(cookie)
	updateRes := httptest.NewRecorder()
	router.ServeHTTP(updateRes, update)
	if updateRes.Code != http.StatusOK {
		t.Fatalf("expected update status %d, got %d body=%s", http.StatusOK, updateRes.Code, updateRes.Body.String())
	}
	if got := updateRes.Body.String(); !strings.Contains(got, `"displayName":"Captain Paper"`) {
		t.Fatalf("expected updated display name, got %s", got)
	}

	change := httptest.NewRequest(http.MethodPut, "/api/account/password", bytes.NewBufferString(`{"currentPassword":"long-enough-password","newPassword":"new-long-enough-password"}`))
	change.Header.Set("Content-Type", "application/json")
	change.AddCookie(cookie)
	changeRes := httptest.NewRecorder()
	router.ServeHTTP(changeRes, change)
	if changeRes.Code != http.StatusOK {
		t.Fatalf("expected password status %d, got %d body=%s", http.StatusOK, changeRes.Code, changeRes.Body.String())
	}

	oldLogin := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(`{"username":"accountuser","password":"long-enough-password"}`))
	oldLogin.Header.Set("Content-Type", "application/json")
	oldLoginRes := httptest.NewRecorder()
	router.ServeHTTP(oldLoginRes, oldLogin)
	if oldLoginRes.Code != http.StatusUnauthorized {
		t.Fatalf("expected old password to fail with %d, got %d", http.StatusUnauthorized, oldLoginRes.Code)
	}

	newLogin := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(`{"username":"accountuser","password":"new-long-enough-password"}`))
	newLogin.Header.Set("Content-Type", "application/json")
	newLoginRes := httptest.NewRecorder()
	router.ServeHTTP(newLoginRes, newLogin)
	if newLoginRes.Code != http.StatusOK {
		t.Fatalf("expected new password login status %d, got %d body=%s", http.StatusOK, newLoginRes.Code, newLoginRes.Body.String())
	}
}

func TestAccountExportAndDeletePortfolioData(t *testing.T) {
	app := newTestServer(t)
	router := app.Routes()
	cookie := registerUser(t, router, "exportuser", "long-enough-password")

	body := `{
		"id":"local-default",
		"schemaVersion":1,
		"startingCashCents":1000000,
		"cashCents":770000,
		"positions":[],
		"transactions":[],
		"createdAt":"2026-06-29T14:00:00Z",
		"updatedAt":"2026-06-29T14:00:00Z"
	}`
	put := httptest.NewRequest(http.MethodPut, "/api/sync/portfolio", bytes.NewBufferString(body))
	put.Header.Set("Content-Type", "application/json")
	put.Header.Set("X-Koala-Client-ID", "client-12345678")
	put.AddCookie(cookie)
	putRes := httptest.NewRecorder()
	router.ServeHTTP(putRes, put)
	if putRes.Code != http.StatusOK {
		t.Fatalf("expected sync status %d, got %d body=%s", http.StatusOK, putRes.Code, putRes.Body.String())
	}

	exportReq := httptest.NewRequest(http.MethodGet, "/api/account/export", nil)
	exportReq.AddCookie(cookie)
	exportRes := httptest.NewRecorder()
	router.ServeHTTP(exportRes, exportReq)
	if exportRes.Code != http.StatusOK {
		t.Fatalf("expected export status %d, got %d body=%s", http.StatusOK, exportRes.Code, exportRes.Body.String())
	}
	if got := exportRes.Body.String(); !strings.Contains(got, `"username":"exportuser"`) || !strings.Contains(got, `"cashCents":770000`) {
		t.Fatalf("expected exported user portfolio, got %s", got)
	}

	del := httptest.NewRequest(http.MethodDelete, "/api/account/portfolio-data", bytes.NewBufferString(`{"password":"long-enough-password"}`))
	del.Header.Set("Content-Type", "application/json")
	del.AddCookie(cookie)
	delRes := httptest.NewRecorder()
	router.ServeHTTP(delRes, del)
	if delRes.Code != http.StatusOK {
		t.Fatalf("expected delete portfolio status %d, got %d body=%s", http.StatusOK, delRes.Code, delRes.Body.String())
	}

	get := httptest.NewRequest(http.MethodGet, "/api/sync/portfolio?id=local-default", nil)
	get.AddCookie(cookie)
	getRes := httptest.NewRecorder()
	router.ServeHTTP(getRes, get)
	if getRes.Code != http.StatusNotFound {
		t.Fatalf("expected deleted account portfolio status %d, got %d body=%s", http.StatusNotFound, getRes.Code, getRes.Body.String())
	}
}

func TestDeleteAccountClearsSession(t *testing.T) {
	app := newTestServer(t)
	router := app.Routes()
	cookie := registerUser(t, router, "deleteuser", "long-enough-password")

	req := httptest.NewRequest(http.MethodDelete, "/api/account/", bytes.NewBufferString(`{"password":"long-enough-password"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected delete account status %d, got %d body=%s", http.StatusOK, res.Code, res.Body.String())
	}

	me := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	me.AddCookie(cookie)
	meRes := httptest.NewRecorder()
	router.ServeHTTP(meRes, me)
	if meRes.Code != http.StatusUnauthorized {
		t.Fatalf("expected deleted account session status %d, got %d", http.StatusUnauthorized, meRes.Code)
	}
}

func sessionCookie(t *testing.T, res *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, cookie := range res.Result().Cookies() {
		if cookie.Name == sessionCookieName {
			return cookie
		}
	}
	t.Fatalf("expected %s cookie, got %v", sessionCookieName, res.Result().Cookies())
	return nil
}

func registerUser(t *testing.T, router http.Handler, username, password string) *http.Cookie {
	t.Helper()
	register := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewBufferString(`{"username":"`+username+`","password":"`+password+`"}`))
	register.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, register)
	if res.Code != http.StatusOK {
		t.Fatalf("expected register status %d, got %d body=%s", http.StatusOK, res.Code, res.Body.String())
	}
	return sessionCookie(t, res)
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
