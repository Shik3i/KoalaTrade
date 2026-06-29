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

const defaultFinnhubBaseURL = "https://finnhub.io/api/v1"

type FinnhubProvider struct {
	client   *http.Client
	baseURL  string
	apiKey   string
	fallback Provider
	assets   map[string]string
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

	return &FinnhubProvider{
		client:   &http.Client{Timeout: timeout},
		baseURL:  strings.TrimRight(baseURL, "/"),
		apiKey:   strings.TrimSpace(apiKey),
		fallback: fallback,
		assets: map[string]string{
			"etf:spy":       "SPY",
			"commodity:gld": "GLD",
		},
	}
}

func (p *FinnhubProvider) Markets(ctx context.Context) ([]Market, error) {
	markets, err := p.fallback.Markets(ctx)
	if err != nil {
		return nil, err
	}
	if p.apiKey == "" {
		return markets, nil
	}

	ids := make([]string, 0, len(p.assets))
	for assetID := range p.assets {
		ids = append(ids, assetID)
	}
	quotes, err := p.fetchQuotes(ctx, ids)
	if err != nil {
		return markets, nil
	}

	byID := make(map[string]Quote, len(quotes))
	for _, quote := range quotes {
		byID[quote.AssetID] = quote
	}
	for index, market := range markets {
		quote, ok := byID[market.AssetID]
		if !ok {
			continue
		}
		markets[index].Source = quote.Source
		markets[index].PriceCents = quote.PriceCents
		markets[index].ChangeBPS = quote.ChangeBPS
		markets[index].UpdatedAt = quote.UpdatedAt
	}
	return markets, nil
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
