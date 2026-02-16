package scheduler

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"hyperkaehler/internal/collector"
	"hyperkaehler/internal/config"
	"hyperkaehler/internal/execution"
	"hyperkaehler/internal/market"
	"hyperkaehler/internal/performance"
	"hyperkaehler/internal/risk"
	"hyperkaehler/internal/strategy"
)

// Scheduler orchestrates the main trading loop.
type Scheduler struct {
	scanner     *market.Scanner
	cache       *market.Cache
	strategies  []strategy.Strategy
	riskMgr     *risk.Manager
	executor    *execution.Executor
	collector   *collector.Collector
	tracker     *performance.Tracker
	portfolio   *risk.Portfolio
	db          *sql.DB
	cfg         config.ScheduleConfig
}

// New creates a new Scheduler with all dependencies.
func New(
	scanner *market.Scanner,
	cache *market.Cache,
	strategies []strategy.Strategy,
	riskMgr *risk.Manager,
	executor *execution.Executor,
	coll *collector.Collector,
	tracker *performance.Tracker,
	portfolio *risk.Portfolio,
	db *sql.DB,
	cfg config.ScheduleConfig,
) *Scheduler {
	return &Scheduler{
		scanner:    scanner,
		cache:      cache,
		strategies: strategies,
		riskMgr:    riskMgr,
		executor:   executor,
		collector:  coll,
		tracker:    tracker,
		portfolio:  portfolio,
		db:         db,
		cfg:        cfg,
	}
}

// Run starts all periodic loops and blocks until context is cancelled.
func (s *Scheduler) Run(ctx context.Context) error {
	slog.Info("scheduler starting",
		"scan_interval", s.cfg.ScanInterval.Duration,
		"snapshot_interval", s.cfg.SnapshotInterval.Duration,
		"performance_interval", s.cfg.PerformanceInterval.Duration,
	)

	// Initial portfolio refresh.
	if err := s.portfolio.Refresh(); err != nil {
		slog.Error("initial portfolio refresh failed", "error", err)
		return err
	}
	s.snapshotBankroll()

	// Run first cycle immediately.
	s.runTradingCycle(ctx)
	s.runCollection()

	scanTicker := time.NewTicker(s.cfg.ScanInterval.Duration)
	snapshotTicker := time.NewTicker(s.cfg.SnapshotInterval.Duration)
	perfTicker := time.NewTicker(s.cfg.PerformanceInterval.Duration)
	defer scanTicker.Stop()
	defer snapshotTicker.Stop()
	defer perfTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("scheduler shutting down")
			return ctx.Err()
		case <-scanTicker.C:
			s.runTradingCycle(ctx)
		case <-snapshotTicker.C:
			s.runCollection()
		case <-perfTicker.C:
			s.runPerformanceReport()
		}
	}
}

func (s *Scheduler) runTradingCycle(ctx context.Context) {
	slog.Info("starting trading cycle")

	// Refresh portfolio.
	if err := s.portfolio.Refresh(); err != nil {
		slog.Error("portfolio refresh failed", "error", err)
		return
	}
	s.riskMgr.Refresh()

	// Load per-market exposure from DB (unresolved bets).
	s.loadMarketExposure()

	// Scan markets.
	binaryMarkets, err := s.scanner.ScanBinary(200)
	if err != nil {
		slog.Error("binary scan failed", "error", err)
	}

	mcMarkets, err := s.scanner.ScanMultipleChoice(200)
	if err != nil {
		slog.Error("multi-choice scan failed", "error", err)
	}

	allMarkets := append(binaryMarkets, mcMarkets...)
	s.cache.SetAll(allMarkets)

	slog.Info("markets scanned", "binary", len(binaryMarkets), "multi_choice", len(mcMarkets))

	// Evaluate strategies.
	var allSignals []strategy.Signal
	for _, strat := range s.strategies {
		if !strat.Enabled() {
			continue
		}

		signals, err := strat.Evaluate(ctx, allMarkets)
		if err != nil {
			slog.Error("strategy evaluation failed", "strategy", strat.Name(), "error", err)
			continue
		}

		slog.Info("strategy evaluated", "strategy", strat.Name(), "signals", len(signals))
		allSignals = append(allSignals, signals...)
	}

	if len(allSignals) == 0 {
		slog.Info("no trading signals this cycle")
		return
	}

	// Size positions via risk manager.
	sized := s.riskMgr.SizeSignals(allSignals)
	slog.Info("signals sized", "approved", len(sized), "total", len(allSignals))

	if len(sized) == 0 {
		return
	}

	// Ensure markets exist in DB before placing bets.
	for _, sig := range sized {
		for _, m := range allMarkets {
			if m.ID == sig.Signal.MarketID {
				s.executor.EnsureMarketExists(
					m.ID, m.Question, m.OutcomeType, m.Mechanism,
					m.CreatorID, m.CreatedTime.UnixMilli(), m.CloseTime.UnixMilli(), m.URL,
				)
				break
			}
		}
	}

	// Execute.
	results := s.executor.Execute(sized)
	successCount := 0
	for _, r := range results {
		if r.Success {
			s.riskMgr.RecordTrade(r.Signal.Signal.MarketID, r.Signal.Amount)
			successCount++
		}
	}

	slog.Info("trading cycle complete", "executed", successCount, "failed", len(results)-successCount)

	// Snapshot bankroll after trades.
	s.snapshotBankroll()
}

func (s *Scheduler) runCollection() {
	slog.Info("starting data collection")
	if err := s.collector.Collect(); err != nil {
		slog.Error("collection failed", "error", err)
	}
}

func (s *Scheduler) runPerformanceReport() {
	report, err := s.tracker.Generate()
	if err != nil {
		slog.Error("performance report failed", "error", err)
		return
	}

	// Add current balance.
	report.CurrentBalance = s.portfolio.TotalValue

	performance.LogReport(report)
}

func (s *Scheduler) loadMarketExposure() {
	rows, err := s.db.Query(`
		SELECT market_id, SUM(amount)
		FROM bot_bets
		WHERE resolved = 0
		GROUP BY market_id`)
	if err != nil {
		slog.Error("failed to load market exposure", "error", err)
		return
	}
	defer rows.Close()

	exposure := make(map[string]float64)
	for rows.Next() {
		var marketID string
		var amount float64
		if err := rows.Scan(&marketID, &amount); err != nil {
			slog.Error("failed to scan market exposure row", "error", err)
			continue
		}
		exposure[marketID] = amount
	}

	s.riskMgr.SetMarketExposure(exposure)
	if len(exposure) > 0 {
		slog.Info("loaded per-market exposure", "markets", len(exposure))
	}
}

func (s *Scheduler) snapshotBankroll() {
	_, err := s.db.Exec(`
		INSERT INTO bankroll_snapshots (balance, investment_value, total_value)
		VALUES (?, ?, ?)`,
		s.portfolio.Balance, s.portfolio.InvestmentValue, s.portfolio.TotalValue,
	)
	if err != nil {
		slog.Error("bankroll snapshot failed", "error", err)
	}
}
