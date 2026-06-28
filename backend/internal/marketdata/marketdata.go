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

type Service struct {
	provider Provider
	ttl      time.Duration
	mu       sync.RWMutex
	cache    map[string]Quote
}

func NewService(provider Provider, ttl time.Duration) *Service {
	return &Service{
		provider: provider,
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

	return markets, nil
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
		fresh, err := s.provider.Quotes(ctx, missing)
		if err != nil {
			return nil, err
		}

		cachedUntil := now.Add(s.ttl)
		s.mu.Lock()
		for _, quote := range fresh {
			quote.CachedUntil = cachedUntil
			s.cache[quote.AssetID] = quote
			quotes = append(quotes, quote)
		}
		s.mu.Unlock()
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
