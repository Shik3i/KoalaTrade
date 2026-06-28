package storage

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
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
			password_hash TEXT,
			created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
			updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		);`,
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
		`CREATE INDEX IF NOT EXISTS idx_asset_quotes_cached_until
			ON asset_quotes(cached_until);`,
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
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := s.db.Connx(ctx)
	if err != nil {
		return fmt.Errorf("open sqlite connection: %w", err)
	}
	defer conn.Close()

	for _, statement := range statements {
		if _, err := conn.ExecContext(ctx, statement); err != nil {
			if err == sql.ErrNoRows {
				continue
			}
			return fmt.Errorf("configure sqlite: %w", err)
		}
	}

	return nil
}
