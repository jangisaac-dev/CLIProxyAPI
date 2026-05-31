#!/bin/bash
DEV_DIR="$(cd "$(dirname "$0")" && pwd)"
PID_FILE="$DEV_DIR/cliproxy-8317.pid"
CONFIG_FILE="$DEV_DIR/config-8317.yaml"
BINARY="$DEV_DIR/cliproxyapi-dev"

echo "Stopping dev instance on 8317..."

if [ -f "$PID_FILE" ]; then
  PID=$(cat "$PID_FILE")
  if ps -p $PID > /dev/null 2>&1; then
    kill $PID
    echo "Sent SIGTERM to PID $PID"
    sleep 1
  fi
  rm -f "$PID_FILE"
fi

pkill -f "$BINARY --config $CONFIG_FILE" 2>/dev/null || true

echo "Stopped."
