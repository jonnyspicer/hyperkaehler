package performance

import (
	"log/slog"
)

// LogReport logs the performance report as structured JSON.
func LogReport(r *Report) {
	slog.Info("=== PERFORMANCE REPORT ===",
		"total_bets", r.TotalBets,
		"resolved_bets", r.ResolvedBets,
		"mana_wagered", r.TotalManaWagered,
		"total_pnl", r.TotalPnL,
		"roi", r.ROI,
		"win_rate", r.WinRate,
		"peak_balance", r.PeakBalance,
		"max_drawdown", r.MaxDrawdown,
	)

	for name, stats := range r.StrategyStats {
		slog.Info("strategy performance",
			"strategy", name,
			"bets", stats.BetCount,
			"wagered", stats.ManaWagered,
			"pnl", stats.PnL,
			"roi", stats.ROI,
			"win_rate", stats.WinRate,
			"avg_edge", stats.AvgEdge,
		)
	}
}
