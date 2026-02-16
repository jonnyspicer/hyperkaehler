package collector

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"

	"hyperkaehler/internal/config"
	"hyperkaehler/internal/market"
	"hyperkaehler/internal/strategy"
)

// Collector periodically snapshots market data for backtesting.
type Collector struct {
	scanner *market.Scanner
	db      *sql.DB
	cfg     config.CollectorConfig
}

func NewCollector(scanner *market.Scanner, db *sql.DB, cfg config.CollectorConfig) *Collector {
	return &Collector{scanner: scanner, db: db, cfg: cfg}
}

// Collect fetches markets and stores snapshots.
func (c *Collector) Collect() error {
	markets, err := c.scanner.ScanAll(int64(c.cfg.MaxMarketsPerScan))
	if err != nil {
		return fmt.Errorf("scanning markets: %w", err)
	}

	inserted, snapshotted := 0, 0
	for _, m := range markets {
		if m.TotalLiquidity < c.cfg.MinLiquidity {
			continue
		}

		// Upsert market record.
		if err := c.upsertMarket(m); err != nil {
			slog.Warn("failed to upsert market", "id", m.ID, "error", err)
			continue
		}
		inserted++

		// Take snapshot.
		if err := c.snapshot(m); err != nil {
			slog.Warn("failed to snapshot market", "id", m.ID, "error", err)
			continue
		}
		snapshotted++
	}

	slog.Info("collection complete", "markets_upserted", inserted, "snapshots_taken", snapshotted)
	return nil
}

func (c *Collector) upsertMarket(m strategy.MarketData) error {
	_, err := c.db.Exec(`
		INSERT INTO markets (id, question, outcome_type, mechanism, creator_id, created_time, close_time, url, is_resolved, resolution)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			is_resolved = excluded.is_resolved,
			resolution = excluded.resolution,
			last_updated_at = datetime('now')`,
		m.ID, m.Question, m.OutcomeType, m.Mechanism, m.CreatorID,
		m.CreatedTime.UnixMilli(), m.CloseTime.UnixMilli(), m.URL,
		boolToInt(m.IsResolved), m.Resolution,
	)
	return err
}

func (c *Collector) snapshot(m strategy.MarketData) error {
	var answerProbs *string
	if len(m.Answers) > 0 {
		probs := make(map[string]float64, len(m.Answers))
		for _, a := range m.Answers {
			probs[a.ID] = a.Probability
		}
		data, err := json.Marshal(probs)
		if err == nil {
			s := string(data)
			answerProbs = &s
		}
	}

	var poolYes, poolNo *float64
	if v, ok := m.Pool["YES"]; ok {
		poolYes = &v
	}
	if v, ok := m.Pool["NO"]; ok {
		poolNo = &v
	}

	_, err := c.db.Exec(`
		INSERT INTO market_snapshots (market_id, probability, answer_probs, volume, volume_24h, total_liquidity, pool_yes, pool_no)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.Probability, answerProbs, m.Volume, m.Volume24Hours, m.TotalLiquidity, poolYes, poolNo,
	)
	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
