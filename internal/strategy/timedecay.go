package strategy

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"time"

	"hyperkaehler/internal/config"
)

var timePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)by (January|February|March|April|May|June|July|August|September|October|November|December) (\d{4})`),
	regexp.MustCompile(`(?i)before (January|February|March|April|May|June|July|August|September|October|November|December) \d{1,2}`),
	regexp.MustCompile(`(?i)in (\d{4})`),
	regexp.MustCompile(`(?i)by end of (\d{4})`),
	regexp.MustCompile(`(?i)by Q[1-4] (\d{4})`),
}

// TimeDecay targets "Will X happen by DATE?" markets that should decay toward NO as time passes.
type TimeDecay struct {
	cfg config.TimeDecayConfig
}

func NewTimeDecay(cfg config.TimeDecayConfig) *TimeDecay {
	return &TimeDecay{cfg: cfg}
}

func (t *TimeDecay) Name() string  { return "timedecay" }
func (t *TimeDecay) Enabled() bool { return t.cfg.Enabled }

func (t *TimeDecay) Evaluate(_ context.Context, markets []MarketData) ([]Signal, error) {
	var signals []Signal
	evaluated := 0

	now := time.Now()

	for _, m := range markets {
		if m.OutcomeType != "BINARY" {
			continue
		}
		if m.IsResolved {
			continue
		}
		if m.Probability >= 0.50 {
			continue
		}
		if m.Volume <= t.cfg.MinVolume {
			continue
		}
		if !matchesTimePattern(m.Question) {
			continue
		}

		evaluated++

		sig, ok := t.evaluateMarket(m, now)
		if ok {
			signals = append(signals, sig)
		}
	}

	slog.Info("timedecay evaluation complete", "markets_evaluated", evaluated, "signals", len(signals))
	return signals, nil
}

func matchesTimePattern(question string) bool {
	for _, pat := range timePatterns {
		if pat.MatchString(question) {
			return true
		}
	}
	return false
}

func (t *TimeDecay) evaluateMarket(m MarketData, now time.Time) (Signal, bool) {
	totalDuration := m.CloseTime.Sub(m.CreatedTime).Seconds()
	if totalDuration <= 0 {
		return Signal{}, false
	}

	elapsed := now.Sub(m.CreatedTime).Seconds()
	timeElapsedFraction := elapsed / totalDuration

	if timeElapsedFraction <= t.cfg.MinTimeElapsedFraction {
		return Signal{}, false
	}

	// Cap the fraction at 1.0 for markets past their close time.
	if timeElapsedFraction > 1.0 {
		timeElapsedFraction = 1.0
	}

	estimatedProb := m.Probability * (1 - timeElapsedFraction*0.5)
	edge := m.Probability - estimatedProb

	if edge <= t.cfg.MinEdge {
		return Signal{}, false
	}

	slog.Debug("timedecay opportunity found",
		"market", m.ID,
		"question", m.Question,
		"market_prob", m.Probability,
		"estimated_prob", estimatedProb,
		"edge", edge,
		"time_elapsed_fraction", timeElapsedFraction,
	)

	confidence := 1 - estimatedProb

	return Signal{
		MarketID:     m.ID,
		Outcome:      "NO",
		Confidence:   confidence,
		MarketProb:   m.Probability,
		Edge:         edge,
		Strategy:     "timedecay",
		Reason:       fmt.Sprintf("time decay: %.0f%% elapsed, prob %.2f -> est %.2f, edge %.2f", timeElapsedFraction*100, m.Probability, estimatedProb, edge),
		IsLimitOrder: true,
		LimitProb:    (m.Probability + estimatedProb) / 2,
	}, true
}
