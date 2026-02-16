package strategy

import (
	"context"
	"testing"
	"time"

	"hyperkaehler/internal/config"
)

func newMarketMakingConfig() config.MarketMakingConfig {
	return config.MarketMakingConfig{
		Enabled:      true,
		BaseSpread:   0.04,
		MinLiquidity: 500,
		MinVolume24h: 50,
	}
}

func TestMarketMaking_GeneratesTwoSignals(t *testing.T) {
	mm := NewMarketMaking(newMarketMakingConfig())

	markets := []MarketData{
		{
			ID:             "mm-1",
			OutcomeType:    "BINARY",
			Probability:    0.50,
			TotalLiquidity: 600,
			Volume24Hours:  100,
			CloseTime:      time.Now().Add(60 * 24 * time.Hour),
		},
	}

	signals, err := mm.Evaluate(context.Background(), markets)
	if err != nil {
		t.Fatal(err)
	}
	if len(signals) != 2 {
		t.Fatalf("expected 2 signals (YES + NO), got %d", len(signals))
	}

	hasYes, hasNo := false, false
	for _, sig := range signals {
		if sig.Outcome == "YES" {
			hasYes = true
			if !sig.IsLimitOrder {
				t.Error("YES signal should be a limit order")
			}
		}
		if sig.Outcome == "NO" {
			hasNo = true
			if !sig.IsLimitOrder {
				t.Error("NO signal should be a limit order")
			}
		}
	}
	if !hasYes || !hasNo {
		t.Error("expected both YES and NO signals")
	}
}

func TestMarketMaking_SkipsLowLiquidity(t *testing.T) {
	mm := NewMarketMaking(newMarketMakingConfig())

	markets := []MarketData{
		{
			ID:             "lowliq-1",
			OutcomeType:    "BINARY",
			Probability:    0.50,
			TotalLiquidity: 100, // Below 500 threshold.
			Volume24Hours:  100,
			CloseTime:      time.Now().Add(60 * 24 * time.Hour),
		},
	}

	signals, err := mm.Evaluate(context.Background(), markets)
	if err != nil {
		t.Fatal(err)
	}
	if len(signals) != 0 {
		t.Errorf("expected 0 signals for low liquidity, got %d", len(signals))
	}
}

func TestMarketMaking_SkipsExtremeProbability(t *testing.T) {
	mm := NewMarketMaking(newMarketMakingConfig())

	markets := []MarketData{
		{
			ID:             "extreme-1",
			OutcomeType:    "BINARY",
			Probability:    0.95, // Outside 0.20-0.80 range.
			TotalLiquidity: 1000,
			Volume24Hours:  100,
			CloseTime:      time.Now().Add(60 * 24 * time.Hour),
		},
	}

	signals, err := mm.Evaluate(context.Background(), markets)
	if err != nil {
		t.Fatal(err)
	}
	if len(signals) != 0 {
		t.Errorf("expected 0 signals for extreme probability, got %d", len(signals))
	}
}

func TestMarketMaking_TightensSpreadForHighLiquidity(t *testing.T) {
	mm := NewMarketMaking(newMarketMakingConfig())

	markets := []MarketData{
		{
			ID:             "highliq-1",
			OutcomeType:    "BINARY",
			Probability:    0.50,
			TotalLiquidity: 2500, // > 2000 should give 0.02 spread.
			Volume24Hours:  100,
			CloseTime:      time.Now().Add(60 * 24 * time.Hour),
		},
	}

	signals, err := mm.Evaluate(context.Background(), markets)
	if err != nil {
		t.Fatal(err)
	}
	if len(signals) != 2 {
		t.Fatalf("expected 2 signals, got %d", len(signals))
	}

	// With spread 0.02 and prob 0.50:
	// YES limit = 0.50 - 0.01 = 0.49
	// NO limit = 0.50 + 0.01 = 0.51
	for _, sig := range signals {
		if sig.Outcome == "YES" && sig.LimitProb != 0.49 {
			t.Errorf("expected YES limit 0.49, got %f", sig.LimitProb)
		}
		if sig.Outcome == "NO" && sig.LimitProb != 0.51 {
			t.Errorf("expected NO limit 0.51, got %f", sig.LimitProb)
		}
	}
}
