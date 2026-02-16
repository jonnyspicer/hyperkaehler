package risk

import (
	"fmt"
	"log/slog"

	"github.com/jonnyspicer/mango"
)

// Portfolio tracks the bot's current balance and investment state.
type Portfolio struct {
	client  *mango.Client
	Balance float64
	InvestmentValue float64
	TotalValue      float64
	UserID          string
}

func NewPortfolio(client *mango.Client) *Portfolio {
	return &Portfolio{client: client}
}

// Refresh fetches the latest balance and portfolio data from the API.
func (p *Portfolio) Refresh() error {
	user, err := p.client.GetAuthenticatedUser()
	if err != nil {
		return fmt.Errorf("getting authenticated user: %w", err)
	}
	if user == nil {
		return fmt.Errorf("authenticated user returned nil")
	}

	p.UserID = user.Id
	p.Balance = user.Balance

	portfolio, err := p.client.GetUserPortfolio(user.Id)
	if err != nil {
		// Non-fatal: we at least have the balance.
		slog.Warn("failed to get portfolio", "error", err)
		p.InvestmentValue = 0
		p.TotalValue = p.Balance
		return nil
	}
	if portfolio != nil {
		p.InvestmentValue = portfolio.InvestmentValue
		p.TotalValue = p.Balance + p.InvestmentValue
	}

	slog.Info("portfolio refreshed",
		"balance", p.Balance,
		"invested", p.InvestmentValue,
		"total", p.TotalValue,
	)
	return nil
}
