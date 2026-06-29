package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// CountUsers reports how many admin/user profiles exist (used for one-time seeding).
func (s *SQLite) CountUsers(ctx context.Context) (int, error) {
	var count int
	if err := s.db.GetContext(ctx, &count, `SELECT COUNT(*) FROM user_profiles`); err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}
	return count, nil
}

// CreateUser inserts a user profile with a pre-hashed password.
func (s *SQLite) CreateUser(ctx context.Context, id, username, passwordHash string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO user_profiles (id, username, password_hash, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`, id, username, passwordHash, now, now)
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

// GetUserPasswordHash returns the stored password hash for a username.
func (s *SQLite) GetUserPasswordHash(ctx context.Context, username string) (string, bool, error) {
	var hash string
	err := s.db.GetContext(ctx, &hash, `SELECT password_hash FROM user_profiles WHERE username = ?`, username)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("get user: %w", err)
	}
	return hash, true, nil
}

type TeamMapping struct {
	OriginalCode   string `db:"original_code" json:"originalCode"`
	PolymarketCode string `db:"polymarket_code" json:"polymarketCode"`
	UpdatedAt      string `db:"updated_at" json:"updatedAt"`
}

func (s *SQLite) ListTeamMappings(ctx context.Context) ([]TeamMapping, error) {
	mappings := []TeamMapping{}
	if err := s.db.SelectContext(ctx, &mappings, `
		SELECT original_code, polymarket_code, updated_at
		FROM team_mappings ORDER BY original_code
	`); err != nil {
		return nil, fmt.Errorf("list team mappings: %w", err)
	}
	return mappings, nil
}

// TeamMappingsMap returns original→polymarket codes (both lowercased) for slug building.
func (s *SQLite) TeamMappingsMap(ctx context.Context) (map[string]string, error) {
	mappings, err := s.ListTeamMappings(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(mappings))
	for _, m := range mappings {
		out[strings.ToLower(m.OriginalCode)] = strings.ToLower(m.PolymarketCode)
	}
	return out, nil
}

func (s *SQLite) UpsertTeamMapping(ctx context.Context, originalCode, polymarketCode string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO team_mappings (original_code, polymarket_code, updated_at)
		VALUES (?, ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		ON CONFLICT(original_code) DO UPDATE SET
			polymarket_code = excluded.polymarket_code,
			updated_at = excluded.updated_at
	`, strings.ToUpper(strings.TrimSpace(originalCode)), strings.ToUpper(strings.TrimSpace(polymarketCode)))
	if err != nil {
		return fmt.Errorf("upsert team mapping: %w", err)
	}
	return nil
}

func (s *SQLite) DeleteTeamMapping(ctx context.Context, originalCode string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM team_mappings WHERE original_code = ?`,
		strings.ToUpper(strings.TrimSpace(originalCode)))
	if err != nil {
		return fmt.Errorf("delete team mapping: %w", err)
	}
	return nil
}
