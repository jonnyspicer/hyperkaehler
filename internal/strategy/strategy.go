package strategy

import (
	"context"
	"time"
)

// Signal represents a trading recommendation from a strategy.
type Signal struct {
	MarketID     string
	AnswerID     string  // empty for binary markets
	Outcome      string  // "YES" or "NO"
	Confidence   float64 // 0.0-1.0: bot's estimated true probability of this outcome resolving YES
	MarketProb   float64 // current market probability
	Edge         float64 // positive means bot thinks this is a good bet
	Strategy     string
	Reason       string
	IsLimitOrder bool
	LimitProb    float64
}

// Strategy is the interface all trading strategies must implement.
type Strategy interface {
	Name() string
	Evaluate(ctx context.Context, markets []MarketData) ([]Signal, error)
	Enabled() bool
}

// MarketData is a unified view of a market that strategies consume.
type MarketData struct {
	ID              string
	Question        string
	OutcomeType     string // "BINARY", "MULTIPLE_CHOICE", etc.
	Probability     float64
	Answers         []AnswerData
	Volume          float64
	Volume24Hours   float64
	TotalLiquidity  float64
	Pool            map[string]float64
	CloseTime       time.Time
	CreatedTime     time.Time
	IsResolved      bool
	Resolution      string
	CreatorID       string
	URL             string
	Mechanism       string
}

// AnswerData represents one answer in a multiple-choice market.
type AnswerData struct {
	ID          string
	Text        string
	Probability float64
	Resolution  string // "YES", "NO", "CANCEL", or "" for unresolved
}
