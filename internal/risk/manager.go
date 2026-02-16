package risk

import (
	"log/slog"
	"math"

	"hyperkaehler/internal/config"
	"hyperkaehler/internal/strategy"
)

// Manager determines position sizing and enforces portfolio limits.
type Manager struct {
	cfg             config.RiskConfig
	portfolio       *Portfolio
	totalExposure   float64
	marketExposure  map[string]float64 // marketID -> total mana wagered
	peakBalance     float64
}

func NewManager(cfg config.RiskConfig, portfolio *Portfolio) *Manager {
	return &Manager{
		cfg:            cfg,
		portfolio:      portfolio,
		marketExposure: make(map[string]float64),
	}
}

// SizedSignal is a Signal that has been approved and sized by the risk manager.
type SizedSignal struct {
	Signal strategy.Signal
	Amount float64
}

// CanTrade returns false if the portfolio is in drawdown or at exposure limits.
func (m *Manager) CanTrade() bool {
	if m.portfolio.TotalValue <= 0 {
		slog.Warn("cannot trade: no balance")
		return false
	}

	// Update peak balance.
	if m.portfolio.TotalValue > m.peakBalance {
		m.peakBalance = m.portfolio.TotalValue
	}

	// Check drawdown.
	if m.peakBalance > 0 {
		drawdown := (m.peakBalance - m.portfolio.TotalValue) / m.peakBalance
		if drawdown >= m.cfg.MaxDrawdownPct {
			slog.Warn("trading halted: max drawdown exceeded",
				"drawdown", drawdown,
				"limit", m.cfg.MaxDrawdownPct,
			)
			return false
		}
	}

	// Check total exposure.
	exposurePct := m.totalExposure / m.portfolio.TotalValue
	if exposurePct >= m.cfg.MaxTotalExposure {
		slog.Warn("trading halted: max exposure reached",
			"exposure_pct", exposurePct,
			"limit", m.cfg.MaxTotalExposure,
		)
		return false
	}

	return true
}

// SizeSignals takes raw signals and returns sized, approved signals.
func (m *Manager) SizeSignals(signals []strategy.Signal) []SizedSignal {
	if !m.CanTrade() {
		return nil
	}

	// Track per-market exposure within this sizing pass.
	cycleMarketExposure := make(map[string]float64)

	sized := make([]SizedSignal, 0, len(signals))
	for _, sig := range signals {
		amount := m.sizePosition(sig)

		// Apply per-market exposure cap.
		if m.cfg.MaxMarketExposurePct > 0 {
			maxMarketExposure := m.cfg.MaxMarketExposurePct * m.portfolio.TotalValue
			existingExposure := m.marketExposure[sig.MarketID] + cycleMarketExposure[sig.MarketID]
			remaining := maxMarketExposure - existingExposure
			if remaining <= 0 {
				slog.Info("signal rejected: market exposure cap reached",
					"market", sig.MarketID,
					"existing_exposure", existingExposure,
					"cap", maxMarketExposure,
				)
				continue
			}
			if amount > remaining {
				amount = math.Floor(remaining)
			}
		}

		if amount >= m.cfg.MinBetAmount {
			cycleMarketExposure[sig.MarketID] += amount
			sized = append(sized, SizedSignal{Signal: sig, Amount: amount})
		} else {
			slog.Info("signal rejected by risk manager",
				"strategy", sig.Strategy,
				"market", sig.MarketID,
				"edge", sig.Edge,
				"confidence", sig.Confidence,
				"market_prob", sig.MarketProb,
				"outcome", sig.Outcome,
				"computed_amount", amount,
			)
		}
	}
	return sized
}

func (m *Manager) sizePosition(sig strategy.Signal) float64 {
	if sig.Edge < m.cfg.MinEdge {
		return 0
	}

	// Standard Kelly criterion: f* = (bp - q) / b
	// where b = net odds (payout ratio), p = probability of winning, q = 1 - p.
	//
	// For a YES bet at market price m: b = (1/m) - 1, p = our confidence in YES.
	// For a NO bet at market price m:  b = (1/(1-m)) - 1, p = our confidence in NO.
	var b float64 // net odds received on the bet
	p := sig.Confidence
	if sig.Outcome == "YES" {
		if sig.MarketProb <= 0 || sig.MarketProb >= 1 {
			return 0
		}
		b = (1.0 / sig.MarketProb) - 1.0
	} else {
		noPrice := 1.0 - sig.MarketProb
		if noPrice <= 0 || noPrice >= 1 {
			return 0
		}
		b = (1.0 / noPrice) - 1.0
	}

	if b <= 0 {
		return 0
	}

	kellyFraction := (b*p - (1 - p)) / b
	if kellyFraction <= 0 {
		return 0
	}

	// Apply fractional Kelly.
	fraction := kellyFraction * m.cfg.KellyFraction

	amount := fraction * m.portfolio.Balance

	// Cap at max position size.
	maxPosition := m.cfg.MaxPositionPct * m.portfolio.TotalValue
	if amount > maxPosition {
		amount = maxPosition
	}

	// Cap at remaining exposure budget.
	remainingExposure := (m.cfg.MaxTotalExposure * m.portfolio.TotalValue) - m.totalExposure
	if amount > remainingExposure {
		amount = remainingExposure
	}

	// Don't bet more than we have.
	if amount > m.portfolio.Balance {
		amount = m.portfolio.Balance
	}

	// Round down to nearest integer (Manifold uses integer mana).
	amount = math.Floor(amount)

	if amount < m.cfg.MinBetAmount {
		return 0
	}

	return amount
}

// RecordTrade updates internal exposure tracking after a trade is executed.
func (m *Manager) RecordTrade(marketID string, amount float64) {
	m.totalExposure += amount
	m.marketExposure[marketID] += amount
}

// Refresh updates the manager's state from the portfolio.
func (m *Manager) Refresh() {
	if m.portfolio.TotalValue > m.peakBalance {
		m.peakBalance = m.portfolio.TotalValue
	}
	// Use the portfolio's investment value as total exposure — this is the actual
	// amount of capital deployed in positions, fetched from the Manifold API.
	m.totalExposure = m.portfolio.InvestmentValue
}

// SetExposure sets the current total exposure (from active bets/orders).
func (m *Manager) SetExposure(exposure float64) {
	m.totalExposure = exposure
}

// SetMarketExposure sets the per-market exposure map (loaded from DB).
func (m *Manager) SetMarketExposure(exposure map[string]float64) {
	m.marketExposure = exposure
}
