package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/Shik3i/KoalaTrade/backend/internal/marketdata"
)

// OpenOrder is a pending Limit/Stop order held server-side. It is evaluated by
// the background engine against live quotes and filled when its trigger is met —
// so it executes even when the user's browser is closed.
type OpenOrder struct {
	ID                string
	PortfolioID       string
	AssetID           string
	Symbol            string
	Name              string
	Kind              marketdata.AssetKind
	Side              string
	OrderType         string // "limit" | "stop"
	QuantityMicro     int64
	TriggerPriceCents int64
	CreatedAt         time.Time
}

type openOrderRow struct {
	ID                string               `db:"id"`
	PortfolioID       string               `db:"portfolio_id"`
	AssetID           string               `db:"asset_id"`
	Symbol            string               `db:"symbol"`
	Name              string               `db:"name"`
	Kind              marketdata.AssetKind `db:"kind"`
	Side              string               `db:"side"`
	OrderType         string               `db:"order_type"`
	QuantityMicro     int64                `db:"quantity_micro"`
	TriggerPriceCents int64                `db:"trigger_price_cents"`
	CreatedAt         string               `db:"created_at"`
}

func (r openOrderRow) toOpenOrder() OpenOrder {
	createdAt, _ := parseTime(r.CreatedAt)
	return OpenOrder{
		ID:                r.ID,
		PortfolioID:       r.PortfolioID,
		AssetID:           r.AssetID,
		Symbol:            r.Symbol,
		Name:              r.Name,
		Kind:              r.Kind,
		Side:              r.Side,
		OrderType:         r.OrderType,
		QuantityMicro:     r.QuantityMicro,
		TriggerPriceCents: r.TriggerPriceCents,
		CreatedAt:         createdAt,
	}
}

func (s *SQLite) CreateOpenOrder(ctx context.Context, order OpenOrder) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO open_orders (
		id, portfolio_id, asset_id, symbol, name, kind, side, order_type,
		quantity_micro, trigger_price_cents, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		order.ID, order.PortfolioID, order.AssetID, order.Symbol, order.Name,
		string(order.Kind), order.Side, order.OrderType, order.QuantityMicro,
		order.TriggerPriceCents, formatTime(order.CreatedAt),
	)
	if err != nil {
		return fmt.Errorf("create open order: %w", err)
	}
	return nil
}

func (s *SQLite) OpenOrdersByPortfolio(ctx context.Context, portfolioID string) ([]OpenOrder, error) {
	var rows []openOrderRow
	if err := s.db.SelectContext(ctx, &rows,
		`SELECT * FROM open_orders WHERE portfolio_id = ? ORDER BY created_at DESC`, portfolioID); err != nil {
		return nil, fmt.Errorf("list open orders: %w", err)
	}
	return toOpenOrders(rows), nil
}

// AllOpenOrders returns every pending order across all portfolios — the input
// to the evaluation engine.
func (s *SQLite) AllOpenOrders(ctx context.Context) ([]OpenOrder, error) {
	var rows []openOrderRow
	if err := s.db.SelectContext(ctx, &rows,
		`SELECT * FROM open_orders ORDER BY created_at ASC`); err != nil {
		return nil, fmt.Errorf("list all open orders: %w", err)
	}
	return toOpenOrders(rows), nil
}

// DeleteOpenOrder removes a pending order scoped to its portfolio (so one user
// can't cancel another's). Returns whether a row was deleted.
func (s *SQLite) DeleteOpenOrder(ctx context.Context, id, portfolioID string) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM open_orders WHERE id = ? AND portfolio_id = ?`, id, portfolioID)
	if err != nil {
		return false, fmt.Errorf("delete open order: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// DeleteOpenOrderByID removes a pending order by id only — used by the engine
// after it fills (or drops) the order.
func (s *SQLite) DeleteOpenOrderByID(ctx context.Context, id string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM open_orders WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete open order by id: %w", err)
	}
	return nil
}

func toOpenOrders(rows []openOrderRow) []OpenOrder {
	orders := make([]OpenOrder, 0, len(rows))
	for _, row := range rows {
		orders = append(orders, row.toOpenOrder())
	}
	return orders
}

// PortfolioIDsWithEventPositions returns the ids of every portfolio currently
// holding an eSports event position — the input to the settlement sweep.
func (s *SQLite) PortfolioIDsWithEventPositions(ctx context.Context) ([]string, error) {
	var ids []string
	if err := s.db.SelectContext(ctx, &ids,
		`SELECT DISTINCT portfolio_id FROM portfolio_positions WHERE asset_id LIKE 'event:%'`); err != nil {
		return nil, fmt.Errorf("list event-position portfolios: %w", err)
	}
	return ids, nil
}
