#!/bin/bash
set -e

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
DEV_DIR="$REPO_ROOT/scripts/dev-8317"
DEV_PORT="${CLIPROXY_DEV_PORT:-8317}"
DEV_MANAGEMENT_KEY="${CLIPROXY_DEV_MANAGEMENT_KEY:-cliproxy-dev-local}"
DEV_PROXY_HOST="${CLIPROXY_DEV_PROXY_HOST:-172.20.10.1}"
DEV_PROXY_PORT="${CLIPROXY_DEV_PROXY_PORT:-3128}"
RUNTIME_DIR="${CLIPROXY_DEV_RUNTIME_DIR:-/private/tmp/cliproxy-dev-8317}"
MAIN_CONF="/opt/homebrew/etc/cliproxyapi.conf"
CONFIG_FILE="$RUNTIME_DIR/config-8317.yaml"
BINARY="$RUNTIME_DIR/cliproxyapi-dev"
RUNNER="$RUNTIME_DIR/run.sh"
PLIST_NAME="local.cli-proxy-api-dev.plist"
SOURCE_PLIST="$DEV_DIR/$PLIST_NAME"
TARGET_PLIST="$HOME/Library/LaunchAgents/$PLIST_NAME"
LABEL="local.cli-proxy-api-dev"
DOMAIN="gui/$(id -u)"

echo "=== Preparing runtime for ${DEV_PORT} ==="
mkdir -p "$RUNTIME_DIR/panel"

if [ -f "$MAIN_CONF" ]; then
  cp "$MAIN_CONF" "$CONFIG_FILE"
  sed -i '' "s/^port: .*/port: ${DEV_PORT}/" "$CONFIG_FILE"
else
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

if [ -f "$DEV_DIR/panel/management.html" ]; then
  cp "$DEV_DIR/panel/management.html" "$RUNTIME_DIR/panel/management.html"
fi

cd "$REPO_ROOT"
export GOCACHE="${GOCACHE:-$REPO_ROOT/.gocache}"
export GOMODCACHE="${GOMODCACHE:-$REPO_ROOT/.gomodcache}"
mkdir -p "$GOCACHE" "$GOMODCACHE"
go build -o "$BINARY" ./cmd/server

cat > "$RUNNER" << RUNNER_EOF
#!/bin/bash
export MANAGEMENT_PASSWORD="$DEV_MANAGEMENT_KEY"
export MANAGEMENT_STATIC_PATH="$RUNTIME_DIR/panel/management.html"
exec "$BINARY" --config "$CONFIG_FILE" --no-browser
RUNNER_EOF
chmod 755 "$RUNNER"

echo "=== Installing launch agent for ${DEV_PORT} ==="
mkdir -p "$HOME/Library/LaunchAgents"
cp "$SOURCE_PLIST" "$TARGET_PLIST"

launchctl bootout "$DOMAIN/$LABEL" 2>/dev/null || true
launchctl bootstrap "$DOMAIN" "$TARGET_PLIST"
launchctl enable "$DOMAIN/$LABEL" 2>/dev/null || true
launchctl kickstart -k "$DOMAIN/$LABEL"

for _ in 1 2 3 4 5 6 7 8 9 10; do
  if curl -s --max-time 2 "http://127.0.0.1:${DEV_PORT}/management.html" -o /dev/null -w "%{http_code}" | grep -q "200"; then
    echo "✅ ${DEV_PORT} started: http://127.0.0.1:${DEV_PORT}/management.html"
    exit 0
  fi
  sleep 1
done

if launchctl print "$DOMAIN/$LABEL" >/dev/null 2>&1; then
  echo "Launch agent started, but HTTP check is not ready yet."
  echo "Check logs: tail -f $RUNTIME_DIR/launchd.log"
else
  echo "Launch agent failed to start."
  exit 1
fi
