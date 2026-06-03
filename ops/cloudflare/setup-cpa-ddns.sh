#!/bin/sh
set -eu

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
RUNTIME="$HOME/.local/share/cliproxyapi/cloudflare-ddns"
UPDATER="$RUNTIME/update-cpa-ddns.sh"
RUNNER="$RUNTIME/run.sh"
RUNTIME_ENV="$RUNTIME/.env"
PLIST_SRC="$ROOT/ops/cloudflare/local.cli-proxy-api-cloudflare-ddns.plist"
PLIST_DST="$HOME/Library/LaunchAgents/local.cli-proxy-api-cloudflare-ddns.plist"
ENV_FILE="$ROOT/.env"

mkdir -p "$RUNTIME" "$HOME/Library/LaunchAgents"

cp "$ENV_FILE" "$RUNTIME_ENV"
chmod 600 "$RUNTIME_ENV"

cp "$ROOT/ops/cloudflare/update-cpa-ddns.sh" "$UPDATER"
chmod 755 "$UPDATER"

cat > "$RUNNER" <<EOF
#!/bin/sh
set -eu
export CLIPROXYAPI_ENV_FILE="$RUNTIME_ENV"
exec "$UPDATER"
EOF
chmod 755 "$RUNNER"

cp "$PLIST_SRC" "$PLIST_DST"
chmod 644 "$PLIST_DST"

if launchctl print "gui/$(id -u)/local.cli-proxy-api-cloudflare-ddns" >/dev/null 2>&1; then
  launchctl kickstart -k "gui/$(id -u)/local.cli-proxy-api-cloudflare-ddns"
else
  launchctl bootstrap "gui/$(id -u)" "$PLIST_DST"
fi

"$RUNNER"
