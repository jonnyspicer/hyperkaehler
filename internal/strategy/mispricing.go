package strategy

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"hyperkaehler/internal/config"
)

// Mispricing finds binary markets where the probability is likely mispriced,
// either through extreme probability confirmation or mean reversion on sudden moves.
type Mispricing struct {
	cfg config.MispricingConfig
}

func NewMispricing(cfg config.MispricingConfig) *Mispricing {
	return &Mispricing{cfg: cfg}
}

func (m *Mispricing) Name() string  { return "mispricing" }
func (m *Mispricing) Enabled() bool { return m.cfg.Enabled }

func (m *Mispricing) Evaluate(_ context.Context, markets []MarketData) ([]Signal, error) {
	var signals []Signal
	now := time.Now()

	for _, mkt := range markets {
		if mkt.OutcomeType != "BINARY" {
			continue
		}
		if mkt.IsResolved {
			continue
		}

		sigs := m.evaluateExtreme(mkt, now)
		signals = append(signals, sigs...)

		// TODO: Sub-strategy B (mean reversion on sudden moves).
		// Requires historical probability snapshots to compare current
		// probability against a recent previous value. Implement once
		// the snapshot store is available in-strategy.
	}

	slog.Info("mispricing evaluation complete", "signals", len(signals))
	return signals, nil
}

// evaluateExtreme implements sub-strategy A: extreme probability confirmation.
// Markets trading near 0 or 1 that have been open long enough with sufficient
// volume are likely correctly priced. We confirm the extreme and bet with it.
func (m *Mispricing) evaluateExtreme(mkt MarketData, now time.Time) []Signal {
	prob := mkt.Probability

	isHigh := prob > m.cfg.ExtremeThresholdHigh
	isLow := prob < m.cfg.ExtremeThresholdLow
	if !isHigh && !isLow {
		return nil
	}

	age := now.Sub(mkt.CreatedTime)
	minAge := time.Duration(m.cfg.MinMarketAgeDays) * 24 * time.Hour
	if age < minAge {
		return nil
	}

	if mkt.Volume < m.cfg.MinVolume {
		return nil
	}

	timeUntilClose := mkt.CloseTime.Sub(now)
	if timeUntilClose < 14*24*time.Hour {
		return nil
	}

	const confidence = 0.97

	var outcome string
	var edge float64
	if isHigh {
		outcome = "YES"
		edge = confidence - prob
	} else {
		outcome = "NO"
		edge = prob - (1 - confidence) // bot's NO prob is 0.97, market's NO prob is (1-prob), edge = 0.97-(1-prob) = prob-0.03
	}

	// Edge can be negative if the market is already past our confidence level.
	if edge <= 0 {
		return nil
	}

	slog.Debug("extreme probability confirmed",
		"market", mkt.ID,
		"question", mkt.Question,
		"probability", prob,
		"outcome", outcome,
		"edge", edge,
	)

	return []Signal{{
		MarketID:   mkt.ID,
		Outcome:    outcome,
		Confidence: confidence,
		MarketProb: prob,
		Edge:       edge,
		Strategy:   "mispricing",
		Reason:     fmt.Sprintf("extreme probability confirmation: market at %.2f, betting %s with confidence %.2f", prob, outcome, confidence),
	}}
}
