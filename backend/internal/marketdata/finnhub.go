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
			"etf:spy": "SPY",
			"etf:qqq": "QQQ",
			"etf:dia": "DIA",
			"etf:iwm": "IWM",
			"etf:urth": "URTH",
			"etf:eem": "EEM",
			"etf:acwi": "ACWI",
			"etf:vgk": "VGK",
			"etf:vwo": "VWO",
			"etf:xlk": "XLK",
			"etf:xlf": "XLF",
			"etf:xlv": "XLV",
			"etf:xle": "XLE",
			"etf:tlt": "TLT",
			"etf:bnd": "BND",
			"commodity:gld": "GLD",
			"commodity:slv": "SLV",
			"commodity:pplt": "PPLT",
			"commodity:pall": "PALL",
			"commodity:uso": "USO",
			"commodity:ung": "UNG",
			"stock:aapl": "AAPL",
			"stock:msft": "MSFT",
			"stock:googl": "GOOGL",
			"stock:amzn": "AMZN",
			"stock:nvda": "NVDA",
			"stock:tsla": "TSLA",
			"stock:meta": "META",
			"stock:amd": "AMD",
			"stock:nflx": "NFLX",
			"stock:intc": "INTC",
			"stock:avgo": "AVGO",
			"stock:csco": "CSCO",
			"stock:orcl": "ORCL",
			"stock:adbe": "ADBE",
			"stock:crm": "CRM",
			"stock:qcom": "QCOM",
			"stock:txn": "TXN",
			"stock:mu": "MU",
			"stock:amat": "AMAT",
			"stock:lrcx": "LRCX",
			"stock:now": "NOW",
			"stock:panw": "PANW",
			"stock:ibm": "IBM",
			"stock:intu": "INTU",
			"stock:sony": "SONY",
			"stock:asml": "ASML",
			"stock:tsm": "TSM",
			"stock:jpm": "JPM",
			"stock:bac": "BAC",
			"stock:wfc": "WFC",
			"stock:ms": "MS",
			"stock:gs": "GS",
			"stock:c": "C",
			"stock:v": "V",
			"stock:ma": "MA",
			"stock:axp": "AXP",
			"stock:pypl": "PYPL",
			"stock:cof": "COF",
			"stock:blk": "BLK",
			"stock:schw": "SCHW",
			"stock:brk.b": "BRK.B",
			"stock:jnj": "JNJ",
			"stock:lly": "LLY",
			"stock:unh": "UNH",
			"stock:pfe": "PFE",
			"stock:mrk": "MRK",
			"stock:abbv": "ABBV",
			"stock:tmo": "TMO",
			"stock:abt": "ABT",
			"stock:dhr": "DHR",
			"stock:bmy": "BMY",
			"stock:amgn": "AMGN",
			"stock:gild": "GILD",
			"stock:wmt": "WMT",
			"stock:hd": "HD",
			"stock:mcd": "MCD",
			"stock:nke": "NKE",
			"stock:sbux": "SBUX",
			"stock:tgt": "TGT",
			"stock:low": "LOW",
			"stock:tjx": "TJX",
			"stock:bkng": "BKNG",
			"stock:pg": "PG",
			"stock:ko": "KO",
			"stock:pep": "PEP",
			"stock:cost": "COST",
			"stock:pm": "PM",
			"stock:mo": "MO",
			"stock:el": "EL",
			"stock:cl": "CL",
			"stock:ge": "GE",
			"stock:cat": "CAT",
			"stock:hon": "HON",
			"stock:ups": "UPS",
			"stock:fdx": "FDX",
			"stock:de": "DE",
			"stock:ba": "BA",
			"stock:lmt": "LMT",
			"stock:rtx": "RTX",
			"stock:xom": "XOM",
			"stock:cvx": "CVX",
			"stock:cop": "COP",
			"stock:slb": "SLB",
			"stock:dis": "DIS",
			"stock:cmcsa": "CMCSA",
			"stock:t": "T",
			"stock:vz": "VZ",
			"stock:spot": "SPOT",
			"stock:f": "F",
			"stock:gm": "GM",
			"stock:tm": "TM",
			"stock:uber": "UBER",
			"stock:abnb": "ABNB",
			"stock:sq": "SQ",
			"stock:coin": "COIN",
			"stock:pltr": "PLTR",
			"stock:snow": "SNOW",
			"stock:crwd": "CRWD",
			"stock:shop": "SHOP",
			"stock:nvax": "NVAX",		},
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
