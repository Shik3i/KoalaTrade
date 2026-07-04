package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// finnhubKinds are the asset kinds Finnhub serves quotes for. Crypto is handled
// by CoinGecko, so it is excluded here.
var finnhubKinds = map[AssetKind]struct{}{
	AssetKindStock:     {},
	AssetKindETF:       {},
	AssetKindCommodity: {},
}

const defaultFinnhubBaseURL = "https://finnhub.io/api/v1"

// finnhubRequestsPerMinute stays under the free-tier limit of 60/min.
const finnhubRequestsPerMinute = 50

type FinnhubProvider struct {
	client   *http.Client
	baseURL  string
	apiKey   string
	fallback Provider
	assets   map[string]string
	limiter  *RateLimiter
}

type finnhubQuote struct {
	Current      float64 `json:"c"`
	PercentDelta float64 `json:"dp"`
	Timestamp    int64   `json:"t"`
}

func NewFinnhubProvider(baseURL, apiKey string, timeout time.Duration, fallback Provider) *FinnhubProvider {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultFinnhubBaseURL
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	// Derive the assetID→symbol map from the catalogue (single source of truth),
	// so adding a stock/ETF/commodity only requires editing the registry.
	assets := map[string]string{}
	if catalog, err := fallback.Markets(context.Background()); err == nil {
		for _, market := range catalog {
			if _, ok := finnhubKinds[market.Kind]; ok {
				assets[market.AssetID] = market.Symbol
			}
		}
	}

	return &FinnhubProvider{
		client:   &http.Client{Timeout: timeout},
		baseURL:  strings.TrimRight(baseURL, "/"),
		apiKey:   strings.TrimSpace(apiKey),
		fallback: fallback,
		assets:   assets,
		limiter:  NewRateLimiter(finnhubRequestsPerMinute),
	}
}

// Markets returns the asset catalogue only. Live prices are populated exclusively
// via Quotes/Refresh (the staggered poller), never on the read path, so opening
// the app or loading a chart never triggers a burst of provider requests.
func (p *FinnhubProvider) Markets(ctx context.Context) ([]Market, error) {
	return p.fallback.Markets(ctx)
}

func (p *FinnhubProvider) Quotes(ctx context.Context, assetIDs []string) ([]Quote, error) {
	liveIDs := make([]string, 0, len(assetIDs))
	fallbackIDs := make([]string, 0, len(assetIDs))
	for _, assetID := range assetIDs {
		if _, ok := p.assets[assetID]; ok && p.apiKey != "" {
			liveIDs = append(liveIDs, assetID)
			continue
		}
		fallbackIDs = append(fallbackIDs, assetID)
	}

	quotes := make([]Quote, 0, len(assetIDs))
	if len(liveIDs) > 0 {
		liveQuotes, err := p.fetchQuotes(ctx, liveIDs)
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

func (p *FinnhubProvider) fetchQuotes(ctx context.Context, assetIDs []string) ([]Quote, error) {
	quotes := make([]Quote, 0, len(assetIDs))
	for _, assetID := range assetIDs {
		symbol, ok := p.assets[assetID]
		if !ok {
			return nil, fmt.Errorf("unsupported Finnhub asset id %q", assetID)
		}

		quote, err := p.fetchQuote(ctx, assetID, symbol)
		if err != nil {
			return nil, err
		}
		quotes = append(quotes, quote)
	}
	return quotes, nil
}

func (p *FinnhubProvider) fetchQuote(ctx context.Context, assetID, symbol string) (Quote, error) {
	endpoint, err := url.Parse(p.baseURL + "/quote")
	if err != nil {
		return Quote{}, fmt.Errorf("parse finnhub endpoint: %w", err)
	}
	query := endpoint.Query()
	query.Set("symbol", symbol)
	query.Set("token", p.apiKey)
	endpoint.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return Quote{}, fmt.Errorf("build finnhub request: %w", err)
	}
	request.Header.Set("Accept", "application/json")

	if err := p.limiter.Wait(ctx); err != nil {
		return Quote{}, err
	}
	response, err := p.client.Do(request)
	if err != nil {
		return Quote{}, fmt.Errorf("fetch finnhub quote: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Quote{}, fmt.Errorf("finnhub returned status %d", response.StatusCode)
	}

	var payload finnhubQuote
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return Quote{}, fmt.Errorf("decode finnhub quote: %w", err)
	}
	if payload.Current <= 0 {
		return Quote{}, fmt.Errorf("finnhub returned empty price for %s", symbol)
	}

	updatedAt := time.Now().UTC()
	if payload.Timestamp > 0 {
		updatedAt = time.Unix(payload.Timestamp, 0).UTC()
	}

	return Quote{
		AssetID:    assetID,
		Symbol:     symbol,
		PriceCents: int64(math.Round(payload.Current * 100)),
		ChangeBPS:  int64(math.Round(payload.PercentDelta * 100)),
		Source:     "finnhub",
		UpdatedAt:  updatedAt,
	}, nil
}
