package config

import (
	"fmt"
	"os"
	"time"

	"github.com/BurntSushi/toml"
)

type Config struct {
	General  GeneralConfig  `toml:"general"`
	Schedule ScheduleConfig `toml:"schedule"`
	Risk     RiskConfig     `toml:"risk"`
	Strategy StrategyConfig `toml:"strategy"`
	Collector CollectorConfig `toml:"collector"`
}

type GeneralConfig struct {
	DBPath   string `toml:"db_path"`
	LogLevel string `toml:"log_level"`
}

type ScheduleConfig struct {
	ScanInterval         Duration `toml:"scan_interval"`
	SnapshotInterval     Duration `toml:"snapshot_interval"`
	PerformanceInterval  Duration `toml:"performance_interval"`
	OrderCleanupInterval Duration `toml:"order_cleanup_interval"`
}

type RiskConfig struct {
	KellyFraction       float64 `toml:"kelly_fraction"`
	MaxPositionPct      float64 `toml:"max_position_pct"`
	MaxMarketExposurePct float64 `toml:"max_market_exposure_pct"`
	MaxTotalExposure    float64 `toml:"max_total_exposure"`
	MaxDrawdownPct      float64 `toml:"max_drawdown_pct"`
	MinBetAmount        float64 `toml:"min_bet_amount"`
	MinEdge             float64 `toml:"min_edge"`
}

type StrategyConfig struct {
	Arbitrage    ArbitrageConfig    `toml:"arbitrage"`
	Mispricing   MispricingConfig   `toml:"mispricing"`
	TimeDecay    TimeDecayConfig    `toml:"timedecay"`
	MarketMaking MarketMakingConfig `toml:"marketmaking"`
}

type ArbitrageConfig struct {
	Enabled             bool    `toml:"enabled"`
	MinLiquidity        float64 `toml:"min_liquidity"`
	MinProbSumDeviation float64 `toml:"min_prob_sum_deviation"`
	MaxMarketsPerCycle  int     `toml:"max_markets_per_cycle"`
	MaxCloseDays        int     `toml:"max_close_days"`
}

type MispricingConfig struct {
	Enabled                bool    `toml:"enabled"`
	ExtremeThresholdHigh   float64 `toml:"extreme_threshold_high"`
	ExtremeThresholdLow    float64 `toml:"extreme_threshold_low"`
	MinMarketAgeDays       int     `toml:"min_market_age_days"`
	MinVolume              float64 `toml:"min_volume"`
	MeanReversionThreshold float64 `toml:"mean_reversion_threshold"`
}

type TimeDecayConfig struct {
	Enabled                 bool    `toml:"enabled"`
	MinTimeElapsedFraction  float64 `toml:"min_time_elapsed_fraction"`
	MinEdge                 float64 `toml:"min_edge"`
	MinVolume               float64 `toml:"min_volume"`
}

type MarketMakingConfig struct {
	Enabled                  bool    `toml:"enabled"`
	BaseSpread               float64 `toml:"base_spread"`
	MinLiquidity             float64 `toml:"min_liquidity"`
	MinVolume24h             float64 `toml:"min_volume_24h"`
	MaxLimitOrderCapitalPct  float64 `toml:"max_limit_order_capital_pct"`
}

type CollectorConfig struct {
	MaxMarketsPerScan int     `toml:"max_markets_per_scan"`
	MinLiquidity      float64 `toml:"min_liquidity"`
}

// Duration wraps time.Duration for TOML unmarshaling.
type Duration struct {
	time.Duration
}

func (d *Duration) UnmarshalText(text []byte) error {
	var err error
	d.Duration, err = time.ParseDuration(string(text))
	return err
}

func Load(path string) (*Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	return cfg, nil
}

func DefaultConfig() *Config {
	return &Config{
		General: GeneralConfig{
			DBPath:   "./data/hyperkaehler.db",
			LogLevel: "info",
		},
		Schedule: ScheduleConfig{
			ScanInterval:         Duration{5 * time.Minute},
			SnapshotInterval:     Duration{15 * time.Minute},
			PerformanceInterval:  Duration{1 * time.Hour},
			OrderCleanupInterval: Duration{10 * time.Minute},
		},
		Risk: RiskConfig{
			KellyFraction:        0.25,
			MaxPositionPct:       0.05,
			MaxMarketExposurePct: 0.10,
			MaxTotalExposure:     0.50,
			MaxDrawdownPct:       0.20,
			MinBetAmount:         1.0,
			MinEdge:              0.05,
		},
		Strategy: StrategyConfig{
			Arbitrage: ArbitrageConfig{
				Enabled:             true,
				MinLiquidity:        50.0,
				MinProbSumDeviation: 0.10,
				MaxMarketsPerCycle:  20,
			},
		},
		Collector: CollectorConfig{
			MaxMarketsPerScan: 500,
			MinLiquidity:      20.0,
		},
	}
}
