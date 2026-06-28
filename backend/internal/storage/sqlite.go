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
