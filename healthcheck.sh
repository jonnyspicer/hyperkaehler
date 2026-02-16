#!/bin/bash
# Health check for hyperkaehler.
# Run via cron every 5 minutes to auto-restart if crashed.
# Crontab entry: */5 * * * * /opt/apps/hyperkaehler/healthcheck.sh

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PIDFILE="$SCRIPT_DIR/data/hyperkaehler.pid"
LOGFILE="$SCRIPT_DIR/logs/healthcheck.log"

mkdir -p "$(dirname "$LOGFILE")"

log() {
    echo "[$(date -Iseconds)] $1" >> "$LOGFILE"
}

# Check if process is running.
if [ -f "$PIDFILE" ]; then
    PID=$(cat "$PIDFILE")
    if kill -0 "$PID" 2>/dev/null; then
        # Running fine.
        exit 0
    fi
    log "PID $PID not running, cleaning up stale pidfile"
    rm -f "$PIDFILE"
fi

# Not running — restart it.
log "Hyperkaehler is not running, starting..."
cd "$SCRIPT_DIR"
nohup ./run.sh >> "$SCRIPT_DIR/logs/runner.log" 2>&1 &
log "Started with PID $!"
