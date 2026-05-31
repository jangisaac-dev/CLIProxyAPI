#!/bin/bash
set -e

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
DEV_DIR="$REPO_ROOT/scripts/dev-8317"
MAIN_CONF="/opt/homebrew/etc/cliproxyapi.conf"
CONFIG_FILE="$DEV_DIR/config-8317.yaml"
PID_FILE="$DEV_DIR/cliproxy-8317.pid"
LOG_FILE="$DEV_DIR/cliproxy-8317.log"
BINARY="$DEV_DIR/cliproxyapi-dev"
DEV_PORT="${CLIPROXY_DEV_PORT:-8317}"
DEV_MANAGEMENT_KEY="${CLIPROXY_DEV_MANAGEMENT_KEY:-cliproxy-dev-local}"
DEV_PROXY_HOST="${CLIPROXY_DEV_PROXY_HOST:-172.20.10.1}"
DEV_PROXY_PORT="${CLIPROXY_DEV_PROXY_PORT:-3128}"

mkdir -p "$DEV_DIR"

echo "=== Preparing config for ${DEV_PORT} ==="

if [ -f "$MAIN_CONF" ]; then
  echo "Main brew config found → full sync"
  cp "$MAIN_CONF" "$CONFIG_FILE"
  sed -i '' "s/^port: .*/port: ${DEV_PORT}/" "$CONFIG_FILE"
else
  echo "WARNING: Main brew config not found (brew uninstalled?)"
  echo "Writing minimal dev config"
  cat > "$CONFIG_FILE" << MINIMAL
host: 127.0.0.1
port: ${DEV_PORT}
log-level: info
remote-management:
  secret-key: "CHANGE-ME"
  allow-remote: false
  disable-control-panel: false
auth-dir: /Users/isaacjang/.cli-proxy-api
MINIMAL
fi

FILTERED_CONFIG="$CONFIG_FILE.tmp"
awk '
  /^[^[:space:]#][^:]*:/ {
    if ($0 ~ /^(logging-to-file|proxy-url|proxy-settings):/) {
      skip = 1
      next
    }
    skip = 0
  }
  !skip { print }
' "$CONFIG_FILE" > "$FILTERED_CONFIG"
mv "$FILTERED_CONFIG" "$CONFIG_FILE"

# Dev starts in direct mode so CLIProxyAPI remains usable when the proxy server
# is not reachable. /usr/local/sbin/proxy-web-on.sh can toggle this on at runtime.
cat >> "$CONFIG_FILE" << PROXYEOF

# === CLIProxyAPI dev runtime overlay ===
logging-to-file: false
proxy-url: "direct"

proxy-settings:
  enabled: false
  protocol: "http"
  host: "${DEV_PROXY_HOST}"
  port: ${DEV_PROXY_PORT}
PROXYEOF

echo "Config ready: $CONFIG_FILE"

echo ""
echo "=== Building from current branch ==="
cd "$REPO_ROOT"
export GOCACHE="${GOCACHE:-$REPO_ROOT/.gocache}"
export GOMODCACHE="${GOMODCACHE:-$REPO_ROOT/.gomodcache}"
mkdir -p "$GOCACHE"
mkdir -p "$GOMODCACHE"
go build -o "$BINARY" ./cmd/server

echo ""
echo "=== Starting ${DEV_PORT} ==="
[ -f "$PID_FILE" ] && kill $(cat "$PID_FILE") 2>/dev/null || true
rm -f "$PID_FILE"

export MANAGEMENT_STATIC_PATH="$DEV_DIR/panel/management.html"
mkdir -p "$DEV_DIR/panel"

MANAGEMENT_PASSWORD="$DEV_MANAGEMENT_KEY" nohup "$BINARY" \
  --config "$CONFIG_FILE" \
  --no-browser \
  > "$LOG_FILE" 2>&1 &

echo $! > "$PID_FILE"

sleep 4

if curl -s --max-time 5 "http://127.0.0.1:${DEV_PORT}/management.html" -o /dev/null -w "%{http_code}" | grep -q "200"; then
  echo "✅ ${DEV_PORT} started: http://127.0.0.1:${DEV_PORT}/management.html"
else
  echo "Started (UI may need a moment). Log: tail -f $LOG_FILE"
fi
