package storage

import (
	"context"
	"testing"
)

func TestOpenSQLiteCreatesFoundationTables(t *testing.T) {
	store, err := OpenSQLite(t.TempDir() + "/koalatrade.db")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	ctx := context.Background()
	for _, table := range []string{
		"app_meta",
		"user_profiles",
		"portfolio_snapshots",
		"leaderboard_snapshots",
	} {
		exists, err := store.TableExists(ctx, table)
		if err != nil {
			t.Fatalf("check table %s: %v", table, err)
		}
		if !exists {
			t.Fatalf("expected table %s to exist", table)
		}
	}
}
