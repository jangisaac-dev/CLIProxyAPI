#!/bin/bash
PLIST_NAME="local.cli-proxy-api-dev.plist"
LABEL="local.cli-proxy-api-dev"
DOMAIN="gui/$(id -u)"
TARGET="$HOME/Library/LaunchAgents/$PLIST_NAME"

launchctl bootout "$DOMAIN/$LABEL" 2>/dev/null || true
rm -f "$TARGET"

echo "✅ Auto-start removed."
