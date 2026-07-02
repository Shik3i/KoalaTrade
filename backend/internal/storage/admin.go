package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	RoleUser  = "user"
	RoleAdmin = "admin"
)

type UserProfile struct {
	ID           string `db:"id" json:"id"`
	Username     string `db:"username" json:"username"`
	DisplayName  string `db:"display_name" json:"displayName"`
	Role         string `db:"role" json:"role"`
	PasswordHash string `db:"password_hash" json:"-"`
	Disabled     bool   `db:"disabled" json:"disabled"`
	CreatedAt    string `db:"created_at" json:"createdAt"`
	UpdatedAt    string `db:"updated_at" json:"updatedAt"`
}

// CountUsers reports how many admin/user profiles exist (used for one-time seeding).
func (s *SQLite) CountUsers(ctx context.Context) (int, error) {
	var count int
	if err := s.db.GetContext(ctx, &count, `SELECT COUNT(*) FROM user_profiles`); err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}
	return count, nil
}

// CreateUser inserts a user profile with a pre-hashed password.
func (s *SQLite) CreateUser(ctx context.Context, id, username, passwordHash, role string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	username = normalizeUsername(username)
	if role == "" {
		role = RoleUser
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO user_profiles (id, username, display_name, password_hash, role, disabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 0, ?, ?)
	`, id, username, username, passwordHash, role, now, now)
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

func (s *SQLite) UserByUsername(ctx context.Context, username string) (UserProfile, bool, error) {
	var user UserProfile
	err := s.db.GetContext(ctx, &user, `SELECT
		id, username, COALESCE(display_name, username) AS display_name, role,
		COALESCE(password_hash, '') AS password_hash, disabled, created_at, updated_at
		FROM user_profiles WHERE username = ?`, normalizeUsername(username))
	if err == sql.ErrNoRows {
		return UserProfile{}, false, nil
	}
	if err != nil {
		return UserProfile{}, false, fmt.Errorf("get user by username: %w", err)
	}
	return user, true, nil
}

func (s *SQLite) UserByID(ctx context.Context, id string) (UserProfile, bool, error) {
	var user UserProfile
	err := s.db.GetContext(ctx, &user, `SELECT
		id, username, COALESCE(display_name, username) AS display_name, role,
		COALESCE(password_hash, '') AS password_hash, disabled, created_at, updated_at
		FROM user_profiles WHERE id = ?`, id)
	if err == sql.ErrNoRows {
		return UserProfile{}, false, nil
	}
	if err != nil {
		return UserProfile{}, false, fmt.Errorf("get user by id: %w", err)
	}
	return user, true, nil
}

func (s *SQLite) UpdateUserRole(ctx context.Context, id, role string) error {
	if role != RoleUser && role != RoleAdmin {
		return errors.New("invalid role")
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE user_profiles
		SET role = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE id = ?
	`, role, id)
	if err != nil {
		return fmt.Errorf("update user role: %w", err)
	}
	return nil
}

func (s *SQLite) UpdateUserDisplayName(ctx context.Context, id, displayName string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE user_profiles
		SET display_name = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE id = ?
	`, strings.TrimSpace(displayName), id)
	if err != nil {
		return fmt.Errorf("update user display name: %w", err)
	}
	return nil
}

func (s *SQLite) UpdateUserPasswordHash(ctx context.Context, id, passwordHash string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE user_profiles
		SET password_hash = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE id = ?
	`, passwordHash, id)
	if err != nil {
		return fmt.Errorf("update user password: %w", err)
	}
	return nil
}

func (s *SQLite) DeleteUser(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM user_profiles WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	return nil
}

func normalizeUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
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
