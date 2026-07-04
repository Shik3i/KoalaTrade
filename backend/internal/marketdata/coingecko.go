package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultCoinGeckoBaseURL = "https://api.coingecko.com/api/v3"

// Requests/minute caps. The keyless public API is aggressively rate-limited; a
// Demo key lifts the ceiling to ~30/min (we stay a touch under).
const (
	coinGeckoKeylessPerMinute = 8
	coinGeckoDemoKeyPerMinute = 25
)

type CoinGeckoProvider struct {
	client   *http.Client
	baseURL  string
	apiKey   string
	fallback Provider
	assets   map[string]coinGeckoAsset
	limiter  *RateLimiter
}

type coinGeckoAsset struct {
	AssetID string
	CoinID  string
	Symbol  string
	Name    string
}

type coinGeckoPrice struct {
	USD             float64 `json:"usd"`
	USD24HChange    float64 `json:"usd_24h_change"`
	LastUpdatedUnix int64   `json:"last_updated_at"`
}

// HistoricalPoint is a single historical price observation used to backfill
// long-range charts.
type HistoricalPoint struct {
	Timestamp  time.Time
	PriceCents int64
}

type coinGeckoMarketChart struct {
	Prices [][]float64 `json:"prices"` // [ [unixMillis, price], ... ]
}

func NewCoinGeckoProvider(baseURL, apiKey string, timeout time.Duration, fallback Provider) *CoinGeckoProvider {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultCoinGeckoBaseURL
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	perMinute := coinGeckoKeylessPerMinute
	if strings.TrimSpace(apiKey) != "" {
		perMinute = coinGeckoDemoKeyPerMinute
	}

	return &CoinGeckoProvider{
		client: &http.Client{
			Timeout: timeout,
		},
		baseURL:  strings.TrimRight(baseURL, "/"),
		apiKey:   strings.TrimSpace(apiKey),
		fallback: fallback,
		limiter:  NewRateLimiter(perMinute),
		assets: map[string]coinGeckoAsset{
			"crypto:btc": {
				AssetID: "crypto:btc",
				CoinID:  "bitcoin",
				Symbol:  "BTC",
				Name:    "Bitcoin",
			},
			"crypto:eth": {
				AssetID: "crypto:eth",
				CoinID:  "ethereum",
				Symbol:  "ETH",
				Name:    "Ethereum",
			},
			"crypto:sol": {
				AssetID: "crypto:sol",
				CoinID:  "solana",
				Symbol:  "SOL",
				Name:    "Solana",
			},
			"crypto:bnb": {
				AssetID: "crypto:bnb",
				CoinID:  "binancecoin",
				Symbol:  "BNB",
				Name:    "Binance Coin",
			},
			"crypto:xrp": {
				AssetID: "crypto:xrp",
				CoinID:  "ripple",
				Symbol:  "XRP",
				Name:    "Ripple",
			},
			"crypto:ada": {
				AssetID: "crypto:ada",
				CoinID:  "cardano",
				Symbol:  "ADA",
				Name:    "Cardano",
			},
			"crypto:doge": {
				AssetID: "crypto:doge",
				CoinID:  "dogecoin",
				Symbol:  "DOGE",
				Name:    "Dogecoin",
			},
			"crypto:link": {
				AssetID: "crypto:link",
				CoinID:  "chainlink",
				Symbol:  "LINK",
				Name:    "Chainlink",
			},
		},
	}
}

// Markets returns the asset catalogue only. It deliberately does NOT fetch live
// prices: price enrichment happens exclusively through Quotes/Refresh (driven by
// the staggered poller), so read-path handlers never trigger a provider burst.
func (p *CoinGeckoProvider) Markets(ctx context.Context) ([]Market, error) {
	return p.fallback.Markets(ctx)
}

func (p *CoinGeckoProvider) Quotes(ctx context.Context, assetIDs []string) ([]Quote, error) {
	cryptoIDs := make([]string, 0, len(assetIDs))
	fallbackIDs := make([]string, 0, len(assetIDs))
	for _, assetID := range assetIDs {
		if _, ok := p.assets[assetID]; ok {
			cryptoIDs = append(cryptoIDs, assetID)
			continue
		}
		fallbackIDs = append(fallbackIDs, assetID)
	}

	quotes := make([]Quote, 0, len(assetIDs))
	if len(cryptoIDs) > 0 {
		liveQuotes, err := p.fetchQuotes(ctx, cryptoIDs)
		if err != nil {
			return p.fallback.Quotes(ctx, assetIDs)
		}
		quotes = append(quotes, liveQuotes...)
	}

	if len(fallbackIDs) > 0 {
		fallbackQuotes, err := p.fallback.Quotes(ctx, fallbackIDs)
		if err != nil {
			return nil, err
		}
		quotes = append(quotes, fallbackQuotes...)
	}

	return quotes, nil
}

func (p *CoinGeckoProvider) fetchQuotes(ctx context.Context, assetIDs []string) ([]Quote, error) {
	if len(assetIDs) == 0 {
		return nil, nil
	}

	coinIDs := make([]string, 0, len(assetIDs))
	assetByCoinID := make(map[string]coinGeckoAsset, len(assetIDs))
	for _, assetID := range assetIDs {
		asset, ok := p.assets[assetID]
		if !ok {
			return nil, fmt.Errorf("unsupported CoinGecko asset id %q", assetID)
		}
		coinIDs = append(coinIDs, asset.CoinID)
		assetByCoinID[asset.CoinID] = asset
	}

	endpoint, err := url.Parse(p.baseURL + "/simple/price")
	if err != nil {
		return nil, fmt.Errorf("parse coingecko endpoint: %w", err)
	}
	query := endpoint.Query()
	query.Set("ids", strings.Join(coinIDs, ","))
	query.Set("vs_currencies", "usd")
	query.Set("include_24hr_change", "true")
	query.Set("include_last_updated_at", "true")
	endpoint.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build coingecko request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	if p.apiKey != "" {
		request.Header.Set("x-cg-demo-api-key", p.apiKey)
	}

	if err := p.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	response, err := p.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("fetch coingecko prices: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("coingecko returned status %d", response.StatusCode)
	}

	var payload map[string]coinGeckoPrice
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode coingecko prices: %w", err)
	}

	quotes := make([]Quote, 0, len(assetIDs))
	for _, coinID := range coinIDs {
		asset := assetByCoinID[coinID]
		price, ok := payload[coinID]
		if !ok {
			return nil, fmt.Errorf("coingecko response missing %q", coinID)
		}

		updatedAt := time.Now().UTC()
		if price.LastUpdatedUnix > 0 {
			updatedAt = time.Unix(price.LastUpdatedUnix, 0).UTC()
		}

		quotes = append(quotes, Quote{
			AssetID:    asset.AssetID,
			Symbol:     asset.Symbol,
			PriceCents: int64(math.Round(price.USD * 100)),
			ChangeBPS:  int64(math.Round(price.USD24HChange * 100)),
			Source:     "coingecko",
			UpdatedAt:  updatedAt,
		})
	}

	return quotes, nil
}

// CryptoAssetIDs returns the asset ids this provider can serve historical data
// for (i.e. the crypto catalogue).
func (p *CoinGeckoProvider) CryptoAssetIDs() []string {
	ids := make([]string, 0, len(p.assets))
	for assetID := range p.assets {
		ids = append(ids, assetID)
	}
	return ids
}

// HistoricalPrices fetches the CoinGecko market chart for an asset over the last
// `days` days. CoinGecko auto-selects granularity: days=1 → 5-minutely, 2–90 →
// hourly, >90 → daily. Used to seed chart history on first run.
func (p *CoinGeckoProvider) HistoricalPrices(ctx context.Context, assetID string, days int) ([]HistoricalPoint, error) {
	asset, ok := p.assets[assetID]
	if !ok {
		return nil, fmt.Errorf("unsupported CoinGecko asset id %q", assetID)
	}

	endpoint, err := url.Parse(p.baseURL + "/coins/" + url.PathEscape(asset.CoinID) + "/market_chart")
	if err != nil {
		return nil, fmt.Errorf("parse coingecko market_chart endpoint: %w", err)
	}
	query := endpoint.Query()
	query.Set("vs_currency", "usd")
	query.Set("days", strconv.Itoa(days))
	endpoint.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build coingecko market_chart request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	if p.apiKey != "" {
		request.Header.Set("x-cg-demo-api-key", p.apiKey)
	}

	if err := p.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	response, err := p.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("fetch coingecko market_chart: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("coingecko market_chart returned status %d", response.StatusCode)
	}

	var payload coinGeckoMarketChart
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode coingecko market_chart: %w", err)
	}

	points := make([]HistoricalPoint, 0, len(payload.Prices))
	for _, pair := range payload.Prices {
		if len(pair) < 2 {
			continue
		}
		price := pair[1]
		if price <= 0 {
			continue
		}
		points = append(points, HistoricalPoint{
			Timestamp:  time.UnixMilli(int64(pair[0])).UTC(),
			PriceCents: int64(math.Round(price * 100)),
		})
	}
	return points, nil
}
