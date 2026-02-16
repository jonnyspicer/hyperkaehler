package db

const schemaSQL = `
CREATE TABLE IF NOT EXISTS schema_version (
    version INTEGER PRIMARY KEY,
    applied_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS markets (
    id TEXT PRIMARY KEY,
    question TEXT NOT NULL,
    outcome_type TEXT NOT NULL,
    mechanism TEXT NOT NULL,
    creator_id TEXT NOT NULL,
    created_time INTEGER NOT NULL,
    close_time INTEGER NOT NULL,
    url TEXT NOT NULL,
    is_resolved INTEGER NOT NULL DEFAULT 0,
    resolution TEXT,
    first_seen_at TEXT NOT NULL DEFAULT (datetime('now')),
    last_updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS market_snapshots (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    market_id TEXT NOT NULL REFERENCES markets(id),
    probability REAL,
    answer_probs TEXT,
    volume REAL NOT NULL,
    volume_24h REAL NOT NULL,
    total_liquidity REAL NOT NULL,
    pool_yes REAL,
    pool_no REAL,
    snapshot_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_snapshots_market_time ON market_snapshots(market_id, snapshot_at);

CREATE TABLE IF NOT EXISTS bot_bets (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    market_id TEXT NOT NULL REFERENCES markets(id),
    strategy TEXT NOT NULL,
    outcome TEXT NOT NULL,
    amount REAL NOT NULL,
    limit_prob REAL,
    expected_prob REAL NOT NULL,
    market_prob_at_bet REAL NOT NULL,
    kelly_fraction REAL NOT NULL,
    placed_at TEXT NOT NULL DEFAULT (datetime('now')),
    resolved INTEGER NOT NULL DEFAULT 0,
    resolution TEXT,
    pnl REAL,
    resolved_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_bets_strategy ON bot_bets(strategy);
CREATE INDEX IF NOT EXISTS idx_bets_market ON bot_bets(market_id);

CREATE TABLE IF NOT EXISTS bankroll_snapshots (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    balance REAL NOT NULL,
    investment_value REAL NOT NULL,
    total_value REAL NOT NULL,
    snapshot_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS active_orders (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    bet_id INTEGER REFERENCES bot_bets(id),
    manifold_bet_id TEXT,
    market_id TEXT NOT NULL REFERENCES markets(id),
    strategy TEXT NOT NULL,
    outcome TEXT NOT NULL,
    amount REAL NOT NULL,
    limit_prob REAL NOT NULL,
    placed_at TEXT NOT NULL DEFAULT (datetime('now')),
    cancelled_at TEXT,
    is_active INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS idx_active_orders_active ON active_orders(is_active) WHERE is_active = 1;
`
