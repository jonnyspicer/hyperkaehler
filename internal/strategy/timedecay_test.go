package strategy

import (
	"context"
	"testing"
	"time"

	"hyperkaehler/internal/config"
)

func newTimeDecayConfig() config.TimeDecayConfig {
	return config.TimeDecayConfig{
		Enabled:                true,
		MinTimeElapsedFraction: 0.50,
		MinEdge:                0.08,
		MinVolume:              50,
	}
}

func TestTimeDecay_GeneratesSignal(t *testing.T) {
	td := NewTimeDecay(newTimeDecayConfig())

	// Market 80% through its duration, at 40% probability, with time-related question.
	now := time.Now()
	created := now.Add(-80 * 24 * time.Hour)
	closes := now.Add(20 * 24 * time.Hour) // 100 day total, 80 elapsed.

	markets := []MarketData{
		{
			ID:          "decay-1",
			OutcomeType: "BINARY",
			Probability: 0.40,
			Volume:      200,
			Question:    "Will X happen by December 2026?",
			CreatedTime: created,
			CloseTime:   closes,
		},
	}

	signals, err := td.Evaluate(context.Background(), markets)
	if err != nil {
		t.Fatal(err)
	}
	if len(signals) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(signals))
	}
	if signals[0].Outcome != "NO" {
		t.Errorf("expected NO outcome, got %s", signals[0].Outcome)
	}
	if signals[0].IsLimitOrder != true {
		t.Error("expected limit order")
	}
}

func TestTimeDecay_SkipsNoTimePattern(t *testing.T) {
	td := NewTimeDecay(newTimeDecayConfig())

	markets := []MarketData{
		{
			ID:          "notime-1",
			OutcomeType: "BINARY",
			Probability: 0.40,
			Volume:      200,
			Question:    "Will the price of Bitcoin exceed $100k?", // No time pattern.
			CreatedTime: time.Now().Add(-80 * 24 * time.Hour),
			CloseTime:   time.Now().Add(20 * 24 * time.Hour),
		},
	}

	signals, err := td.Evaluate(context.Background(), markets)
	if err != nil {
		t.Fatal(err)
	}
	if len(signals) != 0 {
		t.Errorf("expected 0 signals for no-time-pattern question, got %d", len(signals))
	}
}

func TestTimeDecay_SkipsHighProbability(t *testing.T) {
	td := NewTimeDecay(newTimeDecayConfig())

	markets := []MarketData{
		{
			ID:          "highprob-1",
			OutcomeType: "BINARY",
			Probability: 0.60, // Above 0.50 threshold.
			Volume:      200,
			Question:    "Will X happen by December 2026?",
			CreatedTime: time.Now().Add(-80 * 24 * time.Hour),
			CloseTime:   time.Now().Add(20 * 24 * time.Hour),
		},
	}

	signals, err := td.Evaluate(context.Background(), markets)
	if err != nil {
		t.Fatal(err)
	}
	if len(signals) != 0 {
		t.Errorf("expected 0 signals for high probability market, got %d", len(signals))
	}
}

func TestTimeDecay_SkipsEarlyMarket(t *testing.T) {
	td := NewTimeDecay(newTimeDecayConfig())

	// Only 20% through duration.
	now := time.Now()
	created := now.Add(-20 * 24 * time.Hour)
	closes := now.Add(80 * 24 * time.Hour)

	markets := []MarketData{
		{
			ID:          "early-1",
			OutcomeType: "BINARY",
			Probability: 0.40,
			Volume:      200,
			Question:    "Will X happen by December 2026?",
			CreatedTime: created,
			CloseTime:   closes,
		},
	}

	signals, err := td.Evaluate(context.Background(), markets)
	if err != nil {
		t.Fatal(err)
	}
	if len(signals) != 0 {
		t.Errorf("expected 0 signals for early market, got %d", len(signals))
	}
}
