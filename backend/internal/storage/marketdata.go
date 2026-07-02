package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/Shik3i/KoalaTrade/backend/internal/marketdata"
	"github.com/jmoiron/sqlx"
)

type quoteRow struct {
	AssetID     string `db:"asset_id"`
	Symbol      string `db:"symbol"`
	PriceCents  int64  `db:"price_cents"`
	ChangeBPS   int64  `db:"change_bps"`
	Source      string `db:"source"`
	UpdatedAt   string `db:"updated_at"`
	CachedUntil string `db:"cached_until"`
}

func (s *SQLite) UpsertMarkets(ctx context.Context, markets []marketdata.Market) error {
	for _, market := range markets {
		_, err := s.db.ExecContext(ctx, `INSERT INTO assets (
			id, kind, symbol, name, currency, provider, provider_ref, active, updated_at
		) VALUES (?, ?, ?, ?, 'USD', ?, ?, 1, ?)
		ON CONFLICT(id) DO UPDATE SET
			kind = excluded.kind,
			symbol = excluded.symbol,
			name = excluded.name,
			provider = excluded.provider,
			provider_ref = excluded.provider_ref,
			active = 1,
			updated_at = excluded.updated_at`,
			market.AssetID,
			string(market.Kind),
			market.Symbol,
			market.Name,
			market.Source,
			market.AssetID,
			formatTime(market.UpdatedAt),
		)
		if err != nil {
			return fmt.Errorf("upsert asset %s: %w", market.AssetID, err)
		}
	}
	return nil
}

func (s *SQLite) FreshQuotes(ctx context.Context, assetIDs []string, now time.Time) ([]marketdata.Quote, error) {
	if len(assetIDs) == 0 {
		return nil, nil
	}

	query, args, err := sqlx.In(`SELECT asset_id, symbol, price_cents, change_bps, source, updated_at, cached_until
		FROM asset_quotes
		WHERE asset_id IN (?) AND cached_until > ?`, assetIDs, formatTime(now))
	if err != nil {
		return nil, fmt.Errorf("build fresh quote query: %w", err)
	}

	var rows []quoteRow
	if err := s.db.SelectContext(ctx, &rows, s.db.Rebind(query), args...); err != nil {
		return nil, fmt.Errorf("select fresh quotes: %w", err)
	}

	quotes := make([]marketdata.Quote, 0, len(rows))
	for _, row := range rows {
		updatedAt, err := parseTime(row.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("parse quote updated_at: %w", err)
		}
		cachedUntil, err := parseTime(row.CachedUntil)
		if err != nil {
			return nil, fmt.Errorf("parse quote cached_until: %w", err)
		}
		quotes = append(quotes, marketdata.Quote{
			AssetID:     row.AssetID,
			Symbol:      row.Symbol,
			PriceCents:  row.PriceCents,
			ChangeBPS:   row.ChangeBPS,
			Source:      row.Source,
			UpdatedAt:   updatedAt,
			CachedUntil: cachedUntil,
		})
	}

	return quotes, nil
}

func (s *SQLite) LatestQuotes(ctx context.Context, assetIDs []string) ([]marketdata.Quote, error) {
	if len(assetIDs) == 0 {
		return nil, nil
	}

	query, args, err := sqlx.In(`SELECT asset_id, symbol, price_cents, change_bps, source, updated_at, cached_until
		FROM asset_quotes
		WHERE asset_id IN (?)`, assetIDs)
	if err != nil {
		return nil, fmt.Errorf("build latest quote query: %w", err)
	}

	var rows []quoteRow
	if err := s.db.SelectContext(ctx, &rows, s.db.Rebind(query), args...); err != nil {
		return nil, fmt.Errorf("select latest quotes: %w", err)
	}

	quotes := make([]marketdata.Quote, 0, len(rows))
	for _, row := range rows {
		updatedAt, err := parseTime(row.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("parse quote updated_at: %w", err)
		}
		cachedUntil, err := parseTime(row.CachedUntil)
		if err != nil {
			return nil, fmt.Errorf("parse quote cached_until: %w", err)
		}
		quotes = append(quotes, marketdata.Quote{
			AssetID:     row.AssetID,
			Symbol:      row.Symbol,
			PriceCents:  row.PriceCents,
			ChangeBPS:   row.ChangeBPS,
			Source:      row.Source,
			UpdatedAt:   updatedAt,
			CachedUntil: cachedUntil,
		})
	}

	return quotes, nil
}

func (s *SQLite) StoreQuotes(ctx context.Context, quotes []marketdata.Quote) error {
	for _, quote := range quotes {
		_, err := s.db.ExecContext(ctx, `INSERT INTO asset_quotes (
			asset_id, symbol, price_cents, change_bps, source, updated_at, cached_until
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(asset_id) DO UPDATE SET
			symbol = excluded.symbol,
			price_cents = CASE WHEN excluded.price_cents > 0 THEN excluded.price_cents ELSE asset_quotes.price_cents END,
			change_bps = CASE WHEN excluded.price_cents > 0 THEN excluded.change_bps ELSE asset_quotes.change_bps END,
			source = CASE WHEN excluded.price_cents > 0 THEN excluded.source ELSE asset_quotes.source END,
			updated_at = CASE WHEN excluded.price_cents > 0 THEN excluded.updated_at ELSE asset_quotes.updated_at END,
			cached_until = excluded.cached_until`,
			quote.AssetID,
			quote.Symbol,
			quote.PriceCents,
			quote.ChangeBPS,
			quote.Source,
			formatTime(quote.UpdatedAt),
			formatTime(quote.CachedUntil),
		)
		if err != nil {
			return fmt.Errorf("store quote %s: %w", quote.AssetID, err)
		}

		// Only store history if price is greater than 0
		if quote.PriceCents > 0 {
			_, err = s.db.ExecContext(ctx, `INSERT OR IGNORE INTO asset_history (
				asset_id, timestamp, price_cents
			) VALUES (?, ?, ?)`,
				quote.AssetID,
				formatTime(quote.UpdatedAt),
				quote.PriceCents,
			)
			if err != nil {
				return fmt.Errorf("store quote history %s: %w", quote.AssetID, err)
			}
		}
	}

	// Compress and prune: Delete records older than 30 days
	cutoff := time.Now().UTC().AddDate(0, 0, -30)
	_, _ = s.db.ExecContext(ctx, `DELETE FROM asset_history WHERE timestamp < ?`, formatTime(cutoff))

	return nil
}

func (s *SQLite) GetHistory(ctx context.Context, assetID string, cutoff time.Time) ([]marketdata.Quote, error) {
	var rows []struct {
		AssetID    string `db:"asset_id"`
		Timestamp  string `db:"timestamp"`
		PriceCents int64  `db:"price_cents"`
	}
	err := s.db.SelectContext(ctx, &rows, `
		SELECT asset_id, timestamp, price_cents
		FROM asset_history
		WHERE asset_id = ? AND timestamp >= ?
		ORDER BY timestamp ASC`,
		assetID, formatTime(cutoff),
	)
	if err != nil {
		return nil, fmt.Errorf("select history: %w", err)
	}

	quotes := make([]marketdata.Quote, len(rows))
	for i, r := range rows {
		ts, err := parseTime(r.Timestamp)
		if err != nil {
			return nil, fmt.Errorf("parse timestamp: %w", err)
		}
		quotes[i] = marketdata.Quote{
			AssetID:    r.AssetID,
			PriceCents: r.PriceCents,
			UpdatedAt:  ts,
		}
	}
	return quotes, nil
}

func formatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func parseTime(value string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, value)
}
