package storage

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

type SQLite struct {
	db *sqlx.DB
}

func OpenSQLite(path string) (*SQLite, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	db, err := sqlx.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(time.Hour)

	store := &SQLite{db: db}
	if err := store.configure(); err != nil {
		_ = db.Close()
		return nil, err
	}

	return store, nil
}

func (s *SQLite) Close() error {
	return s.db.Close()
}

func (s *SQLite) PingContext(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// GetMeta reads a value from the generic app_meta key/value store.
func (s *SQLite) GetMeta(ctx context.Context, key string) (string, bool, error) {
	var value string
	err := s.db.GetContext(ctx, &value, `SELECT value FROM app_meta WHERE key = ?`, key)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("get meta: %w", err)
	}
	return value, true, nil
}

// SetMeta upserts a value into the generic app_meta key/value store.
func (s *SQLite) SetMeta(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO app_meta (key, value, updated_at)
		VALUES (?, ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
	`, key, value)
	if err != nil {
		return fmt.Errorf("set meta: %w", err)
	}
	return nil
}

func (s *SQLite) TableExists(ctx context.Context, name string) (bool, error) {
	var exists bool
	err := s.db.GetContext(ctx, &exists, `SELECT EXISTS (
		SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?
	)`, name)
	if err != nil {
		return false, fmt.Errorf("check table exists: %w", err)
	}
	return exists, nil
}

func (s *SQLite) configure() error {
	statements := []string{
		"PRAGMA journal_mode = WAL;",
		"PRAGMA foreign_keys = ON;",
		"PRAGMA busy_timeout = 5000;",
		`CREATE TABLE IF NOT EXISTS app_meta (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		);`,
		`CREATE TABLE IF NOT EXISTS user_profiles (
			id TEXT PRIMARY KEY,
			username TEXT UNIQUE,
			display_name TEXT,
			password_hash TEXT,
			role TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('user', 'admin')),
			disabled INTEGER NOT NULL DEFAULT 0 CHECK (disabled IN (0, 1)),
			created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
			updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		);`,
		`ALTER TABLE user_profiles ADD COLUMN display_name TEXT;`,
		`ALTER TABLE user_profiles ADD COLUMN role TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('user', 'admin'));`,
		`ALTER TABLE user_profiles ADD COLUMN disabled INTEGER NOT NULL DEFAULT 0 CHECK (disabled IN (0, 1));`,
		`UPDATE user_profiles SET display_name = username WHERE display_name IS NULL OR display_name = '';`,
		`CREATE TABLE IF NOT EXISTS assets (
			id TEXT PRIMARY KEY,
			kind TEXT NOT NULL CHECK (kind IN ('stock', 'etf', 'crypto', 'commodity', 'event')),
			symbol TEXT NOT NULL,
			name TEXT NOT NULL,
			currency TEXT NOT NULL DEFAULT 'USD',
			provider TEXT NOT NULL,
			provider_ref TEXT NOT NULL DEFAULT '',
			active INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
			created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
			updated_at TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_assets_kind_symbol
			ON assets(kind, symbol);`,
		`CREATE TABLE IF NOT EXISTS asset_quotes (
			asset_id TEXT PRIMARY KEY REFERENCES assets(id) ON DELETE CASCADE,
			symbol TEXT NOT NULL,
			price_cents INTEGER NOT NULL CHECK (price_cents >= 0),
			change_bps INTEGER NOT NULL,
			source TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			cached_until TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS asset_history (
			asset_id TEXT NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
			timeframe TEXT NOT NULL,
			timestamp TEXT NOT NULL,
			price_cents INTEGER NOT NULL CHECK (price_cents >= 0),
			PRIMARY KEY (asset_id, timeframe, timestamp)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_asset_history_timeframe_timestamp
			ON asset_history(timeframe, timestamp);`,
		`CREATE INDEX IF NOT EXISTS idx_asset_quotes_cached_until
			ON asset_quotes(cached_until);`,
		`CREATE TABLE IF NOT EXISTS portfolios (
			id TEXT PRIMARY KEY,
			user_id TEXT REFERENCES user_profiles(id) ON DELETE CASCADE,
			client_id TEXT NOT NULL,
			client_portfolio_id TEXT NOT NULL,
			schema_version INTEGER NOT NULL DEFAULT 1,
			starting_cash_cents INTEGER NOT NULL CHECK (starting_cash_cents >= 0),
			cash_cents INTEGER NOT NULL CHECK (cash_cents >= 0),
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE(client_id, client_portfolio_id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_portfolios_user_updated
			ON portfolios(user_id, updated_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_portfolios_client_updated
			ON portfolios(client_id, updated_at DESC);`,
		`CREATE TABLE IF NOT EXISTS portfolio_positions (
			portfolio_id TEXT NOT NULL REFERENCES portfolios(id) ON DELETE CASCADE,
			asset_id TEXT NOT NULL REFERENCES assets(id) ON DELETE RESTRICT,
			symbol TEXT NOT NULL,
			name TEXT NOT NULL,
			kind TEXT NOT NULL CHECK (kind IN ('stock', 'etf', 'crypto', 'commodity', 'event')),
			quantity_micro INTEGER NOT NULL CHECK (quantity_micro > 0),
			average_cost_cents INTEGER NOT NULL CHECK (average_cost_cents >= 0),
			last_price_cents INTEGER NOT NULL CHECK (last_price_cents >= 0),
			updated_at TEXT NOT NULL,
			PRIMARY KEY (portfolio_id, asset_id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_portfolio_positions_asset
			ON portfolio_positions(asset_id);`,
		`CREATE TABLE IF NOT EXISTS portfolio_transactions (
			id TEXT PRIMARY KEY,
			portfolio_id TEXT NOT NULL REFERENCES portfolios(id) ON DELETE CASCADE,
			asset_id TEXT NOT NULL REFERENCES assets(id) ON DELETE RESTRICT,
			symbol TEXT NOT NULL,
			side TEXT NOT NULL CHECK (side IN ('buy', 'sell')),
			quantity_micro INTEGER NOT NULL CHECK (quantity_micro > 0),
			price_cents INTEGER NOT NULL CHECK (price_cents >= 0),
			fee_cents INTEGER NOT NULL CHECK (fee_cents >= 0),
			status TEXT NOT NULL CHECK (status IN ('local', 'synced')),
			created_at TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_portfolio_transactions_portfolio_created
			ON portfolio_transactions(portfolio_id, created_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_portfolio_transactions_asset_created
			ON portfolio_transactions(asset_id, created_at DESC);`,
		`CREATE TABLE IF NOT EXISTS portfolio_snapshots (
			id TEXT PRIMARY KEY,
			user_id TEXT REFERENCES user_profiles(id) ON DELETE CASCADE,
			client_id TEXT NOT NULL,
			starting_cash_cents INTEGER NOT NULL,
			cash_cents INTEGER NOT NULL,
			positions_json TEXT NOT NULL,
			transactions_json TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_portfolio_snapshots_user_updated
			ON portfolio_snapshots(user_id, updated_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_portfolio_snapshots_client_updated
			ON portfolio_snapshots(client_id, updated_at DESC);`,
		`CREATE TABLE IF NOT EXISTS leaderboard_snapshots (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL REFERENCES user_profiles(id) ON DELETE CASCADE,
			display_name TEXT NOT NULL,
			total_equity_cents INTEGER NOT NULL,
			total_return_bps INTEGER NOT NULL,
			period TEXT NOT NULL,
			recorded_at TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_leaderboard_snapshots_period_rank
			ON leaderboard_snapshots(period, total_return_bps DESC, recorded_at DESC);`,
		`CREATE TABLE IF NOT EXISTS team_mappings (
			original_code TEXT PRIMARY KEY,
			polymarket_code TEXT NOT NULL,
			updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		);`,
		`CREATE TABLE IF NOT EXISTS open_orders (
			id TEXT PRIMARY KEY,
			portfolio_id TEXT NOT NULL REFERENCES portfolios(id) ON DELETE CASCADE,
			asset_id TEXT NOT NULL REFERENCES assets(id) ON DELETE RESTRICT,
			symbol TEXT NOT NULL,
			name TEXT NOT NULL,
			kind TEXT NOT NULL CHECK (kind IN ('stock', 'etf', 'crypto', 'commodity', 'event')),
			side TEXT NOT NULL CHECK (side IN ('buy', 'sell')),
			order_type TEXT NOT NULL CHECK (order_type IN ('limit', 'stop')),
			quantity_micro INTEGER NOT NULL CHECK (quantity_micro > 0),
			trigger_price_cents INTEGER NOT NULL CHECK (trigger_price_cents > 0),
			created_at TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_open_orders_portfolio
			ON open_orders(portfolio_id, created_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_open_orders_asset
			ON open_orders(asset_id);`,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := s.db.Connx(ctx)
	if err != nil {
		return fmt.Errorf("open sqlite connection: %w", err)
	}
	defer conn.Close()

	// One-time migration: the asset_history schema gained a `timeframe` column as
	// part of the primary key. The old shape can't be ALTERed into the new one in
	// SQLite, so drop it — but ONLY when it still has the old schema, so this does
	// not wipe accumulated history on every restart.
	if err := migrateLegacyAssetHistory(ctx, conn); err != nil {
		return fmt.Errorf("migrate asset_history: %w", err)
	}

	// One-time migration: relax the portfolio_transactions price constraint from
	// `> 0` to `>= 0` so settled losing eSports bets (which pay out 0¢) can be
	// recorded and synced. The indexes are recreated by the statements loop below.
	if err := migrateTransactionPriceConstraint(ctx, conn); err != nil {
		return fmt.Errorf("migrate portfolio_transactions: %w", err)
	}

	for _, statement := range statements {
		if _, err := conn.ExecContext(ctx, statement); err != nil {
			if err == sql.ErrNoRows || strings.Contains(err.Error(), "duplicate column name") {
				continue
			}
			return fmt.Errorf("configure sqlite: %w", err)
		}
	}

	return nil
}

// migrateLegacyAssetHistory drops asset_history only if it exists with the old
// (pre-timeframe) schema, so the new CREATE TABLE IF NOT EXISTS can recreate it.
// Once migrated, the table already has the `timeframe` column and is left alone —
// making this safe to run on every startup without losing history.
func migrateLegacyAssetHistory(ctx context.Context, conn *sqlx.Conn) error {
	rows, err := conn.QueryContext(ctx, `PRAGMA table_info(asset_history)`)
	if err != nil {
		return err
	}
	defer rows.Close()

	hasColumns := false
	hasTimeframe := false
	for rows.Next() {
		var (
			cid       int
			name      string
			ctype     string
			notNull   int
			dfltValue sql.NullString
			pk        int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dfltValue, &pk); err != nil {
			return err
		}
		hasColumns = true
		if name == "timeframe" {
			hasTimeframe = true
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	// Table exists (has columns) but predates the timeframe column → drop it once.
	if hasColumns && !hasTimeframe {
		if _, err := conn.ExecContext(ctx, `DROP TABLE asset_history`); err != nil {
			return err
		}
	}
	return nil
}

// migrateTransactionPriceConstraint rebuilds portfolio_transactions if it still
// carries the legacy `price_cents > 0` CHECK, replacing it with `price_cents >= 0`.
// It is a no-op once migrated (or on a fresh database).
func migrateTransactionPriceConstraint(ctx context.Context, conn *sqlx.Conn) error {
	var ddl sql.NullString
	err := conn.QueryRowContext(ctx,
		`SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'portfolio_transactions'`,
	).Scan(&ddl)
	if err == sql.ErrNoRows {
		return nil // fresh DB: the statements loop creates the up-to-date schema.
	}
	if err != nil {
		return err
	}
	if !ddl.Valid || !strings.Contains(ddl.String, "price_cents > 0") {
		return nil // already migrated.
	}

	steps := []string{
		`ALTER TABLE portfolio_transactions RENAME TO portfolio_transactions_legacy;`,
		`CREATE TABLE portfolio_transactions (
			id TEXT PRIMARY KEY,
			portfolio_id TEXT NOT NULL REFERENCES portfolios(id) ON DELETE CASCADE,
			asset_id TEXT NOT NULL REFERENCES assets(id) ON DELETE RESTRICT,
			symbol TEXT NOT NULL,
			side TEXT NOT NULL CHECK (side IN ('buy', 'sell')),
			quantity_micro INTEGER NOT NULL CHECK (quantity_micro > 0),
			price_cents INTEGER NOT NULL CHECK (price_cents >= 0),
			fee_cents INTEGER NOT NULL CHECK (fee_cents >= 0),
			status TEXT NOT NULL CHECK (status IN ('local', 'synced')),
			created_at TEXT NOT NULL
		);`,
		`INSERT INTO portfolio_transactions SELECT * FROM portfolio_transactions_legacy;`,
		`DROP TABLE portfolio_transactions_legacy;`,
	}
	for _, step := range steps {
		if _, err := conn.ExecContext(ctx, step); err != nil {
			return err
		}
	}
	return nil
}
