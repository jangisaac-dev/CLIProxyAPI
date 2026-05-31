#!/bin/bash
PLIST_NAME="local.cli-proxy-api-dev.plist"
TARGET="$HOME/Library/LaunchAgents/$PLIST_NAME"

launchctl unload "$TARGET" 2>/dev/null || true
rm -f "$TARGET"

echo "✅ Auto-start removed."
