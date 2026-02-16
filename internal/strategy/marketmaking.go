package strategy

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"hyperkaehler/internal/config"
)

// MarketMaking places limit orders on both sides of liquid binary markets
// to capture the bid-ask spread.
type MarketMaking struct {
	cfg config.MarketMakingConfig
}

func NewMarketMaking(cfg config.MarketMakingConfig) *MarketMaking {
	return &MarketMaking{cfg: cfg}
}

func (mm *MarketMaking) Name() string  { return "marketmaking" }
func (mm *MarketMaking) Enabled() bool { return mm.cfg.Enabled }

func (mm *MarketMaking) Evaluate(_ context.Context, markets []MarketData) ([]Signal, error) {
	var signals []Signal

	for _, m := range markets {
		if !mm.isEligible(m) {
			continue
		}

		spread := mm.calculateSpread(m)
		sigs := mm.generateSignals(m, spread)
		signals = append(signals, sigs...)
	}

	slog.Info("marketmaking evaluation complete", "signals", len(signals))
	return signals, nil
}

func (mm *MarketMaking) isEligible(m MarketData) bool {
	if m.OutcomeType != "BINARY" {
		return false
	}
	if m.IsResolved {
		return false
	}
	if m.TotalLiquidity < mm.cfg.MinLiquidity {
		return false
	}
	if m.Volume24Hours < mm.cfg.MinVolume24h {
		return false
	}
	if m.Probability < 0.20 || m.Probability > 0.80 {
		return false
	}
	if time.Until(m.CloseTime) < 30*24*time.Hour {
		return false
	}
	return true
}

func (mm *MarketMaking) calculateSpread(m MarketData) float64 {
	switch {
	case m.TotalLiquidity > 2000:
		return 0.02
	case m.TotalLiquidity > 1000:
		return 0.03
	default:
		return mm.cfg.BaseSpread
	}
}

func (mm *MarketMaking) generateSignals(m MarketData, spread float64) []Signal {
	halfSpread := spread / 2

	slog.Debug("marketmaking opportunity",
		"market", m.ID,
		"question", m.Question,
		"prob", m.Probability,
		"spread", spread,
		"liquidity", m.TotalLiquidity,
	)

	yesBuyProb := m.Probability - halfSpread
	noBuyProb := m.Probability + halfSpread

	return []Signal{
		{
			MarketID:     m.ID,
			Outcome:      "YES",
			Confidence:   m.Probability,
			MarketProb:   m.Probability,
			Edge:         halfSpread,
			Strategy:     "marketmaking",
			Reason:       fmt.Sprintf("bid YES at %.3f (market %.3f, spread %.3f)", yesBuyProb, m.Probability, spread),
			IsLimitOrder: true,
			LimitProb:    yesBuyProb,
		},
		{
			MarketID:     m.ID,
			Outcome:      "NO",
			Confidence:   1 - m.Probability,
			MarketProb:   m.Probability,
			Edge:         halfSpread,
			Strategy:     "marketmaking",
			Reason:       fmt.Sprintf("ask NO at %.3f (market %.3f, spread %.3f)", noBuyProb, m.Probability, spread),
			IsLimitOrder: true,
			LimitProb:    noBuyProb,
		},
	}
}
