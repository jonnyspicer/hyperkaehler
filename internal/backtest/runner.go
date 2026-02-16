package backtest

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"hyperkaehler/internal/config"
	"hyperkaehler/internal/risk"
	"hyperkaehler/internal/strategy"
)

// Runner replays historical market snapshots through strategies to simulate trading.
type Runner struct {
	db         *sql.DB
	strategies []strategy.Strategy
	riskCfg    config.RiskConfig
	startBalance float64
}

func NewRunner(db *sql.DB, strategies []strategy.Strategy, riskCfg config.RiskConfig, startBalance float64) *Runner {
	return &Runner{
		db:           db,
		strategies:   strategies,
		riskCfg:      riskCfg,
		startBalance: startBalance,
	}
}

// Run executes the backtest over the given date range.
func (r *Runner) Run(fromStr, toStr string) error {
	from, to, err := parseDateRange(fromStr, toStr)
	if err != nil {
		return err
	}

	slog.Info("backtest starting", "from", from.Format("2006-01-02"), "to", to.Format("2006-01-02"), "balance", r.startBalance)

	// Load all snapshot timestamps in the range.
	timestamps, err := r.loadSnapshotTimestamps(from, to)
	if err != nil {
		return fmt.Errorf("loading snapshot timestamps: %w", err)
	}
	if len(timestamps) == 0 {
		return fmt.Errorf("no market snapshots found in range %s to %s", fromStr, toStr)
	}

	slog.Info("loaded snapshot timestamps", "count", len(timestamps))

	// Simulate with a virtual portfolio.
	portfolio := &risk.Portfolio{
		Balance:    r.startBalance,
		TotalValue: r.startBalance,
	}
	riskMgr := risk.NewManager(r.riskCfg, portfolio)

	var totalBets int
	var totalWagered float64

	ctx := context.Background()

	for _, ts := range timestamps {
		// Load market data for this snapshot time.
		markets, err := r.loadMarketsAtTimestamp(ts)
		if err != nil {
			slog.Warn("failed to load markets at timestamp", "timestamp", ts, "error", err)
			continue
		}

		riskMgr.Refresh()

		// Evaluate all strategies.
		var allSignals []strategy.Signal
		for _, strat := range r.strategies {
			if !strat.Enabled() {
				continue
			}
			signals, err := strat.Evaluate(ctx, markets)
			if err != nil {
				slog.Warn("strategy error in backtest", "strategy", strat.Name(), "error", err)
				continue
			}
			allSignals = append(allSignals, signals...)
		}

		// Size and simulate execution.
		sized := riskMgr.SizeSignals(allSignals)
		for _, sig := range sized {
			totalBets++
			totalWagered += sig.Amount
			riskMgr.RecordTrade(sig.Signal.MarketID, sig.Amount)

			// Record to backtest_bets table.
			r.recordBacktestBet(sig, ts)
		}
	}

	// Report results.
	slog.Info("=== BACKTEST RESULTS ===",
		"period", fmt.Sprintf("%s to %s", from.Format("2006-01-02"), to.Format("2006-01-02")),
		"snapshots_processed", len(timestamps),
		"total_signals_placed", totalBets,
		"total_mana_wagered", totalWagered,
		"starting_balance", r.startBalance,
	)

	return nil
}

func parseDateRange(fromStr, toStr string) (time.Time, time.Time, error) {
	var from, to time.Time

	if fromStr == "" {
		from = time.Now().AddDate(-1, 0, 0) // Default: 1 year ago.
	} else {
		var err error
		from, err = time.Parse("2006-01-02", fromStr)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("parsing from date: %w", err)
		}
	}

	if toStr == "" {
		to = time.Now()
	} else {
		var err error
		to, err = time.Parse("2006-01-02", toStr)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("parsing to date: %w", err)
		}
	}

	return from, to, nil
}

func (r *Runner) loadSnapshotTimestamps(from, to time.Time) ([]string, error) {
	rows, err := r.db.Query(`
		SELECT DISTINCT snapshot_at FROM market_snapshots
		WHERE snapshot_at >= ? AND snapshot_at <= ?
		ORDER BY snapshot_at`,
		from.Format("2006-01-02 15:04:05"),
		to.Format("2006-01-02 15:04:05"),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var timestamps []string
	for rows.Next() {
		var ts string
		if err := rows.Scan(&ts); err != nil {
			return nil, err
		}
		timestamps = append(timestamps, ts)
	}
	return timestamps, rows.Err()
}

func (r *Runner) loadMarketsAtTimestamp(ts string) ([]strategy.MarketData, error) {
	rows, err := r.db.Query(`
		SELECT m.id, m.question, m.outcome_type, m.mechanism, m.creator_id,
		       m.created_time, m.close_time, m.url, m.is_resolved, m.resolution,
		       s.probability, s.answer_probs, s.volume, s.volume_24h, s.total_liquidity,
		       s.pool_yes, s.pool_no
		FROM market_snapshots s
		JOIN markets m ON m.id = s.market_id
		WHERE s.snapshot_at = ?`,
		ts,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var markets []strategy.MarketData
	for rows.Next() {
		var (
			md           strategy.MarketData
			createdTime  int64
			closeTime    int64
			isResolved   int
			prob         sql.NullFloat64
			answerProbs  sql.NullString
			volume       float64
			volume24h    float64
			liquidity    float64
			poolYes      sql.NullFloat64
			poolNo       sql.NullFloat64
		)

		if err := rows.Scan(
			&md.ID, &md.Question, &md.OutcomeType, &md.Mechanism, &md.CreatorID,
			&createdTime, &closeTime, &md.URL, &isResolved, &md.Resolution,
			&prob, &answerProbs, &volume, &volume24h, &liquidity,
			&poolYes, &poolNo,
		); err != nil {
			return nil, err
		}

		md.CreatedTime = time.UnixMilli(createdTime)
		md.CloseTime = time.UnixMilli(closeTime)
		md.IsResolved = isResolved == 1
		md.Volume = volume
		md.Volume24Hours = volume24h
		md.TotalLiquidity = liquidity

		if prob.Valid {
			md.Probability = prob.Float64
		}

		md.Pool = make(map[string]float64)
		if poolYes.Valid {
			md.Pool["YES"] = poolYes.Float64
		}
		if poolNo.Valid {
			md.Pool["NO"] = poolNo.Float64
		}

		if answerProbs.Valid && answerProbs.String != "" {
			var probs map[string]float64
			if err := json.Unmarshal([]byte(answerProbs.String), &probs); err == nil {
				for id, p := range probs {
					md.Answers = append(md.Answers, strategy.AnswerData{
						ID:          id,
						Probability: p,
					})
				}
			}
		}

		markets = append(markets, md)
	}

	return markets, rows.Err()
}

func (r *Runner) recordBacktestBet(sig risk.SizedSignal, ts string) {
	_, err := r.db.Exec(`
		INSERT INTO bot_bets (market_id, strategy, outcome, amount, expected_prob, market_prob_at_bet, kelly_fraction, placed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		sig.Signal.MarketID, sig.Signal.Strategy, sig.Signal.Outcome,
		sig.Amount, sig.Signal.Confidence, sig.Signal.MarketProb,
		0.0, ts,
	)
	if err != nil {
		slog.Warn("failed to record backtest bet", "error", err)
	}
}
