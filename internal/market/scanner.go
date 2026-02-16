package market

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jonnyspicer/mango"

	"hyperkaehler/internal/strategy"
)

// Scanner fetches markets from the Manifold API and converts them to MarketData.
type Scanner struct {
	client *mango.Client
}

func NewScanner(client *mango.Client) *Scanner {
	return &Scanner{client: client}
}

// ScanBinary fetches open binary markets sorted by liquidity.
func (s *Scanner) ScanBinary(limit int64) ([]strategy.MarketData, error) {
	markets, err := s.client.SearchMarkets(mango.SearchMarketsRequest{
		Filter:       "open",
		ContractType: "BINARY",
		Sort:         "liquidity",
		Limit:        limit,
	})
	if err != nil {
		return nil, fmt.Errorf("searching binary markets: %w", err)
	}
	if markets == nil {
		return nil, nil
	}

	result := make([]strategy.MarketData, 0, len(*markets))
	for _, m := range *markets {
		result = append(result, fullMarketToData(m))
	}
	slog.Info("scanned binary markets", "count", len(result))
	return result, nil
}

// ScanMultipleChoice fetches open multiple-choice markets sorted by liquidity,
// then enriches them with answer probabilities via the batch probability API.
func (s *Scanner) ScanMultipleChoice(limit int64) ([]strategy.MarketData, error) {
	markets, err := s.client.SearchMarkets(mango.SearchMarketsRequest{
		Filter:       "open",
		ContractType: "MULTIPLE_CHOICE",
		Sort:         "liquidity",
		Limit:        limit,
	})
	if err != nil {
		return nil, fmt.Errorf("searching multi-choice markets: %w", err)
	}
	if markets == nil {
		return nil, nil
	}

	result := make([]strategy.MarketData, 0, len(*markets))
	for _, m := range *markets {
		result = append(result, fullMarketToData(m))
	}

	// The search API doesn't return answer probabilities for multi-choice markets.
	// Use the batch probability API to fill them in.
	s.enrichWithProbs(result)

	// Fetch full market details to get per-answer resolution status.
	// The batch probability API doesn't include this.
	s.enrichWithResolution(result)

	slog.Info("scanned multi-choice markets", "count", len(result))
	return result, nil
}

// enrichWithProbs fetches answer probabilities via GetMarketProbs (100 at a time)
// and updates the Answers field on each market.
func (s *Scanner) enrichWithProbs(markets []strategy.MarketData) {
	// Collect market IDs in batches of 100.
	for i := 0; i < len(markets); i += 100 {
		end := i + 100
		if end > len(markets) {
			end = len(markets)
		}
		batch := markets[i:end]

		ids := make([]string, len(batch))
		for j, m := range batch {
			ids[j] = m.ID
		}

		probs, err := s.client.GetMarketProbs(ids)
		if err != nil {
			slog.Warn("failed to get batch probabilities", "error", err)
			continue
		}
		if probs == nil {
			continue
		}

		for j := range batch {
			mp, ok := (*probs)[batch[j].ID]
			if !ok {
				continue
			}

			// For multi-choice markets, AnswerProbs maps answer ID -> probability.
			if len(mp.AnswerProbs) > 0 {
				// Rebuild answers with probabilities.
				// If we already have answer metadata (text), merge; otherwise create from IDs.
				if len(batch[j].Answers) > 0 {
					for k := range batch[j].Answers {
						if p, found := mp.AnswerProbs[batch[j].Answers[k].ID]; found {
							batch[j].Answers[k].Probability = p
						}
					}
				} else {
					answers := make([]strategy.AnswerData, 0, len(mp.AnswerProbs))
					for id, p := range mp.AnswerProbs {
						answers = append(answers, strategy.AnswerData{
							ID:          id,
							Probability: p,
						})
					}
					batch[j].Answers = answers
				}
			}

			// Also update binary probability if present.
			if mp.Prob > 0 {
				batch[j].Probability = mp.Prob
			}
		}
	}
}

// enrichWithResolution fetches full market details via GetMarketByID to get
// per-answer resolution status and text. Only fetches markets that have 2+
// answers with non-trivial probabilities (likely arbitrage candidates).
// Uses concurrent fetching with up to 10 parallel requests.
func (s *Scanner) enrichWithResolution(markets []strategy.MarketData) {
	// Identify candidate indices.
	var candidates []int
	for i := range markets {
		if markets[i].TotalLiquidity < 50 {
			continue
		}
		activeAnswers := 0
		for _, a := range markets[i].Answers {
			if a.Probability > 0.001 && a.Probability < 0.999 {
				activeAnswers++
			}
		}
		if activeAnswers < 2 {
			continue
		}
		candidates = append(candidates, i)
	}

	if len(candidates) == 0 {
		return
	}

	type fetchResult struct {
		idx     int
		answers []mango.Answer
	}

	results := make(chan fetchResult, len(candidates))
	sem := make(chan struct{}, 10) // Max 10 concurrent requests.
	var wg sync.WaitGroup

	for _, idx := range candidates {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}        // Acquire.
			defer func() { <-sem }() // Release.

			market, err := s.client.GetMarketByID(markets[i].ID)
			if err != nil {
				slog.Warn("failed to fetch answer resolutions",
					"market", markets[i].ID, "error", err)
				return
			}
			if market == nil {
				return
			}
			results <- fetchResult{idx: i, answers: market.Answers}
		}(idx)
	}

	// Close results channel after all goroutines complete.
	go func() {
		wg.Wait()
		close(results)
	}()

	fetched := 0
	for r := range results {
		fetched++
		answerMap := make(map[string]mango.Answer, len(r.answers))
		for _, a := range r.answers {
			answerMap[a.Id] = a
		}
		for j := range markets[r.idx].Answers {
			id := markets[r.idx].Answers[j].ID
			if a, ok := answerMap[id]; ok {
				markets[r.idx].Answers[j].Resolution = a.Resolution
				if markets[r.idx].Answers[j].Text == "" {
					markets[r.idx].Answers[j].Text = a.Text
				}
			}
		}
	}

	if fetched > 0 {
		slog.Info("enriched markets with answer resolutions", "fetched", fetched)
	}
}

// ScanAll fetches all open markets (binary + multi-choice) for the collector.
func (s *Scanner) ScanAll(limit int64) ([]strategy.MarketData, error) {
	markets, err := s.client.SearchMarkets(mango.SearchMarketsRequest{
		Filter: "open",
		Sort:   "liquidity",
		Limit:  limit,
	})
	if err != nil {
		return nil, fmt.Errorf("searching all markets: %w", err)
	}
	if markets == nil {
		return nil, nil
	}

	result := make([]strategy.MarketData, 0, len(*markets))
	for _, m := range *markets {
		result = append(result, fullMarketToData(m))
	}
	slog.Info("scanned all markets", "count", len(result))
	return result, nil
}

// GetFullMarket fetches a single market with full details (including answers).
func (s *Scanner) GetFullMarket(id string) (*strategy.MarketData, error) {
	m, err := s.client.GetMarketByID(id)
	if err != nil {
		return nil, fmt.Errorf("getting market %s: %w", id, err)
	}
	if m == nil {
		return nil, nil
	}
	md := fullMarketToData(*m)
	return &md, nil
}

func fullMarketToData(m mango.FullMarket) strategy.MarketData {
	answers := make([]strategy.AnswerData, 0, len(m.Answers))
	for _, a := range m.Answers {
		answers = append(answers, strategy.AnswerData{
			ID:          a.Id,
			Text:        a.Text,
			Probability: a.Probability,
			Resolution:  a.Resolution,
		})
	}

	pool := make(map[string]float64)
	if m.Pool != nil {
		for k, v := range m.Pool {
			pool[k] = v
		}
	}

	return strategy.MarketData{
		ID:             m.Id,
		Question:       m.Question,
		OutcomeType:    string(m.OutcomeType),
		Probability:    m.Probability,
		Answers:        answers,
		Volume:         m.Volume,
		Volume24Hours:  m.Volume24Hours,
		TotalLiquidity: m.TotalLiquidity,
		CloseTime:      time.UnixMilli(m.CloseTime),
		CreatedTime:    time.UnixMilli(m.CreatedTime),
		IsResolved:     m.IsResolved,
		Resolution:     m.Resolution,
		CreatorID:      m.CreatorId,
		URL:            m.Url,
		Mechanism:      m.Mechanism,
	}
}
