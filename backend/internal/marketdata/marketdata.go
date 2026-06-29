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

type Store interface {
	UpsertMarkets(ctx context.Context, markets []Market) error
	FreshQuotes(ctx context.Context, assetIDs []string, now time.Time) ([]Quote, error)
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

		fresh, err := s.provider.Quotes(ctx, missing)
		if err != nil {
			return nil, err
		}

		cachedUntil := now.Add(s.ttl)
		cachedFresh := make([]Quote, 0, len(fresh))
		s.mu.Lock()
		for _, quote := range fresh {
			quote.CachedUntil = cachedUntil
			s.cache[quote.AssetID] = quote
			quotes = append(quotes, quote)
			cachedFresh = append(cachedFresh, quote)
		}
		s.mu.Unlock()

		if s.store != nil {
			_ = s.store.StoreQuotes(ctx, cachedFresh)
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
