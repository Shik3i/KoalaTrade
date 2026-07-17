package storage

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/Shik3i/KoalaTrade/backend/internal/marketdata"
)

// LeaderboardPortfolio is a ranked (account-owned) portfolio reduced to what the
// leaderboard needs: identity + cash + positions to value.
type LeaderboardPortfolio struct {
	UserID            string
	DisplayName       string
	Username          string
	StartingCashCents int64
	CashCents         int64
	Positions         []PortfolioPosition
}

type leaderboardRow struct {
	ID                string `db:"id"`
	UserID            string `db:"user_id"`
	DisplayName       string `db:"display_name"`
	Username          string `db:"username"`
	StartingCashCents int64  `db:"starting_cash_cents"`
	CashCents         int64  `db:"cash_cents"`
}

type leaderboardPositionRow struct {
	PortfolioID   string               `db:"portfolio_id"`
	AssetID       string               `db:"asset_id"`
	Symbol        string               `db:"symbol"`
	Name          string               `db:"name"`
	Kind          marketdata.AssetKind `db:"kind"`
	QuantityMicro int64                `db:"quantity_micro"`
	LastPriceCents int64               `db:"last_price_cents"`
}

// LeaderboardPortfolios returns one ranked portfolio per account (the
// 'local-default' competition portfolio of every non-disabled user), with its
// positions loaded. Anonymous client-only portfolios are excluded — ranking is
// accounts-only. Uses two queries (rows, then batched positions).
func (s *SQLite) LeaderboardPortfolios(ctx context.Context) ([]LeaderboardPortfolio, error) {
	var rows []leaderboardRow
	if err := s.db.SelectContext(ctx, &rows, `
		SELECT p.id, p.user_id,
		       COALESCE(u.display_name, '') AS display_name,
		       COALESCE(u.username, '')     AS username,
		       p.starting_cash_cents, p.cash_cents
		FROM portfolios p
		JOIN user_profiles u ON u.id = p.user_id
		WHERE p.user_id IS NOT NULL AND p.user_id != ''
		  AND p.client_portfolio_id = 'local-default'
		  AND COALESCE(u.disabled, 0) = 0`); err != nil {
		return nil, fmt.Errorf("leaderboard portfolios: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}

	out := make([]LeaderboardPortfolio, len(rows))
	byID := make(map[string]*LeaderboardPortfolio, len(rows))
	ids := make([]string, len(rows))
	for i, r := range rows {
		out[i] = LeaderboardPortfolio{
			UserID:            r.UserID,
			DisplayName:       r.DisplayName,
			Username:          r.Username,
			StartingCashCents: r.StartingCashCents,
			CashCents:         r.CashCents,
		}
		byID[r.ID] = &out[i]
		ids[i] = r.ID
	}

	query, args, err := sqlx.In(`SELECT portfolio_id, asset_id, symbol, name, kind,
		quantity_micro, last_price_cents
		FROM portfolio_positions WHERE portfolio_id IN (?)`, ids)
	if err != nil {
		return nil, fmt.Errorf("leaderboard positions query: %w", err)
	}
	query = s.db.Rebind(query)

	var posRows []leaderboardPositionRow
	if err := s.db.SelectContext(ctx, &posRows, query, args...); err != nil {
		return nil, fmt.Errorf("leaderboard positions: %w", err)
	}
	for _, pr := range posRows {
		lp, ok := byID[pr.PortfolioID]
		if !ok {
			continue
		}
		lp.Positions = append(lp.Positions, PortfolioPosition{
			AssetID:        pr.AssetID,
			Symbol:         pr.Symbol,
			Name:           pr.Name,
			Kind:           pr.Kind,
			QuantityMicro:  pr.QuantityMicro,
			LastPriceCents: pr.LastPriceCents,
		})
	}

	return out, nil
}
