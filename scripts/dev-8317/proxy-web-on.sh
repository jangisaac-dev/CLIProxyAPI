#!/bin/bash
# macOS system proxy + CLIProxyAPI dev proxy toggle.
# Safe when CLIProxyAPI is not running: system proxy changes still apply.

SERVICE_NAME="${PROXY_NETWORK_SERVICE:-Wi-Fi}"
PROXY_HOST="${PROXY_HOST:-172.20.10.1}"
PROXY_PORT="${PROXY_PORT:-3128}"
MANAGEMENT_URL="${MANAGEMENT_URL:-http://127.0.0.1:8317}"
MANAGEMENT_KEY="${MANAGEMENT_KEY:-${CLIPROXY_DEV_MANAGEMENT_KEY:-cliproxy-dev-local}}"

set_system_proxy() {
  /usr/sbin/networksetup -setwebproxy "$SERVICE_NAME" "$PROXY_HOST" "$PROXY_PORT"
  /usr/sbin/networksetup -setsecurewebproxy "$SERVICE_NAME" "$PROXY_HOST" "$PROXY_PORT"
  /usr/sbin/networksetup -setwebproxystate "$SERVICE_NAME" on
  /usr/sbin/networksetup -setsecurewebproxystate "$SERVICE_NAME" on
}

if [ "$(id -u)" -eq 0 ]; then
  set_system_proxy
else
  /usr/bin/osascript -e "do shell script \"/usr/sbin/networksetup -setwebproxy '$SERVICE_NAME' '$PROXY_HOST' '$PROXY_PORT' && /usr/sbin/networksetup -setsecurewebproxy '$SERVICE_NAME' '$PROXY_HOST' '$PROXY_PORT' && /usr/sbin/networksetup -setwebproxystate '$SERVICE_NAME' on && /usr/sbin/networksetup -setsecurewebproxystate '$SERVICE_NAME' on\" with administrator privileges"
fi

if [ -n "$MANAGEMENT_KEY" ]; then
  /usr/bin/curl -sS --max-time 3 -X PATCH "${MANAGEMENT_URL}/v0/management/proxy-settings" \
    -H "X-Management-Key: ${MANAGEMENT_KEY}" \
    -H "Content-Type: application/json" \
    -d "{\"enabled\":true,\"protocol\":\"http\",\"host\":\"${PROXY_HOST}\",\"port\":${PROXY_PORT}}" \
    >/dev/null 2>&1 || true
fi
