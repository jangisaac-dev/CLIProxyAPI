#!/bin/bash
set -e
PLIST_NAME="local.cli-proxy-api-dev.plist"
SOURCE_PLIST="$(cd "$(dirname "$0")" && pwd)/$PLIST_NAME"
TARGET="$HOME/Library/LaunchAgents/$PLIST_NAME"

mkdir -p "$HOME/Library/LaunchAgents"
cp "$SOURCE_PLIST" "$TARGET"

launchctl unload "$TARGET" 2>/dev/null || true
launchctl load "$TARGET"

echo "✅ Auto-start installed. The 8317 dev instance will start on login."
echo "To disable: ./scripts/dev-8317/uninstall-autostart.sh"
