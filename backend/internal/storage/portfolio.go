package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/Shik3i/KoalaTrade/backend/internal/marketdata"
)

type Portfolio struct {
	ID                string
	UserID            string
	ClientID          string
	ClientPortfolioID string
	SchemaVersion     int64
	StartingCashCents int64
	CashCents         int64
	Positions         []PortfolioPosition
	Transactions      []PortfolioTransaction
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type PortfolioPosition struct {
	AssetID          string
	Symbol           string
	Name             string
	Kind             marketdata.AssetKind
	QuantityMicro    int64
	AverageCostCents int64
	LastPriceCents   int64
	UpdatedAt        time.Time
}

type PortfolioTransaction struct {
	ID            string
	AssetID       string
	Symbol        string
	Side          string
	QuantityMicro int64
	PriceCents    int64
	FeeCents      int64
	Status        string
	CreatedAt     time.Time
}

type portfolioRow struct {
	ID                string `db:"id"`
	UserID            string `db:"user_id"`
	ClientID          string `db:"client_id"`
	ClientPortfolioID string `db:"client_portfolio_id"`
	SchemaVersion     int64  `db:"schema_version"`
	StartingCashCents int64  `db:"starting_cash_cents"`
	CashCents         int64  `db:"cash_cents"`
	CreatedAt         string `db:"created_at"`
	UpdatedAt         string `db:"updated_at"`
}

type positionRow struct {
	AssetID          string               `db:"asset_id"`
	Symbol           string               `db:"symbol"`
	Name             string               `db:"name"`
	Kind             marketdata.AssetKind `db:"kind"`
	QuantityMicro    int64                `db:"quantity_micro"`
	AverageCostCents int64                `db:"average_cost_cents"`
	LastPriceCents   int64                `db:"last_price_cents"`
	UpdatedAt        string               `db:"updated_at"`
}

type transactionRow struct {
	ID            string `db:"id"`
	AssetID       string `db:"asset_id"`
	Symbol        string `db:"symbol"`
	Side          string `db:"side"`
	QuantityMicro int64  `db:"quantity_micro"`
	PriceCents    int64  `db:"price_cents"`
	FeeCents      int64  `db:"fee_cents"`
	Status        string `db:"status"`
	CreatedAt     string `db:"created_at"`
}

func (s *SQLite) UpsertPortfolio(ctx context.Context, portfolio Portfolio) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin portfolio upsert: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if _, err = tx.ExecContext(ctx, `INSERT INTO portfolios (
		id, user_id, client_id, client_portfolio_id, schema_version,
		starting_cash_cents, cash_cents, created_at, updated_at
	) VALUES (?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		user_id = excluded.user_id,
		client_id = excluded.client_id,
		client_portfolio_id = excluded.client_portfolio_id,
		schema_version = excluded.schema_version,
		starting_cash_cents = excluded.starting_cash_cents,
		cash_cents = excluded.cash_cents,
		updated_at = excluded.updated_at`,
		portfolio.ID,
		portfolio.UserID,
		portfolio.ClientID,
		portfolio.ClientPortfolioID,
		portfolio.SchemaVersion,
		portfolio.StartingCashCents,
		portfolio.CashCents,
		formatTime(portfolio.CreatedAt),
		formatTime(portfolio.UpdatedAt),
	); err != nil {
		return fmt.Errorf("upsert portfolio %s: %w", portfolio.ID, err)
	}

	if _, err = tx.ExecContext(ctx, `DELETE FROM portfolio_positions WHERE portfolio_id = ?`, portfolio.ID); err != nil {
		return fmt.Errorf("delete portfolio positions: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM portfolio_transactions WHERE portfolio_id = ?`, portfolio.ID); err != nil {
		return fmt.Errorf("delete portfolio transactions: %w", err)
	}

	for _, position := range portfolio.Positions {
		if _, err = tx.ExecContext(ctx, `INSERT INTO portfolio_positions (
			portfolio_id, asset_id, symbol, name, kind, quantity_micro,
			average_cost_cents, last_price_cents, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			portfolio.ID,
			position.AssetID,
			position.Symbol,
			position.Name,
			string(position.Kind),
			position.QuantityMicro,
			position.AverageCostCents,
			position.LastPriceCents,
			formatTime(position.UpdatedAt),
		); err != nil {
			return fmt.Errorf("insert portfolio position %s: %w", position.AssetID, err)
		}
	}

	for _, transaction := range portfolio.Transactions {
		if _, err = tx.ExecContext(ctx, `INSERT INTO portfolio_transactions (
			id, portfolio_id, asset_id, symbol, side, quantity_micro,
			price_cents, fee_cents, status, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			transaction.ID,
			portfolio.ID,
			transaction.AssetID,
			transaction.Symbol,
			transaction.Side,
			transaction.QuantityMicro,
			transaction.PriceCents,
			transaction.FeeCents,
			transaction.Status,
			formatTime(transaction.CreatedAt),
		); err != nil {
			return fmt.Errorf("insert portfolio transaction %s: %w", transaction.ID, err)
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit portfolio upsert: %w", err)
	}
	return nil
}

func (s *SQLite) Portfolio(ctx context.Context, id string) (Portfolio, error) {
	return s.portfolio(ctx, id)
}

func (s *SQLite) PortfolioByClient(ctx context.Context, clientID, clientPortfolioID string) (Portfolio, error) {
	var id string
	if err := s.db.GetContext(ctx, &id, `SELECT id FROM portfolios
		WHERE client_id = ? AND client_portfolio_id = ?`, clientID, clientPortfolioID); err != nil {
		return Portfolio{}, fmt.Errorf("select portfolio by client: %w", err)
	}
	return s.portfolio(ctx, id)
}

func (s *SQLite) portfolio(ctx context.Context, id string) (Portfolio, error) {
	var row portfolioRow
	if err := s.db.GetContext(ctx, &row, `SELECT
		id, COALESCE(user_id, '') AS user_id, client_id, client_portfolio_id, schema_version,
		starting_cash_cents, cash_cents, created_at, updated_at
		FROM portfolios WHERE id = ?`, id); err != nil {
		return Portfolio{}, fmt.Errorf("select portfolio %s: %w", id, err)
	}

	portfolio, err := row.portfolio()
	if err != nil {
		return Portfolio{}, err
	}

	var positions []positionRow
	if err := s.db.SelectContext(ctx, &positions, `SELECT
		asset_id, symbol, name, kind, quantity_micro, average_cost_cents,
		last_price_cents, updated_at
		FROM portfolio_positions WHERE portfolio_id = ?
		ORDER BY symbol`, id); err != nil {
		return Portfolio{}, fmt.Errorf("select portfolio positions: %w", err)
	}

	var transactions []transactionRow
	if err := s.db.SelectContext(ctx, &transactions, `SELECT
		id, asset_id, symbol, side, quantity_micro, price_cents,
		fee_cents, status, created_at
		FROM portfolio_transactions WHERE portfolio_id = ?
		ORDER BY created_at DESC`, id); err != nil {
		return Portfolio{}, fmt.Errorf("select portfolio transactions: %w", err)
	}

	portfolio.Positions = make([]PortfolioPosition, 0, len(positions))
	for _, row := range positions {
		position, err := row.position()
		if err != nil {
			return Portfolio{}, err
		}
		portfolio.Positions = append(portfolio.Positions, position)
	}

	portfolio.Transactions = make([]PortfolioTransaction, 0, len(transactions))
	for _, row := range transactions {
		transaction, err := row.transaction()
		if err != nil {
			return Portfolio{}, err
		}
		portfolio.Transactions = append(portfolio.Transactions, transaction)
	}

	return portfolio, nil
}

func (r portfolioRow) portfolio() (Portfolio, error) {
	createdAt, err := parseTime(r.CreatedAt)
	if err != nil {
		return Portfolio{}, fmt.Errorf("parse portfolio created_at: %w", err)
	}
	updatedAt, err := parseTime(r.UpdatedAt)
	if err != nil {
		return Portfolio{}, fmt.Errorf("parse portfolio updated_at: %w", err)
	}
	return Portfolio{
		ID:                r.ID,
		UserID:            r.UserID,
		ClientID:          r.ClientID,
		ClientPortfolioID: r.ClientPortfolioID,
		SchemaVersion:     r.SchemaVersion,
		StartingCashCents: r.StartingCashCents,
		CashCents:         r.CashCents,
		CreatedAt:         createdAt,
		UpdatedAt:         updatedAt,
	}, nil
}

func (r positionRow) position() (PortfolioPosition, error) {
	updatedAt, err := parseTime(r.UpdatedAt)
	if err != nil {
		return PortfolioPosition{}, fmt.Errorf("parse position updated_at: %w", err)
	}
	return PortfolioPosition{
		AssetID:          r.AssetID,
		Symbol:           r.Symbol,
		Name:             r.Name,
		Kind:             r.Kind,
		QuantityMicro:    r.QuantityMicro,
		AverageCostCents: r.AverageCostCents,
		LastPriceCents:   r.LastPriceCents,
		UpdatedAt:        updatedAt,
	}, nil
}

func (r transactionRow) transaction() (PortfolioTransaction, error) {
	createdAt, err := parseTime(r.CreatedAt)
	if err != nil {
		return PortfolioTransaction{}, fmt.Errorf("parse transaction created_at: %w", err)
	}
	return PortfolioTransaction{
		ID:            r.ID,
		AssetID:       r.AssetID,
		Symbol:        r.Symbol,
		Side:          r.Side,
		QuantityMicro: r.QuantityMicro,
		PriceCents:    r.PriceCents,
		FeeCents:      r.FeeCents,
		Status:        r.Status,
		CreatedAt:     createdAt,
	}, nil
}
