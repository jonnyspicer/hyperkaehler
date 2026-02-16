package strategy

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"hyperkaehler/internal/config"
)

// Arbitrage finds multi-choice markets where answer probabilities don't sum to ~100%.
type Arbitrage struct {
	cfg config.ArbitrageConfig
}

func NewArbitrage(cfg config.ArbitrageConfig) *Arbitrage {
	return &Arbitrage{cfg: cfg}
}

func (a *Arbitrage) Name() string     { return "arbitrage" }
func (a *Arbitrage) Enabled() bool    { return a.cfg.Enabled }

func (a *Arbitrage) Evaluate(_ context.Context, markets []MarketData) ([]Signal, error) {
	var signals []Signal
	evaluated := 0

	now := time.Now()

	for _, m := range markets {
		if m.OutcomeType != "MULTIPLE_CHOICE" {
			continue
		}
		if m.IsResolved {
			continue
		}
		if m.TotalLiquidity < a.cfg.MinLiquidity {
			continue
		}
		if len(m.Answers) < 2 {
			continue
		}
		// Skip markets that close too far in the future to avoid locking up capital.
		if a.cfg.MaxCloseDays > 0 && m.CloseTime.After(now.AddDate(0, 0, a.cfg.MaxCloseDays)) {
			continue
		}

		evaluated++
		if evaluated > a.cfg.MaxMarketsPerCycle {
			break
		}

		sigs := a.evaluateMarket(m)
		signals = append(signals, sigs...)
	}

	slog.Info("arbitrage evaluation complete", "markets_evaluated", evaluated, "signals", len(signals))
	return signals, nil
}

func (a *Arbitrage) evaluateMarket(m MarketData) []Signal {
	// Filter out resolved answers (explicitly resolved or probability pinned to 0/1).
	var active []AnswerData
	for _, ans := range m.Answers {
		if ans.Resolution != "" {
			continue // Skip YES, NO, CANCEL resolved answers.
		}
		if ans.Probability > 0.001 && ans.Probability < 0.999 {
			active = append(active, ans)
		}
	}
	if len(active) < 2 {
		return nil
	}

	// Sum all active answer probabilities.
	var total float64
	for _, ans := range active {
		total += ans.Probability
	}

	deviation := total - 1.0

	// Not enough deviation to profit.
	if deviation > -a.cfg.MinProbSumDeviation && deviation < a.cfg.MinProbSumDeviation {
		return nil
	}

	slog.Debug("arbitrage opportunity found",
		"market", m.ID,
		"question", m.Question,
		"prob_sum", total,
		"deviation", deviation,
		"answers", len(m.Answers),
	)

	var signals []Signal

	if deviation > a.cfg.MinProbSumDeviation {
		// Probabilities sum to more than 100%: answers are overpriced.
		// Bet NO on the most overpriced answers.
		for _, ans := range active {
			fairValue := ans.Probability / total
			edge := ans.Probability - fairValue

			if edge < 0.03 { // minimum per-answer edge
				continue
			}

			// For a NO bet, our confidence in NO = 1 - fairValue.
			// The market's implied NO prob = 1 - ans.Probability.
			confidence := 1.0 - fairValue

			signals = append(signals, Signal{
				MarketID:     m.ID,
				AnswerID:     ans.ID,
				Outcome:      "NO",
				Confidence:   confidence,
				MarketProb:   ans.Probability,
				Edge:         edge,
				Strategy:     "arbitrage",
				Reason:       fmt.Sprintf("probs sum to %.2f, answer '%s' at %.2f vs fair %.2f", total, ans.Text, ans.Probability, fairValue),
				IsLimitOrder: true,
				LimitProb:    (ans.Probability + fairValue) / 2, // Midpoint between market and fair.
			})
		}
	} else {
		// Probabilities sum to less than 100%: answers are underpriced.
		// Bet YES on the most underpriced answers.
		for _, ans := range active {
			fairValue := ans.Probability / total
			edge := fairValue - ans.Probability

			if edge < 0.03 {
				continue
			}

			signals = append(signals, Signal{
				MarketID:     m.ID,
				AnswerID:     ans.ID,
				Outcome:      "YES",
				Confidence:   fairValue,
				MarketProb:   ans.Probability,
				Edge:         edge,
				Strategy:     "arbitrage",
				Reason:       fmt.Sprintf("probs sum to %.2f, answer '%s' at %.2f vs fair %.2f", total, ans.Text, ans.Probability, fairValue),
				IsLimitOrder: true,
				LimitProb:    (ans.Probability + fairValue) / 2,
			})
		}
	}

	return signals
}
