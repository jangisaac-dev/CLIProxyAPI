#!/bin/bash
# macOS system proxy + CLIProxyAPI dev proxy toggle.
# Safe when CLIProxyAPI is not running: system proxy changes still apply.

SERVICE_NAME="${PROXY_NETWORK_SERVICE:-Wi-Fi}"
MANAGEMENT_URL="${MANAGEMENT_URL:-http://127.0.0.1:8317}"
MANAGEMENT_KEY="${MANAGEMENT_KEY:-${CLIPROXY_DEV_MANAGEMENT_KEY:-cliproxy-dev-local}}"

set_system_proxy_off() {
  /usr/sbin/networksetup -setwebproxystate "$SERVICE_NAME" off
  /usr/sbin/networksetup -setsecurewebproxystate "$SERVICE_NAME" off
  /usr/sbin/networksetup -setsocksfirewallproxystate "$SERVICE_NAME" off
}

if [ "$(id -u)" -eq 0 ]; then
  set_system_proxy_off
else
  /usr/bin/osascript -e "do shell script \"/usr/sbin/networksetup -setwebproxystate '$SERVICE_NAME' off && /usr/sbin/networksetup -setsecurewebproxystate '$SERVICE_NAME' off && /usr/sbin/networksetup -setsocksfirewallproxystate '$SERVICE_NAME' off\" with administrator privileges"
fi

if [ -n "$MANAGEMENT_KEY" ]; then
  /usr/bin/curl -sS --max-time 3 -X PATCH "${MANAGEMENT_URL}/v0/management/proxy-settings" \
    -H "X-Management-Key: ${MANAGEMENT_KEY}" \
    -H "Content-Type: application/json" \
    -d '{"enabled":false}' \
    >/dev/null 2>&1 || true
fi
