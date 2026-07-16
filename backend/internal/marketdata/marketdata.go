package marketdata

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type AssetKind string

const (
	AssetKindStock     AssetKind = "stock"
	AssetKindETF       AssetKind = "etf"
	AssetKindCrypto    AssetKind = "crypto"
	AssetKindCommodity AssetKind = "commodity"
	AssetKindEvent     AssetKind = "event"
)

type Market struct {
	AssetID    string    `json:"assetId"`
	Symbol     string    `json:"symbol"`
	Name       string    `json:"name"`
	Kind       AssetKind `json:"kind"`
	Source     string    `json:"source"`
	PriceCents int64     `json:"priceCents"`
	ChangeBPS  int64     `json:"changeBps"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type Quote struct {
	AssetID     string    `json:"assetId"`
	Symbol      string    `json:"symbol"`
	PriceCents  int64     `json:"priceCents"`
	ChangeBPS   int64     `json:"changeBps"`
	Source      string    `json:"source"`
	UpdatedAt   time.Time `json:"updatedAt"`
	CachedUntil time.Time `json:"cachedUntil"`
}

type Provider interface {
	Markets(ctx context.Context) ([]Market, error)
	Quotes(ctx context.Context, assetIDs []string) ([]Quote, error)
}

// HistoricalPricer is implemented by providers that can supply historical price
// series for backfilling long-range charts.
type HistoricalPricer interface {
	HistoricalPrices(ctx context.Context, assetID string, days int) ([]HistoricalPoint, error)
}

type Store interface {
	UpsertMarkets(ctx context.Context, markets []Market) error
	FreshQuotes(ctx context.Context, assetIDs []string, now time.Time) ([]Quote, error)
	LatestQuotes(ctx context.Context, assetIDs []string) ([]Quote, error)
	StoreQuotes(ctx context.Context, quotes []Quote) error
}

type Service struct {
	provider Provider
	store    Store
	ttl      time.Duration
	mu       sync.RWMutex
	cache    map[string]Quote
}

func NewService(provider Provider, ttl time.Duration, stores ...Store) *Service {
	var store Store
	if len(stores) > 0 {
		store = stores[0]
	}

	return &Service{
		provider: provider,
		store:    store,
		ttl:      ttl,
		cache:    make(map[string]Quote),
	}
}

func (s *Service) Markets(ctx context.Context) ([]Market, error) {
	markets, err := s.provider.Markets(ctx)
	if err != nil {
		return nil, err
	}

	sort.Slice(markets, func(i, j int) bool {
		return markets[i].Symbol < markets[j].Symbol
	})

	if s.store != nil {
		_ = s.store.UpsertMarkets(ctx, markets)

		// Load latest quotes from database to populate prices/change rates
		assetIDs := make([]string, len(markets))
		for i, m := range markets {
			assetIDs[i] = m.AssetID
		}

		storedQuotes, err := s.store.LatestQuotes(ctx, assetIDs)
		if err == nil {
			quotesMap := make(map[string]Quote, len(storedQuotes))
			for _, q := range storedQuotes {
				quotesMap[q.AssetID] = q
			}

			s.mu.Lock()
			for i, m := range markets {
				if q, ok := quotesMap[m.AssetID]; ok {
					markets[i].PriceCents = q.PriceCents
					markets[i].ChangeBPS = q.ChangeBPS
					markets[i].UpdatedAt = q.UpdatedAt

					// Warm memory cache if empty
					if _, cacheOk := s.cache[m.AssetID]; !cacheOk {
						s.cache[m.AssetID] = q
					}
				}
			}
			s.mu.Unlock()
		}
	}

	return markets, nil
}

func (s *Service) RefreshAll(ctx context.Context) ([]Quote, error) {
	markets, err := s.Markets(ctx)
	if err != nil {
		return nil, err
	}

	assetIDs := make([]string, 0, len(markets))
	for _, market := range markets {
		assetIDs = append(assetIDs, market.AssetID)
	}
	return s.Refresh(ctx, assetIDs)
}

func (s *Service) Refresh(ctx context.Context, assetIDs []string) ([]Quote, error) {
	normalized := normalizeAssetIDs(assetIDs)
	if len(normalized) == 0 {
		return nil, errors.New("at least one asset id is required")
	}

	fresh, err := s.provider.Quotes(ctx, normalized)
	if err != nil {
		return nil, err
	}

	cachedUntil := time.Now().UTC().Add(s.ttl)
	s.mu.Lock()
	for index := range fresh {
		fresh[index].CachedUntil = cachedUntil
		s.cache[fresh[index].AssetID] = fresh[index]
	}
	s.mu.Unlock()

	if s.store != nil {
		_ = s.store.StoreQuotes(ctx, fresh)
	}
	return fresh, nil
}

func (s *Service) Quotes(ctx context.Context, assetIDs []string) ([]Quote, error) {
	normalized := normalizeAssetIDs(assetIDs)
	if len(normalized) == 0 {
		return nil, errors.New("at least one asset id is required")
	}

	now := time.Now().UTC()
	quotes := make([]Quote, 0, len(normalized))
	missing := make([]string, 0)

	s.mu.RLock()
	for _, assetID := range normalized {
		quote, ok := s.cache[assetID]
		if ok && quote.CachedUntil.After(now) {
			quotes = append(quotes, quote)
			continue
		}
		missing = append(missing, assetID)
	}
	s.mu.RUnlock()

	if len(missing) > 0 {
		if s.store != nil {
			stored, err := s.store.FreshQuotes(ctx, missing, now)
			if err == nil {
				cachedUntilByID := make(map[string]Quote, len(stored))
				for _, quote := range stored {
					cachedUntilByID[quote.AssetID] = quote
				}

				stillMissing := make([]string, 0, len(missing))
				s.mu.Lock()
				for _, assetID := range missing {
					quote, ok := cachedUntilByID[assetID]
					if !ok {
						stillMissing = append(stillMissing, assetID)
						continue
					}
					s.cache[assetID] = quote
					quotes = append(quotes, quote)
				}
				s.mu.Unlock()
				missing = stillMissing
			}
		}

		if len(missing) == 0 {
			sort.Slice(quotes, func(i, j int) bool {
				return quotes[i].Symbol < quotes[j].Symbol
			})
			return quotes, nil
		}

		// Read path is intentionally DB-only. The background poller (Refresh) is
		// the sole live fetcher — staggered under the per-minute rate limit — so a
		// burst of quote requests can never stampede the provider, saturate the
		// shared limiter, or blow the 30s request deadline. (The old fallthrough
		// here synchronously live-fetched *every* stale symbol at once; under a
		// cold cache that meant ~129 serialized provider calls per request →
		// request timeouts, 502s, and starvation of the poller waiting on the
		// same limiter.) Serve the latest stored quote even if past its freshness
		// window; assets the poller hasn't reached yet have none and render as
		// "no feed" upstream — honest, and never blocking.
		if s.store != nil {
			if stored, storedErr := s.store.LatestQuotes(ctx, missing); storedErr == nil {
				quotes = append(quotes, stored...)
			}
		}
	}

	sort.Slice(quotes, func(i, j int) bool {
		return quotes[i].Symbol < quotes[j].Symbol
	})

	return quotes, nil
}

func normalizeAssetIDs(assetIDs []string) []string {
	seen := make(map[string]struct{}, len(assetIDs))
	normalized := make([]string, 0, len(assetIDs))
	for _, assetID := range assetIDs {
		trimmed := strings.TrimSpace(assetID)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}
	return normalized
}
