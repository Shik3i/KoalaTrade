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

// EsportsTeam is the persisted, server-owned representation of a LoL team.
// Logo bytes are deliberately kept in SQLite so the browser never has to
// fetch an image from lolesports.com.
type EsportsTeam struct {
	Code            string `db:"code"`
	Name            string `db:"name"`
	League          string `db:"league"`
	Logo            []byte `db:"logo"`
	LogoContentType string `db:"logo_content_type"`
	LogoSourceURL   string `db:"logo_source_url"`
	SyncedAt        string `db:"synced_at"`
	UpdatedAt       string `db:"updated_at"`
}

type EsportsMatchDetail struct {
	MatchID    string `db:"match_id"`
	State      string `db:"state"`
	Team1Code  string `db:"team1_code"`
	Team2Code  string `db:"team2_code"`
	Team1Score int    `db:"team1_score"`
	Team2Score int    `db:"team2_score"`
	FetchedAt  string `db:"fetched_at"`
}

type EsportsMatchGame struct {
	MatchID    string `db:"match_id"`
	GameID     string `db:"game_id"`
	GameNumber int    `db:"game_number"`
	State      string `db:"state"`
}

type EsportsMatchVideo struct {
	MatchID  string `db:"match_id"`
	GameID   string `db:"game_id"`
	Kind     string `db:"kind"`
	URL      string `db:"url"`
	Provider string `db:"provider"`
	Locale   string `db:"locale"`
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
		`CREATE TABLE IF NOT EXISTS esports_teams (
			code TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			league TEXT NOT NULL,
			logo BLOB,
			logo_content_type TEXT NOT NULL DEFAULT '',
			logo_source_url TEXT NOT NULL DEFAULT '',
			synced_at TEXT NOT NULL,
			updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		);`,
		`CREATE INDEX IF NOT EXISTS idx_esports_teams_synced_at
			ON esports_teams(synced_at);`,
		`CREATE TABLE IF NOT EXISTS esports_match_details (
			match_id TEXT PRIMARY KEY,
			state TEXT NOT NULL,
			team1_code TEXT NOT NULL,
			team2_code TEXT NOT NULL,
			team1_score INTEGER NOT NULL DEFAULT 0 CHECK (team1_score >= 0),
			team2_score INTEGER NOT NULL DEFAULT 0 CHECK (team2_score >= 0),
			fetched_at TEXT NOT NULL,
			updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		);`,
		`CREATE TABLE IF NOT EXISTS esports_match_games (
			match_id TEXT NOT NULL REFERENCES esports_match_details(match_id) ON DELETE CASCADE,
			game_id TEXT NOT NULL,
			game_number INTEGER NOT NULL CHECK (game_number > 0),
			state TEXT NOT NULL,
			PRIMARY KEY (match_id, game_id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_esports_match_games_match
			ON esports_match_games(match_id, game_number);`,
		`CREATE TABLE IF NOT EXISTS esports_match_videos (
			match_id TEXT NOT NULL REFERENCES esports_match_details(match_id) ON DELETE CASCADE,
			game_id TEXT NOT NULL DEFAULT '',
			kind TEXT NOT NULL CHECK (kind IN ('vod', 'stream')),
			url TEXT NOT NULL,
			provider TEXT NOT NULL DEFAULT '',
			locale TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (match_id, game_id, kind, url)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_esports_match_videos_match
			ON esports_match_videos(match_id, kind);`,
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

// LoadEsportsTeams returns the last complete team snapshot. The sync timestamp
// is stored per row so a restart can decide whether the weekly snapshot is
// still fresh without another metadata blob.
func (s *SQLite) LoadEsportsTeams(ctx context.Context) ([]EsportsTeam, error) {
	var teams []EsportsTeam
	if err := s.db.SelectContext(ctx, &teams, `
		SELECT code, name, league, logo, logo_content_type, logo_source_url, synced_at, updated_at
		FROM esports_teams
		ORDER BY name COLLATE NOCASE, code
	`); err != nil {
		return nil, fmt.Errorf("load esports teams: %w", err)
	}
	return teams, nil
}

// UpsertEsportsTeams stores one complete upstream snapshot. A missing logo in
// a transient upstream response does not erase the last known good logo.
func (s *SQLite) UpsertEsportsTeams(ctx context.Context, teams []EsportsTeam) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin esports teams upsert: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	const query = `
		INSERT INTO esports_teams
			(code, name, league, logo, logo_content_type, logo_source_url, synced_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		ON CONFLICT(code) DO UPDATE SET
			name = excluded.name,
			league = excluded.league,
			logo = CASE WHEN excluded.logo IS NOT NULL AND length(excluded.logo) > 0
				THEN excluded.logo ELSE esports_teams.logo END,
			logo_content_type = CASE WHEN excluded.logo IS NOT NULL AND length(excluded.logo) > 0
				THEN excluded.logo_content_type ELSE esports_teams.logo_content_type END,
			logo_source_url = CASE WHEN excluded.logo_source_url <> ''
				THEN excluded.logo_source_url ELSE esports_teams.logo_source_url END,
			synced_at = CASE WHEN excluded.synced_at <> ''
				THEN excluded.synced_at ELSE esports_teams.synced_at END,
			updated_at = excluded.updated_at
	`
	for _, team := range teams {
		if _, err := tx.ExecContext(ctx, query, team.Code, team.Name, team.League,
			team.Logo, team.LogoContentType, team.LogoSourceURL, team.SyncedAt); err != nil {
			return fmt.Errorf("upsert esports team %s: %w", team.Code, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit esports teams upsert: %w", err)
	}
	return nil
}

// EsportsTeamLogo returns a single stored logo for the same-origin image route.
func (s *SQLite) EsportsTeamLogo(ctx context.Context, code string) ([]byte, string, bool, error) {
	var team EsportsTeam
	err := s.db.GetContext(ctx, &team, `
		SELECT logo, logo_content_type
		FROM esports_teams
		WHERE code = ?
	`, strings.ToUpper(strings.TrimSpace(code)))
	if err == sql.ErrNoRows {
		return nil, "", false, nil
	}
	if err != nil {
		return nil, "", false, fmt.Errorf("load esports team logo: %w", err)
	}
	if len(team.Logo) == 0 || team.LogoContentType == "" {
		return nil, "", false, nil
	}
	return team.Logo, team.LogoContentType, true, nil
}

func (s *SQLite) LoadEsportsMatchDetails(ctx context.Context, matchID string) (EsportsMatchDetail, []EsportsMatchGame, []EsportsMatchVideo, bool, error) {
	var detail EsportsMatchDetail
	if err := s.db.GetContext(ctx, &detail, `
		SELECT match_id, state, team1_code, team2_code, team1_score, team2_score, fetched_at
		FROM esports_match_details WHERE match_id = ?
	`, matchID); err != nil {
		if err == sql.ErrNoRows {
			return EsportsMatchDetail{}, nil, nil, false, nil
		}
		return EsportsMatchDetail{}, nil, nil, false, fmt.Errorf("load esports match details: %w", err)
	}

	var games []EsportsMatchGame
	if err := s.db.SelectContext(ctx, &games, `
		SELECT match_id, game_id, game_number, state
		FROM esports_match_games WHERE match_id = ? ORDER BY game_number, game_id
	`, matchID); err != nil {
		return EsportsMatchDetail{}, nil, nil, false, fmt.Errorf("load esports match games: %w", err)
	}
	var videos []EsportsMatchVideo
	if err := s.db.SelectContext(ctx, &videos, `
		SELECT match_id, game_id, kind, url, provider, locale
		FROM esports_match_videos WHERE match_id = ? ORDER BY kind, game_id, url
	`, matchID); err != nil {
		return EsportsMatchDetail{}, nil, nil, false, fmt.Errorf("load esports match videos: %w", err)
	}
	return detail, games, videos, true, nil
}

func (s *SQLite) UpsertEsportsMatchDetails(ctx context.Context, detail EsportsMatchDetail, games []EsportsMatchGame, videos []EsportsMatchVideo) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin esports match details upsert: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO esports_match_details
			(match_id, state, team1_code, team2_code, team1_score, team2_score, fetched_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		ON CONFLICT(match_id) DO UPDATE SET
			state = excluded.state,
			team1_code = excluded.team1_code,
			team2_code = excluded.team2_code,
			team1_score = excluded.team1_score,
			team2_score = excluded.team2_score,
			fetched_at = excluded.fetched_at,
			updated_at = excluded.updated_at
	`, detail.MatchID, detail.State, detail.Team1Code, detail.Team2Code,
		detail.Team1Score, detail.Team2Score, detail.FetchedAt); err != nil {
		return fmt.Errorf("upsert esports match detail: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM esports_match_games WHERE match_id = ?`, detail.MatchID); err != nil {
		return fmt.Errorf("replace esports match games: %w", err)
	}
	for _, game := range games {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO esports_match_games (match_id, game_id, game_number, state)
			VALUES (?, ?, ?, ?)
		`, detail.MatchID, game.GameID, game.GameNumber, game.State); err != nil {
			return fmt.Errorf("insert esports match game: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM esports_match_videos WHERE match_id = ?`, detail.MatchID); err != nil {
		return fmt.Errorf("replace esports match videos: %w", err)
	}
	for _, video := range videos {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO esports_match_videos (match_id, game_id, kind, url, provider, locale)
			VALUES (?, ?, ?, ?, ?, ?)
		`, detail.MatchID, video.GameID, video.Kind, video.URL, video.Provider, video.Locale); err != nil {
			return fmt.Errorf("insert esports match video: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit esports match details upsert: %w", err)
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
