package execution

import (
	"database/sql"
	"fmt"
	"log/slog"
	"math"
	"strings"

	"github.com/jonnyspicer/mango"

	"hyperkaehler/internal/risk"
)

// Executor translates sized signals into API calls and records results.
type Executor struct {
	client     *mango.Client
	db         *sql.DB
	failedBets map[string]int // "marketID:answerID" -> consecutive failure count
}

func NewExecutor(client *mango.Client, db *sql.DB) *Executor {
	return &Executor{
		client:     client,
		db:         db,
		failedBets: make(map[string]int),
	}
}

// ExecutionResult records what happened when a signal was executed.
type ExecutionResult struct {
	Signal  risk.SizedSignal
	Success bool
	Error   error
}

// Execute places bets for all sized signals and records them in the database.
func (e *Executor) Execute(signals []risk.SizedSignal) []ExecutionResult {
	results := make([]ExecutionResult, 0, len(signals))

	for _, sig := range signals {
		result := e.executeSingle(sig)
		results = append(results, result)
	}

	return results
}

func (e *Executor) executeSingle(sig risk.SizedSignal) ExecutionResult {
	// Skip bets that have failed 3+ times consecutively (e.g., resolved answers).
	key := sig.Signal.MarketID + ":" + sig.Signal.AnswerID
	if e.failedBets[key] >= 3 {
		slog.Info("skipping repeatedly failed bet", "market", sig.Signal.MarketID, "answer", sig.Signal.AnswerID)
		return ExecutionResult{Signal: sig, Success: false, Error: fmt.Errorf("skipped: failed %d times", e.failedBets[key])}
	}

	slog.Info("placing bet",
		"market", sig.Signal.MarketID,
		"answer", sig.Signal.AnswerID,
		"outcome", sig.Signal.Outcome,
		"amount", sig.Amount,
		"strategy", sig.Signal.Strategy,
		"edge", sig.Signal.Edge,
		"reason", sig.Signal.Reason,
	)

	req := mango.PostBetRequest{
		Amount:     sig.Amount,
		ContractId: sig.Signal.MarketID,
		Outcome:    sig.Signal.Outcome,
		AnswerId:   sig.Signal.AnswerID,
	}
	if sig.Signal.IsLimitOrder {
		prob := math.Round(sig.Signal.LimitProb*100) / 100
		if prob >= 0.01 && prob <= 0.99 {
			req.LimitProb = &prob
		}
	}

	_, err := e.client.PostBet(req)

	if err != nil {
		errStr := err.Error()
		// Immediately blacklist permanently-failing bets (resolved answers, closed markets).
		if strings.Contains(errStr, "resolved") || strings.Contains(errStr, "status 403") || strings.Contains(errStr, "status 404") {
			e.failedBets[key] = 100 // Permanent blacklist.
			slog.Warn("bet permanently blacklisted",
				"market", sig.Signal.MarketID,
				"answer", sig.Signal.AnswerID,
				"error", err,
			)
		} else {
			e.failedBets[key]++
		}
		slog.Error("bet failed",
			"market", sig.Signal.MarketID,
			"error", err,
			"consecutive_failures", e.failedBets[key],
		)
		return ExecutionResult{Signal: sig, Success: false, Error: err}
	}
	delete(e.failedBets, key) // Reset on success.

	// Record in database.
	if dbErr := e.recordBet(sig); dbErr != nil {
		slog.Error("failed to record bet in db", "error", dbErr)
	}

	slog.Info("bet placed successfully",
		"market", sig.Signal.MarketID,
		"answer", sig.Signal.AnswerID,
		"outcome", sig.Signal.Outcome,
		"amount", sig.Amount,
		"strategy", sig.Signal.Strategy,
	)

	return ExecutionResult{Signal: sig, Success: true}
}

func (e *Executor) recordBet(sig risk.SizedSignal) error {
	var limitProb *float64
	if sig.Signal.IsLimitOrder {
		lp := sig.Signal.LimitProb
		limitProb = &lp
	}

	_, err := e.db.Exec(`
		INSERT INTO bot_bets (market_id, strategy, outcome, amount, limit_prob, expected_prob, market_prob_at_bet, kelly_fraction)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		sig.Signal.MarketID,
		sig.Signal.Strategy,
		sig.Signal.Outcome,
		sig.Amount,
		limitProb,
		sig.Signal.Confidence,
		sig.Signal.MarketProb,
		0,
	)
	if err != nil {
		return fmt.Errorf("inserting bot_bet: %w", err)
	}
	return nil
}

// EnsureMarketExists inserts a market into the DB if it doesn't already exist.
func (e *Executor) EnsureMarketExists(id, question, outcomeType, mechanism, creatorID string, createdTime, closeTime int64, url string) error {
	_, err := e.db.Exec(`
		INSERT OR IGNORE INTO markets (id, question, outcome_type, mechanism, creator_id, created_time, close_time, url)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, question, outcomeType, mechanism, creatorID, createdTime, closeTime, url,
	)
	return err
}
