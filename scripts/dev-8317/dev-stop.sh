#!/bin/bash
DEV_DIR="$(cd "$(dirname "$0")" && pwd)"
PLIST_NAME="local.cli-proxy-api-dev.plist"
LABEL="local.cli-proxy-api-dev"
DOMAIN="gui/$(id -u)"
TARGET="$HOME/Library/LaunchAgents/$PLIST_NAME"

echo "Stopping dev instance on 8317..."

launchctl bootout "$DOMAIN/$LABEL" 2>/dev/null || true

echo "Stopped."
