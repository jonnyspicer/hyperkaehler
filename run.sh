#!/bin/bash
# Hyperkaehler runner with auto-restart and logging.
# Usage: nohup ./run.sh &
# Or use with screen/tmux for interactive monitoring.

set -eu

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

LOG_DIR="$SCRIPT_DIR/logs"
mkdir -p "$LOG_DIR" "$SCRIPT_DIR/data"

PIDFILE="$SCRIPT_DIR/data/hyperkaehler.pid"

cleanup() {
    echo "[$(date -Iseconds)] Stopping hyperkaehler..."
    if [ -f "$PIDFILE" ]; then
        kill "$(cat "$PIDFILE")" 2>/dev/null || true
        rm -f "$PIDFILE"
    fi
    exit 0
}
trap cleanup SIGINT SIGTERM

echo "[$(date -Iseconds)] Hyperkaehler runner starting"
echo "$$" > "$PIDFILE"

while true; do
    LOGFILE="$LOG_DIR/hyperkaehler-$(date +%Y-%m-%d).log"

    echo "[$(date -Iseconds)] Starting hyperkaehler (logging to $LOGFILE)"
    ./hyperkaehler >> "$LOGFILE" 2>&1 &
    BOT_PID=$!
    echo "$BOT_PID" > "$PIDFILE"

    wait "$BOT_PID" || true
    EXIT_CODE=$?

    echo "[$(date -Iseconds)] Hyperkaehler exited with code $EXIT_CODE"

    if [ $EXIT_CODE -eq 0 ]; then
        echo "[$(date -Iseconds)] Clean exit, not restarting"
        break
    fi

    echo "[$(date -Iseconds)] Restarting in 30 seconds..."
    sleep 30
done
