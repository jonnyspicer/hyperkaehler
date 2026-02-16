package risk

import (
	"testing"

	"hyperkaehler/internal/config"
	"hyperkaehler/internal/strategy"
)

func newTestManager(balance float64) *Manager {
	portfolio := &Portfolio{
		Balance:         balance,
		InvestmentValue: 0,
		TotalValue:      balance,
	}
	cfg := config.RiskConfig{
		KellyFraction:        0.25,
		MaxPositionPct:       0.05,
		MaxMarketExposurePct: 0.10,
		MaxTotalExposure:     0.50,
		MaxDrawdownPct:       0.20,
		MinBetAmount:         1.0,
		MinEdge:              0.05,
	}
	return NewManager(cfg, portfolio)
}

func TestCanTrade_WithBalance(t *testing.T) {
	m := newTestManager(2300)
	if !m.CanTrade() {
		t.Error("expected CanTrade to return true with balance")
	}
}

func TestCanTrade_ZeroBalance(t *testing.T) {
	m := newTestManager(0)
	if m.CanTrade() {
		t.Error("expected CanTrade to return false with zero balance")
	}
}

func TestCanTrade_DrawdownHalt(t *testing.T) {
	m := newTestManager(2300)
	m.peakBalance = 3000 // Set a peak much higher than current.
	// Drawdown = (3000 - 2300) / 3000 = 0.233 > 0.20.
	if m.CanTrade() {
		t.Error("expected CanTrade to return false during drawdown")
	}
}

func TestCanTrade_ExposureLimit(t *testing.T) {
	m := newTestManager(2300)
	// Max exposure = 50% of 2300 = 1150.
	m.totalExposure = 1200
	if m.CanTrade() {
		t.Error("expected CanTrade to return false at exposure limit")
	}
}

func TestSizeSignals_RejectsLowEdge(t *testing.T) {
	m := newTestManager(2300)
	signals := []strategy.Signal{
		{
			MarketID:   "test",
			Outcome:    "YES",
			Confidence: 0.52,
			MarketProb: 0.50,
			Edge:       0.02, // Below min_edge of 0.05.
			Strategy:   "test",
		},
	}
	sized := m.SizeSignals(signals)
	if len(sized) != 0 {
		t.Errorf("expected 0 sized signals for low edge, got %d", len(sized))
	}
}

func TestSizeSignals_ApproveGoodEdge(t *testing.T) {
	m := newTestManager(2300)
	signals := []strategy.Signal{
		{
			MarketID:   "test",
			Outcome:    "YES",
			Confidence: 0.70,
			MarketProb: 0.50,
			Edge:       0.20,
			Strategy:   "test",
		},
	}
	sized := m.SizeSignals(signals)
	if len(sized) != 1 {
		t.Fatalf("expected 1 sized signal, got %d", len(sized))
	}
	if sized[0].Amount < 1 {
		t.Errorf("expected amount >= 1, got %f", sized[0].Amount)
	}
	// Max position = 5% of 2300 = 115.
	if sized[0].Amount > 115 {
		t.Errorf("expected amount <= 115 (max position), got %f", sized[0].Amount)
	}
}

func TestSizeSignals_CapsAtMaxPosition(t *testing.T) {
	m := newTestManager(2300)
	signals := []strategy.Signal{
		{
			MarketID:   "test",
			Outcome:    "YES",
			Confidence: 0.99,
			MarketProb: 0.10,
			Edge:       0.89, // Massive edge to get huge Kelly.
			Strategy:   "test",
		},
	}
	sized := m.SizeSignals(signals)
	if len(sized) != 1 {
		t.Fatalf("expected 1 sized signal, got %d", len(sized))
	}
	maxPosition := 0.05 * 2300
	if sized[0].Amount > maxPosition {
		t.Errorf("expected amount <= %f (max position), got %f", maxPosition, sized[0].Amount)
	}
}

func TestRecordTrade_IncreasesExposure(t *testing.T) {
	m := newTestManager(2300)
	m.RecordTrade("market-1", 50)
	m.RecordTrade("market-2", 30)
	if m.totalExposure != 80 {
		t.Errorf("expected total exposure 80, got %f", m.totalExposure)
	}
	if m.marketExposure["market-1"] != 50 {
		t.Errorf("expected market-1 exposure 50, got %f", m.marketExposure["market-1"])
	}
}

func TestSizeSignals_MarketExposureCap(t *testing.T) {
	m := newTestManager(2300)
	// Pre-load 200 mana of exposure on market-1 (out of 230 cap at 10%).
	m.SetMarketExposure(map[string]float64{"market-1": 200})

	signals := []strategy.Signal{
		{
			MarketID:   "market-1",
			Outcome:    "YES",
			Confidence: 0.90,
			MarketProb: 0.50,
			Edge:       0.40,
			Strategy:   "test",
		},
		{
			MarketID:   "market-2",
			Outcome:    "YES",
			Confidence: 0.90,
			MarketProb: 0.50,
			Edge:       0.40,
			Strategy:   "test",
		},
	}

	sized := m.SizeSignals(signals)

	// market-1 should be capped to ~30 mana remaining (230 cap - 200 existing).
	// market-2 should be uncapped.
	var market1Amount, market2Amount float64
	for _, s := range sized {
		if s.Signal.MarketID == "market-1" {
			market1Amount = s.Amount
		} else {
			market2Amount = s.Amount
		}
	}

	maxMarketExposure := 0.10 * 2300.0 // 230
	if market1Amount > maxMarketExposure-200+1 {
		t.Errorf("market-1 amount should be capped to ~30, got %f", market1Amount)
	}
	if market2Amount <= market1Amount {
		t.Errorf("market-2 should have higher amount than capped market-1, got %f vs %f", market2Amount, market1Amount)
	}
}
