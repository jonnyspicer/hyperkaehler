# Hyperkaehler

A multi-strategy trading bot for [Manifold Markets](https://manifold.markets). It scans for opportunities across multiple-choice and binary markets, sizes positions using fractional Kelly criterion, and manages risk with per-market exposure caps, drawdown circuit breakers, and total exposure limits.

Built in Go using [mango](https://github.com/jonnyspicer/mango) as the Manifold Markets API client.

## Strategies

- **Arbitrage** — Exploits probability mispricing in multiple-choice markets where answer probabilities don't sum to 1. Bets against overpriced answers (or for underpriced ones) using limit orders. Filters out resolved answers and markets closing more than 90 days out.
- **Time Decay** — Targets "Will X happen by DATE?" markets where the event hasn't occurred and time is running out. As the deadline approaches, the probability should decay toward NO if nothing has changed.
- **Mispricing** — Identifies extreme probability markets (>95% or <5%) that are likely correct but not extreme enough, and detects mean reversion opportunities after sudden moves by poorly-calibrated users.
- **Market Making** — Places limit orders on both sides of active binary markets to capture the spread. Low priority at small bankroll sizes.

Each strategy can be independently enabled/disabled via `config.toml`.

## Risk Management

- Fractional Kelly sizing (default 1/4 Kelly)
- Max 5% of bankroll per position
- Max 10% exposure per market
- Max 50% total capital deployed
- 20% drawdown circuit breaker halts all trading
- Minimum edge threshold (default 3%)
- Permanent blacklisting of bets on resolved/closed answers

## Project Structure

```
cmd/hyperkaehler/main.go       Entry point, config loading, wiring
internal/
  config/                      TOML configuration
  db/                          SQLite setup and migrations
  market/                      Market scanning and API enrichment
  strategy/                    Strategy interface and implementations
  risk/                        Kelly sizing, exposure tracking, drawdown
  execution/                   Bet placement and DB recording
  collector/                   Periodic market snapshots for backtesting
  performance/                 ROI, Sharpe ratio, win rate tracking
  scheduler/                   Main loop orchestrating scan -> evaluate -> size -> execute
  backtest/                    Historical replay of strategies against snapshots
```

## Setup

### Prerequisites

- Go 1.21+
- A Manifold Markets API key

### Configuration

1. Create a `.env` file with your API key:
   ```
   MANIFOLD_API_KEY=your-key-here
   ```

2. Edit `config.toml` to enable/disable strategies and tune parameters.

### Build and Run

```sh
go build -o hyperkaehler ./cmd/hyperkaehler
./hyperkaehler
```

Or use the runner script with auto-restart and daily log rotation:

```sh
nohup bash run.sh &
```

### Docker

```sh
docker build -t hyperkaehler .
docker run --env-file .env hyperkaehler
```

## Data

Uses SQLite (pure Go, no CGO) with WAL mode. The database stores:

- **markets** — All markets the bot has seen
- **market_snapshots** — Periodic probability/volume snapshots for backtesting
- **bot_bets** — Every bet placed, with strategy, sizing, and expected probabilities
- **bankroll_snapshots** — Balance over time for drawdown calculation
- **active_orders** — Outstanding limit orders

## License

GPL-3.0