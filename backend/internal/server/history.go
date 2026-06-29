package server

import (
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

type candle struct {
	Time   time.Time `json:"time"`
	Open   int64     `json:"open"`
	High   int64     `json:"high"`
	Low    int64     `json:"low"`
	Close  int64     `json:"close"`
	Volume int64     `json:"volume"`
}

type historyResponse struct {
	AssetID string   `json:"assetId"`
	Range   string   `json:"range"`
	Candles []candle `json:"candles"`
}

type rangeSpec struct {
	points int
	step   time.Duration
	vol    float64
}

var historyRanges = map[string]rangeSpec{
	"1H": {points: 60, step: time.Minute, vol: 0.0009},
	"1D": {points: 96, step: 15 * time.Minute, vol: 0.0016},
	"1W": {points: 168, step: time.Hour, vol: 0.0034},
	"1M": {points: 90, step: 8 * time.Hour, vol: 0.0052},
	"1Y": {points: 180, step: 48 * time.Hour, vol: 0.0089},
}

// handleMarketHistory serves a deterministic, mock OHLCV price history for a
// single asset. The series is anchored so its final close equals the asset's
// current price, which keeps the chart consistent with the live quote.
func (s *Server) handleMarketHistory(w http.ResponseWriter, r *http.Request) {
	// assetIds contain a colon (e.g. crypto:btc); the client percent-encodes it,
	// so decode the path param before matching.
	assetID, err := url.PathUnescape(chi.URLParam(r, "assetId"))
	if err != nil {
		assetID = chi.URLParam(r, "assetId")
	}
	rng := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("range")))
	spec, ok := historyRanges[rng]
	if !ok {
		rng = "1D"
		spec = historyRanges[rng]
	}

	markets, err := s.marketData.Markets(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "market data unavailable"})
		return
	}

	var price int64
	found := false
	for _, m := range markets {
		if m.AssetID == assetID {
			price = m.PriceCents
			found = true
			break
		}
	}
	if !found {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "unknown asset id"})
		return
	}

	writeJSON(w, http.StatusOK, historyResponse{
		AssetID: assetID,
		Range:   rng,
		Candles: generateCandles(assetID, rng, price, spec),
	})
}

func generateCandles(assetID, rng string, price int64, spec rangeSpec) []candle {
	if price <= 0 {
		price = 1
	}
	n := spec.points

	// xorshift64 seeded via FNV-1a over (assetID|range) so each asset/range
	// pair yields a stable but distinct walk across reloads.
	seed := uint64(1469598103934665603)
	for _, c := range assetID + "|" + rng {
		seed ^= uint64(c)
		seed *= 1099511628211
	}
	if seed == 0 {
		seed = 0x9e3779b97f4a7c15
	}
	next := func() float64 {
		seed ^= seed << 13
		seed ^= seed >> 7
		seed ^= seed << 17
		return float64(seed>>11) / float64(uint64(1)<<53)
	}

	drift := (next() - 0.5) * spec.vol * 0.4
	cum := 1.0
	cums := make([]float64, n)
	for i := 0; i < n; i++ {
		shock := (next()*2-1)*spec.vol + drift
		cum *= 1 + shock
		if cum < 0.2 {
			cum = 0.2
		}
		cums[i] = cum
	}

	cur := float64(price)
	endC := cums[n-1]
	closes := make([]float64, n)
	for i := 0; i < n; i++ {
		closes[i] = cur * cums[i] / endC
	}

	now := time.Now().UTC()
	candles := make([]candle, n)
	for i := 0; i < n; i++ {
		c := closes[i]
		o := c * (1 - (next()-0.5)*spec.vol)
		if i > 0 {
			o = closes[i-1]
		}
		hi := math.Max(o, c) * (1 + next()*spec.vol*0.8)
		lo := math.Min(o, c) * (1 - next()*spec.vol*0.8)
		candles[i] = candle{
			Time:   now.Add(-time.Duration(n-1-i) * spec.step),
			Open:   int64(math.Round(o)),
			High:   int64(math.Round(hi)),
			Low:    int64(math.Max(1, math.Round(lo))),
			Close:  int64(math.Round(c)),
			Volume: int64(1000 + next()*9000),
		}
	}

	// Anchor the final candle exactly to the live price.
	last := &candles[n-1]
	last.Close = price
	if last.High < price {
		last.High = price
	}
	if last.Low > price {
		last.Low = price
	}
	return candles
}
