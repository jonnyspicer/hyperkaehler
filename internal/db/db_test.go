package db

import (
	"testing"
)

func TestMigrate_CreatesAllTables(t *testing.T) {
	database, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	if err := Migrate(database); err != nil {
		t.Fatal(err)
	}

	tables := []string{
		"schema_version",
		"markets",
		"market_snapshots",
		"bot_bets",
		"bankroll_snapshots",
		"active_orders",
	}

	for _, table := range tables {
		row := database.QueryRow(
			`SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?`, table)
		var count int
		if err := row.Scan(&count); err != nil {
			t.Fatalf("checking table %s: %v", table, err)
		}
		if count != 1 {
			t.Errorf("table %s not found", table)
		}
	}
}

func TestMigrate_Idempotent(t *testing.T) {
	database, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	// Run twice — should not error.
	if err := Migrate(database); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(database); err != nil {
		t.Fatal(err)
	}
}

func TestMigrate_InsertAndQuery(t *testing.T) {
	database, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	if err := Migrate(database); err != nil {
		t.Fatal(err)
	}

	// Insert a market.
	_, err = database.Exec(`
		INSERT INTO markets (id, question, outcome_type, mechanism, creator_id, created_time, close_time, url)
		VALUES ('m1', 'Test?', 'BINARY', 'cpmm-1', 'user1', 1700000000000, 1800000000000, 'https://example.com')`)
	if err != nil {
		t.Fatal(err)
	}

	// Insert a snapshot.
	_, err = database.Exec(`
		INSERT INTO market_snapshots (market_id, probability, volume, volume_24h, total_liquidity)
		VALUES ('m1', 0.65, 500, 50, 200)`)
	if err != nil {
		t.Fatal(err)
	}

	// Insert a bet.
	_, err = database.Exec(`
		INSERT INTO bot_bets (market_id, strategy, outcome, amount, expected_prob, market_prob_at_bet, kelly_fraction)
		VALUES ('m1', 'arbitrage', 'YES', 10, 0.70, 0.60, 0.25)`)
	if err != nil {
		t.Fatal(err)
	}

	// Verify.
	var count int
	row := database.QueryRow(`SELECT COUNT(*) FROM bot_bets`)
	if err := row.Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("expected 1 bet, got %d", count)
	}
}
