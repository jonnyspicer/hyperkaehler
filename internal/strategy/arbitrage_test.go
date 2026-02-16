package strategy

import (
	"context"
	"testing"
	"time"

	"hyperkaehler/internal/config"
)

func TestArbitrage_NoSignalWhenBalanced(t *testing.T) {
	arb := NewArbitrage(config.ArbitrageConfig{
		Enabled:             true,
		MinLiquidity:        50,
		MinProbSumDeviation: 0.10,
		MaxMarketsPerCycle:  20,
	})

	markets := []MarketData{
		{
			ID:             "test-1",
			OutcomeType:    "MULTIPLE_CHOICE",
			TotalLiquidity: 100,
			Answers: []AnswerData{
				{ID: "a1", Text: "Option A", Probability: 0.50},
				{ID: "a2", Text: "Option B", Probability: 0.50},
			},
			CloseTime: time.Now().Add(30 * 24 * time.Hour),
		},
	}

	signals, err := arb.Evaluate(context.Background(), markets)
	if err != nil {
		t.Fatal(err)
	}
	if len(signals) != 0 {
		t.Errorf("expected 0 signals for balanced market, got %d", len(signals))
	}
}

func TestArbitrage_SignalsWhenOverpriced(t *testing.T) {
	arb := NewArbitrage(config.ArbitrageConfig{
		Enabled:             true,
		MinLiquidity:        50,
		MinProbSumDeviation: 0.10,
		MaxMarketsPerCycle:  20,
	})

	// Probabilities sum to 1.3 — 30% overpriced.
	markets := []MarketData{
		{
			ID:             "test-over",
			OutcomeType:    "MULTIPLE_CHOICE",
			TotalLiquidity: 200,
			Answers: []AnswerData{
				{ID: "a1", Text: "Option A", Probability: 0.50},
				{ID: "a2", Text: "Option B", Probability: 0.45},
				{ID: "a3", Text: "Option C", Probability: 0.35},
			},
			CloseTime: time.Now().Add(30 * 24 * time.Hour),
		},
	}

	signals, err := arb.Evaluate(context.Background(), markets)
	if err != nil {
		t.Fatal(err)
	}
	if len(signals) == 0 {
		t.Fatal("expected signals for overpriced market")
	}
	for _, sig := range signals {
		if sig.Outcome != "NO" {
			t.Errorf("expected NO outcome for overpriced market, got %s", sig.Outcome)
		}
		if sig.Edge <= 0 {
			t.Errorf("expected positive edge, got %f", sig.Edge)
		}
		if sig.Strategy != "arbitrage" {
			t.Errorf("expected strategy 'arbitrage', got %s", sig.Strategy)
		}
	}
}

func TestArbitrage_SignalsWhenUnderpriced(t *testing.T) {
	arb := NewArbitrage(config.ArbitrageConfig{
		Enabled:             true,
		MinLiquidity:        50,
		MinProbSumDeviation: 0.10,
		MaxMarketsPerCycle:  20,
	})

	// Probabilities sum to 0.70 — 30% underpriced.
	markets := []MarketData{
		{
			ID:             "test-under",
			OutcomeType:    "MULTIPLE_CHOICE",
			TotalLiquidity: 200,
			Answers: []AnswerData{
				{ID: "a1", Text: "Option A", Probability: 0.30},
				{ID: "a2", Text: "Option B", Probability: 0.25},
				{ID: "a3", Text: "Option C", Probability: 0.15},
			},
			CloseTime: time.Now().Add(30 * 24 * time.Hour),
		},
	}

	signals, err := arb.Evaluate(context.Background(), markets)
	if err != nil {
		t.Fatal(err)
	}
	if len(signals) == 0 {
		t.Fatal("expected signals for underpriced market")
	}
	for _, sig := range signals {
		if sig.Outcome != "YES" {
			t.Errorf("expected YES outcome for underpriced market, got %s", sig.Outcome)
		}
		if sig.Edge <= 0 {
			t.Errorf("expected positive edge, got %f", sig.Edge)
		}
	}
}

func TestArbitrage_SkipsBinaryMarkets(t *testing.T) {
	arb := NewArbitrage(config.ArbitrageConfig{
		Enabled:             true,
		MinLiquidity:        50,
		MinProbSumDeviation: 0.10,
		MaxMarketsPerCycle:  20,
	})

	markets := []MarketData{
		{
			ID:             "binary-1",
			OutcomeType:    "BINARY",
			TotalLiquidity: 200,
			Probability:    0.60,
		},
	}

	signals, err := arb.Evaluate(context.Background(), markets)
	if err != nil {
		t.Fatal(err)
	}
	if len(signals) != 0 {
		t.Errorf("expected 0 signals for binary market, got %d", len(signals))
	}
}

func TestArbitrage_SkipsResolvedAnswers(t *testing.T) {
	arb := NewArbitrage(config.ArbitrageConfig{
		Enabled:             true,
		MinLiquidity:        50,
		MinProbSumDeviation: 0.10,
		MaxMarketsPerCycle:  20,
	})

	// Market with 3 answers summing to 1.36, but one is CANCEL-resolved.
	// Without the resolution filter, this would look like arbitrage.
	// With the filter, the cancelled answer (0.76) is removed,
	// leaving two answers summing to 0.60, which triggers YES signals.
	markets := []MarketData{
		{
			ID:             "test-resolved",
			OutcomeType:    "MULTIPLE_CHOICE",
			TotalLiquidity: 200,
			Answers: []AnswerData{
				{ID: "a1", Text: "Option A", Probability: 0.35},
				{ID: "a2", Text: "Option B", Probability: 0.25},
				{ID: "a3", Text: "Cancelled", Probability: 0.76, Resolution: "CANCEL"},
			},
			CloseTime: time.Now().Add(30 * 24 * time.Hour),
		},
	}

	signals, err := arb.Evaluate(context.Background(), markets)
	if err != nil {
		t.Fatal(err)
	}

	// The cancelled answer should be excluded. Remaining: 0.35 + 0.25 = 0.60.
	// This is 0.40 below 1.0, triggering YES signals.
	for _, sig := range signals {
		if sig.AnswerID == "a3" {
			t.Error("should not generate signal for CANCEL-resolved answer")
		}
		if sig.Outcome != "YES" {
			t.Errorf("expected YES for underpriced market, got %s", sig.Outcome)
		}
	}
}

func TestArbitrage_SkipsLowLiquidity(t *testing.T) {
	arb := NewArbitrage(config.ArbitrageConfig{
		Enabled:             true,
		MinLiquidity:        50,
		MinProbSumDeviation: 0.10,
		MaxMarketsPerCycle:  20,
	})

	markets := []MarketData{
		{
			ID:             "low-liq",
			OutcomeType:    "MULTIPLE_CHOICE",
			TotalLiquidity: 10, // Below threshold.
			Answers: []AnswerData{
				{ID: "a1", Text: "A", Probability: 0.80},
				{ID: "a2", Text: "B", Probability: 0.80},
			},
		},
	}

	signals, err := arb.Evaluate(context.Background(), markets)
	if err != nil {
		t.Fatal(err)
	}
	if len(signals) != 0 {
		t.Errorf("expected 0 signals for low liquidity market, got %d", len(signals))
	}
}

func TestArbitrage_SkipsFarCloseTime(t *testing.T) {
	arb := NewArbitrage(config.ArbitrageConfig{
		Enabled:             true,
		MinLiquidity:        50,
		MinProbSumDeviation: 0.10,
		MaxMarketsPerCycle:  20,
		MaxCloseDays:        90,
	})

	markets := []MarketData{
		{
			ID:             "far-close",
			OutcomeType:    "MULTIPLE_CHOICE",
			TotalLiquidity: 200,
			CloseTime:      time.Now().AddDate(1, 0, 0), // 1 year out.
			Answers: []AnswerData{
				{ID: "a1", Text: "A", Probability: 0.80},
				{ID: "a2", Text: "B", Probability: 0.80},
			},
		},
		{
			ID:             "near-close",
			OutcomeType:    "MULTIPLE_CHOICE",
			TotalLiquidity: 200,
			CloseTime:      time.Now().Add(30 * 24 * time.Hour), // 30 days out.
			Answers: []AnswerData{
				{ID: "a1", Text: "A", Probability: 0.80},
				{ID: "a2", Text: "B", Probability: 0.80},
			},
		},
	}

	signals, err := arb.Evaluate(context.Background(), markets)
	if err != nil {
		t.Fatal(err)
	}

	// Only near-close market should generate signals.
	for _, sig := range signals {
		if sig.MarketID == "far-close" {
			t.Error("should not generate signals for market closing > 90 days out")
		}
	}
	if len(signals) == 0 {
		t.Error("expected signals for near-close market")
	}
}
