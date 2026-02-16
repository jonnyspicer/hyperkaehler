package performance

import (
	"database/sql"
	"fmt"
	"math"
)

// Tracker computes performance metrics from the database.
type Tracker struct {
	db *sql.DB
}

func NewTracker(db *sql.DB) *Tracker {
	return &Tracker{db: db}
}

// Report contains all performance metrics.
type Report struct {
	TotalBets        int
	ResolvedBets     int
	TotalManaWagered float64
	TotalPnL         float64
	ROI              float64
	WinRate          float64
	CurrentBalance   float64
	PeakBalance      float64
	MaxDrawdown      float64
	StrategyStats    map[string]StrategyStats
}

// StrategyStats contains per-strategy performance.
type StrategyStats struct {
	BetCount    int
	ManaWagered float64
	PnL         float64
	ROI         float64
	WinRate     float64
	AvgEdge     float64
}

// Generate computes the full performance report.
func (t *Tracker) Generate() (*Report, error) {
	r := &Report{
		StrategyStats: make(map[string]StrategyStats),
	}

	if err := t.computeOverall(r); err != nil {
		return nil, fmt.Errorf("computing overall stats: %w", err)
	}
	if err := t.computeStrategyStats(r); err != nil {
		return nil, fmt.Errorf("computing strategy stats: %w", err)
	}
	if err := t.computeDrawdown(r); err != nil {
		return nil, fmt.Errorf("computing drawdown: %w", err)
	}

	return r, nil
}

func (t *Tracker) computeOverall(r *Report) error {
	row := t.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(amount), 0) FROM bot_bets`)
	if err := row.Scan(&r.TotalBets, &r.TotalManaWagered); err != nil {
		return err
	}

	row = t.db.QueryRow(`
		SELECT COUNT(*), COALESCE(SUM(pnl), 0)
		FROM bot_bets WHERE resolved = 1`)
	var resolvedCount int
	var totalPnL float64
	if err := row.Scan(&resolvedCount, &totalPnL); err != nil {
		return err
	}
	r.ResolvedBets = resolvedCount
	r.TotalPnL = totalPnL

	if r.TotalManaWagered > 0 {
		r.ROI = r.TotalPnL / r.TotalManaWagered
	}

	// Win rate.
	if resolvedCount > 0 {
		row = t.db.QueryRow(`SELECT COUNT(*) FROM bot_bets WHERE resolved = 1 AND pnl > 0`)
		var wins int
		if err := row.Scan(&wins); err != nil {
			return err
		}
		r.WinRate = float64(wins) / float64(resolvedCount)
	}

	return nil
}

func (t *Tracker) computeStrategyStats(r *Report) error {
	rows, err := t.db.Query(`
		SELECT strategy, COUNT(*), COALESCE(SUM(amount), 0),
		       COALESCE(SUM(CASE WHEN resolved = 1 THEN pnl ELSE 0 END), 0),
		       COALESCE(AVG(CASE WHEN resolved = 1 THEN market_prob_at_bet - expected_prob END), 0),
		       COALESCE(SUM(CASE WHEN resolved = 1 AND pnl > 0 THEN 1 ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN resolved = 1 THEN 1 ELSE 0 END), 0)
		FROM bot_bets GROUP BY strategy`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		var stats StrategyStats
		var wins, resolved int
		if err := rows.Scan(&name, &stats.BetCount, &stats.ManaWagered, &stats.PnL, &stats.AvgEdge, &wins, &resolved); err != nil {
			return err
		}
		if stats.ManaWagered > 0 {
			stats.ROI = stats.PnL / stats.ManaWagered
		}
		if resolved > 0 {
			stats.WinRate = float64(wins) / float64(resolved)
		}
		r.StrategyStats[name] = stats
	}
	return rows.Err()
}

func (t *Tracker) computeDrawdown(r *Report) error {
	rows, err := t.db.Query(`SELECT total_value FROM bankroll_snapshots ORDER BY snapshot_at ASC`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var peak float64
	var maxDD float64
	for rows.Next() {
		var value float64
		if err := rows.Scan(&value); err != nil {
			return err
		}
		if value > peak {
			peak = value
		}
		if peak > 0 {
			dd := (peak - value) / peak
			maxDD = math.Max(maxDD, dd)
		}
	}
	r.PeakBalance = peak
	r.MaxDrawdown = maxDD
	return rows.Err()
}
