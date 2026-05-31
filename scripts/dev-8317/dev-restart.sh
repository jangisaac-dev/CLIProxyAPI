#!/bin/bash
DEV_DIR="$(cd "$(dirname "$0")" && pwd)"
LABEL="local.cli-proxy-api-dev"
DOMAIN="gui/$(id -u)"

echo "Restarting dev instance..."
if launchctl print "$DOMAIN/$LABEL" >/dev/null 2>&1; then
  launchctl kickstart -k "$DOMAIN/$LABEL"
else
  "$DEV_DIR/dev-start.sh"
fi
