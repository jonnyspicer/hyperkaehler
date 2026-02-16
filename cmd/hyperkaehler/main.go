package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jonnyspicer/mango"

	"hyperkaehler/internal/backtest"
	"hyperkaehler/internal/collector"
	"hyperkaehler/internal/config"
	"hyperkaehler/internal/db"
	"hyperkaehler/internal/execution"
	"hyperkaehler/internal/market"
	"hyperkaehler/internal/performance"
	"hyperkaehler/internal/risk"
	"hyperkaehler/internal/scheduler"
	"hyperkaehler/internal/strategy"
)

func main() {
	// Parse CLI flags.
	backtestMode := flag.Bool("backtest", false, "Run in backtest mode against historical data")
	backtestFrom := flag.String("from", "", "Backtest start date (YYYY-MM-DD)")
	backtestTo := flag.String("to", "", "Backtest end date (YYYY-MM-DD)")
	backtestBalance := flag.Float64("balance", 2300, "Starting balance for backtest simulation")
	flag.Parse()

	// Set up structured logging.
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	slog.Info("hyperkaehler starting")

	// Load configuration.
	configPath := "config.toml"
	if p := os.Getenv("HK_CONFIG_PATH"); p != "" {
		configPath = p
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Initialize database.
	database, err := db.Open(cfg.General.DBPath)
	if err != nil {
		slog.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	if err := db.Migrate(database); err != nil {
		slog.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}
	slog.Info("database initialized", "path", cfg.General.DBPath)

	// Register strategies.
	strategies := []strategy.Strategy{
		strategy.NewArbitrage(cfg.Strategy.Arbitrage),
		strategy.NewMispricing(cfg.Strategy.Mispricing),
		strategy.NewTimeDecay(cfg.Strategy.TimeDecay),
		strategy.NewMarketMaking(cfg.Strategy.MarketMaking),
	}
	slog.Info("strategies registered", "count", len(strategies))

	// Backtest mode.
	if *backtestMode {
		runner := backtest.NewRunner(database, strategies, cfg.Risk, *backtestBalance)
		if err := runner.Run(*backtestFrom, *backtestTo); err != nil {
			slog.Error("backtest failed", "error", err)
			os.Exit(1)
		}
		return
	}

	// Live mode — initialize Manifold client.
	mc := mango.DefaultClientInstance()
	slog.Info("manifold client initialized")

	scanner := market.NewScanner(mc)
	cache := market.NewCache(10 * time.Minute)
	portfolio := risk.NewPortfolio(mc)
	riskMgr := risk.NewManager(cfg.Risk, portfolio)
	executor := execution.NewExecutor(mc, database)
	coll := collector.NewCollector(scanner, database, cfg.Collector)
	tracker := performance.NewTracker(database)

	sched := scheduler.New(
		scanner, cache, strategies, riskMgr, executor,
		coll, tracker, portfolio, database, cfg.Schedule,
	)

	// Graceful shutdown.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		slog.Info("received signal, shutting down", "signal", sig)
		cancel()
	}()

	if err := sched.Run(ctx); err != nil && err != context.Canceled {
		slog.Error("scheduler error", "error", err)
		os.Exit(1)
	}

	slog.Info("hyperkaehler stopped")
}
