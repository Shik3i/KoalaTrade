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

const defaultYahooBaseURL = "https://query1.finance.yahoo.com"

// yahooUserAgent is required — Yahoo's chart endpoint rejects requests without a
// browser-like User-Agent.
const yahooUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36"

// yahooRequestsPerMinute conservatively caps requests to Yahoo's undocumented
// chart endpoint. It has no published limit but throttles sustained high rates;
// 30/min (0.5/s) stays comfortably clear.
const yahooRequestsPerMinute = 30

// YahooProvider serves keyless live quotes and historical prices for equities
// (stocks, ETFs, commodities) from Yahoo Finance's public chart endpoint. Crypto
// and anything else it delegates to its fallback. It requires no API key, which
// makes the full 129-asset catalogue work out of the box.
type YahooProvider struct {
	client   *http.Client
	baseURL  string
	fallback Provider
	assets   map[string]string // assetID -> Yahoo symbol
	limiter  *RateLimiter
}

type yahooChart struct {
	Chart struct {
		Result []struct {
			Meta struct {
				RegularMarketPrice float64 `json:"regularMarketPrice"`
				PreviousClose      float64 `json:"previousClose"`
				ChartPreviousClose float64 `json:"chartPreviousClose"`
				RegularMarketTime  int64   `json:"regularMarketTime"`
			} `json:"meta"`
			Timestamp  []int64 `json:"timestamp"`
			Indicators struct {
				Quote []struct {
					Close []*float64 `json:"close"`
				} `json:"quote"`
			} `json:"indicators"`
		} `json:"result"`
		Error interface{} `json:"error"`
	} `json:"chart"`
}

func NewYahooProvider(baseURL string, timeout time.Duration, fallback Provider) *YahooProvider {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultYahooBaseURL
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	// Derive the assetID→symbol map from the catalogue (same equity kinds Finnhub
	// serves), converting ticker dots to Yahoo's dash convention (BRK.B → BRK-B).
	assets := map[string]string{}
	if catalog, err := fallback.Markets(context.Background()); err == nil {
		for _, market := range catalog {
			if _, ok := finnhubKinds[market.Kind]; ok {
				assets[market.AssetID] = yahooSymbol(market.Symbol)
			}
		}
	}

	return &YahooProvider{
		client:   &http.Client{Timeout: timeout},
		baseURL:  strings.TrimRight(baseURL, "/"),
		fallback: fallback,
		assets:   assets,
		limiter:  NewRateLimiter(yahooRequestsPerMinute),
	}
}

func yahooSymbol(symbol string) string {
	return strings.ReplaceAll(strings.TrimSpace(symbol), ".", "-")
}

// Markets returns the catalogue only; prices are populated via Quotes/Refresh.
func (p *YahooProvider) Markets(ctx context.Context) ([]Market, error) {
	return p.fallback.Markets(ctx)
}

func (p *YahooProvider) Quotes(ctx context.Context, assetIDs []string) ([]Quote, error) {
	liveIDs := make([]string, 0, len(assetIDs))
	fallbackIDs := make([]string, 0, len(assetIDs))
	for _, assetID := range assetIDs {
		if _, ok := p.assets[assetID]; ok {
			liveIDs = append(liveIDs, assetID)
			continue
		}
		fallbackIDs = append(fallbackIDs, assetID)
	}

	quotes := make([]Quote, 0, len(assetIDs))
	for _, assetID := range liveIDs {
		quote, err := p.fetchQuote(ctx, assetID, p.assets[assetID])
		if err != nil {
			// Fall back to the downstream provider for this asset (registry).
			fallbackIDs = append(fallbackIDs, assetID)
			continue
		}
		quotes = append(quotes, quote)
	}

	if len(fallbackIDs) > 0 {
		fallbackQuotes, err := p.fallback.Quotes(ctx, fallbackIDs)
		if err != nil {
			if len(quotes) > 0 {
				return quotes, nil
			}
			return nil, err
		}
		quotes = append(quotes, fallbackQuotes...)
	}
	return quotes, nil
}

func (p *YahooProvider) fetchQuote(ctx context.Context, assetID, symbol string) (Quote, error) {
	chart, err := p.fetchChart(ctx, symbol, "1d", "1d")
	if err != nil {
		return Quote{}, err
	}
	if len(chart.Chart.Result) == 0 {
		return Quote{}, fmt.Errorf("yahoo returned no result for %s", symbol)
	}
	meta := chart.Chart.Result[0].Meta
	price := meta.RegularMarketPrice
	if price <= 0 {
		return Quote{}, fmt.Errorf("yahoo returned empty price for %s", symbol)
	}

	prev := meta.PreviousClose
	if prev <= 0 {
		prev = meta.ChartPreviousClose
	}
	var changeBPS int64
	if prev > 0 {
		changeBPS = int64(math.Round((price/prev - 1) * 10_000))
	}

	updatedAt := time.Now().UTC()
	if meta.RegularMarketTime > 0 {
		updatedAt = time.Unix(meta.RegularMarketTime, 0).UTC()
	}

	return Quote{
		AssetID:    assetID,
		Symbol:     symbol,
		PriceCents: int64(math.Round(price * 100)),
		ChangeBPS:  changeBPS,
		Source:     "yahoo",
		UpdatedAt:  updatedAt,
	}, nil
}

// HistoricalPrices fetches historical closes for the last `days` days, mapped to
// a Yahoo range/interval. Implements the HistoricalPricer interface.
func (p *YahooProvider) HistoricalPrices(ctx context.Context, assetID string, days int) ([]HistoricalPoint, error) {
	symbol, ok := p.assets[assetID]
	if !ok {
		return nil, fmt.Errorf("unsupported Yahoo asset id %q", assetID)
	}
	rng, interval := yahooRangeInterval(days)

	chart, err := p.fetchChart(ctx, symbol, rng, interval)
	if err != nil {
		return nil, err
	}
	if len(chart.Chart.Result) == 0 {
		return nil, fmt.Errorf("yahoo returned no result for %s", symbol)
	}
	result := chart.Chart.Result[0]
	if len(result.Indicators.Quote) == 0 {
		return nil, nil
	}
	closes := result.Indicators.Quote[0].Close

	points := make([]HistoricalPoint, 0, len(result.Timestamp))
	for i, ts := range result.Timestamp {
		if i >= len(closes) || closes[i] == nil {
			continue
		}
		price := *closes[i]
		if price <= 0 {
			continue
		}
		points = append(points, HistoricalPoint{
			Timestamp:  time.Unix(ts, 0).UTC(),
			PriceCents: int64(math.Round(price * 100)),
		})
	}
	return points, nil
}

func (p *YahooProvider) fetchChart(ctx context.Context, symbol, rng, interval string) (yahooChart, error) {
	if err := p.limiter.Wait(ctx); err != nil {
		return yahooChart{}, err
	}

	endpoint, err := url.Parse(p.baseURL + "/v8/finance/chart/" + url.PathEscape(symbol))
	if err != nil {
		return yahooChart{}, fmt.Errorf("parse yahoo endpoint: %w", err)
	}
	query := endpoint.Query()
	query.Set("interval", interval)
	query.Set("range", rng)
	endpoint.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return yahooChart{}, fmt.Errorf("build yahoo request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", yahooUserAgent)

	response, err := p.client.Do(request)
	if err != nil {
		return yahooChart{}, fmt.Errorf("fetch yahoo chart: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return yahooChart{}, fmt.Errorf("yahoo returned status %d", response.StatusCode)
	}

	var payload yahooChart
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return yahooChart{}, fmt.Errorf("decode yahoo chart: %w", err)
	}
	return payload, nil
}

// yahooRangeInterval maps a lookback in days to a Yahoo (range, interval) pair.
// Yahoo caps intraday history (≈60d for 5m, ≈730d for 1h), so coarser ranges use
// daily bars.
func yahooRangeInterval(days int) (string, string) {
	switch {
	case days <= 1:
		return "1d", "5m"
	case days <= 7:
		return "5d", "60m"
	case days <= 31:
		return "1mo", "60m"
	case days <= 95:
		return "3mo", "1d"
	case days <= 370:
		return "1y", "1d"
	case days <= 740:
		return "2y", "1d"
	default:
		return "5y", "1d"
	}
}
