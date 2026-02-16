#!/bin/bash
# Rotate and compress old logs.
# Run daily via cron: 0 3 * * * /opt/apps/hyperkaehler/rotate-logs.sh

LOG_DIR="/opt/apps/hyperkaehler/logs"
KEEP_DAYS=30

# Compress logs older than 1 day.
find "$LOG_DIR" -name "*.log" -mtime +1 -exec gzip -q {} \;

# Delete compressed logs older than KEEP_DAYS.
find "$LOG_DIR" -name "*.log.gz" -mtime +$KEEP_DAYS -delete

# Trim the SQLite database snapshots older than 90 days (saves ~450MB).
sqlite3 /opt/apps/hyperkaehler/data/hyperkaehler.db \
    "DELETE FROM market_snapshots WHERE snapshot_at < datetime('now', '-90 days');" 2>/dev/null || true
