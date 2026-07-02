package server

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Shik3i/KoalaTrade/backend/internal/marketdata"
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

// handleMarketHistory serves the real historical quotes from the SQLite database
// aggregated into OHLCV candles matching the requested range.
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

	// Calculate cutoff based on range
	var cutoff time.Time
	now := time.Now().UTC()
	switch rng {
	case "1H":
		cutoff = now.Add(-time.Hour)
	case "1D":
		cutoff = now.Add(-24 * time.Hour)
	case "1W":
		cutoff = now.Add(-7 * 24 * time.Hour)
	case "1M":
		cutoff = now.Add(-30 * 24 * time.Hour)
	case "1Y":
		cutoff = now.Add(-365 * 24 * time.Hour)
	default:
		cutoff = now.Add(-24 * time.Hour)
	}

	// Load quotes from SQLite database
	historyQuotes, err := s.db.GetHistory(r.Context(), assetID, cutoff)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "history unavailable: " + err.Error()})
		return
	}

	// Aggregate raw quotes into OHLCV candles
	candles := aggregateCandles(historyQuotes, spec.step)

	// Fetch current live price and anchor the final candle
	markets, err := s.marketData.Markets(r.Context())
	if err == nil {
		var price int64
		found := false
		for _, m := range markets {
			if m.AssetID == assetID {
				price = m.PriceCents
				found = true
				break
			}
		}
		if found && price > 0 {
			if len(candles) > 0 {
				last := &candles[len(candles)-1]
				if now.Sub(last.Time) < spec.step {
					last.Close = price
					if last.High < price {
						last.High = price
					}
					if last.Low > price {
						last.Low = price
					}
				} else {
					candles = append(candles, candle{
						Time:   now.Truncate(spec.step),
						Open:   last.Close,
						High:   price,
						Low:    price,
						Close:  price,
						Volume: 100,
					})
				}
			}
		}
	}

	writeJSON(w, http.StatusOK, historyResponse{
		AssetID: assetID,
		Range:   rng,
		Candles: candles,
	})
}

func aggregateCandles(quotes []marketdata.Quote, step time.Duration) []candle {
	if len(quotes) == 0 {
		return []candle{}
	}

	var candles []candle
	var currentCandle *candle
	var bucketStart time.Time

	for _, q := range quotes {
		t := q.UpdatedAt.Truncate(step)

		if currentCandle == nil || t.After(bucketStart) {
			if currentCandle != nil {
				candles = append(candles, *currentCandle)
			}
			bucketStart = t
			currentCandle = &candle{
				Time:   t,
				Open:   q.PriceCents,
				High:   q.PriceCents,
				Low:    q.PriceCents,
				Close:  q.PriceCents,
				Volume: 100,
			}
		} else {
			if q.PriceCents > currentCandle.High {
				currentCandle.High = q.PriceCents
			}
			if q.PriceCents < currentCandle.Low {
				currentCandle.Low = q.PriceCents
			}
			currentCandle.Close = q.PriceCents
			currentCandle.Volume += 100
		}
	}

	if currentCandle != nil {
		candles = append(candles, *currentCandle)
	}

	return candles
}
