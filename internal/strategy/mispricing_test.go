package strategy

import (
	"context"
	"testing"
	"time"

	"hyperkaehler/internal/config"
)

func newMispricingConfig() config.MispricingConfig {
	return config.MispricingConfig{
		Enabled:              true,
		ExtremeThresholdHigh: 0.95,
		ExtremeThresholdLow:  0.05,
		MinMarketAgeDays:     7,
		MinVolume:            100,
	}
}

func TestMispricing_HighExtreme(t *testing.T) {
	m := NewMispricing(newMispricingConfig())
	markets := []MarketData{
		{
			ID:          "high-1",
			OutcomeType: "BINARY",
			Probability: 0.96,
			Volume:      500,
			CreatedTime: time.Now().Add(-30 * 24 * time.Hour),
			CloseTime:   time.Now().Add(30 * 24 * time.Hour),
		},
	}

	signals, err := m.Evaluate(context.Background(), markets)
	if err != nil {
		t.Fatal(err)
	}
	if len(signals) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(signals))
	}
	if signals[0].Outcome != "YES" {
		t.Errorf("expected YES outcome, got %s", signals[0].Outcome)
	}
	if signals[0].Edge <= 0 {
		t.Errorf("expected positive edge, got %f", signals[0].Edge)
	}
}

func TestMispricing_LowExtreme(t *testing.T) {
	m := NewMispricing(newMispricingConfig())
	markets := []MarketData{
		{
			ID:          "low-1",
			OutcomeType: "BINARY",
			Probability: 0.04, // Edge = 0.04 - 0.03 = 0.01 (positive).
			Volume:      200,
			CreatedTime: time.Now().Add(-30 * 24 * time.Hour),
			CloseTime:   time.Now().Add(30 * 24 * time.Hour),
		},
	}

	signals, err := m.Evaluate(context.Background(), markets)
	if err != nil {
		t.Fatal(err)
	}
	if len(signals) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(signals))
	}
	if signals[0].Outcome != "NO" {
		t.Errorf("expected NO outcome, got %s", signals[0].Outcome)
	}
}

func TestMispricing_SkipsTooNew(t *testing.T) {
	m := NewMispricing(newMispricingConfig())
	markets := []MarketData{
		{
			ID:          "new-1",
			OutcomeType: "BINARY",
			Probability: 0.97,
			Volume:      500,
			CreatedTime: time.Now().Add(-2 * 24 * time.Hour), // Only 2 days old.
			CloseTime:   time.Now().Add(30 * 24 * time.Hour),
		},
	}

	signals, err := m.Evaluate(context.Background(), markets)
	if err != nil {
		t.Fatal(err)
	}
	if len(signals) != 0 {
		t.Errorf("expected 0 signals for new market, got %d", len(signals))
	}
}

func TestMispricing_SkipsLowVolume(t *testing.T) {
	m := NewMispricing(newMispricingConfig())
	markets := []MarketData{
		{
			ID:          "lowvol-1",
			OutcomeType: "BINARY",
			Probability: 0.97,
			Volume:      10, // Below threshold.
			CreatedTime: time.Now().Add(-30 * 24 * time.Hour),
			CloseTime:   time.Now().Add(30 * 24 * time.Hour),
		},
	}

	signals, err := m.Evaluate(context.Background(), markets)
	if err != nil {
		t.Fatal(err)
	}
	if len(signals) != 0 {
		t.Errorf("expected 0 signals for low volume market, got %d", len(signals))
	}
}

func TestMispricing_SkipsCloseToExpiry(t *testing.T) {
	m := NewMispricing(newMispricingConfig())
	markets := []MarketData{
		{
			ID:          "closing-1",
			OutcomeType: "BINARY",
			Probability: 0.97,
			Volume:      500,
			CreatedTime: time.Now().Add(-30 * 24 * time.Hour),
			CloseTime:   time.Now().Add(5 * 24 * time.Hour), // Closes in 5 days.
		},
	}

	signals, err := m.Evaluate(context.Background(), markets)
	if err != nil {
		t.Fatal(err)
	}
	if len(signals) != 0 {
		t.Errorf("expected 0 signals for near-close market, got %d", len(signals))
	}
}
