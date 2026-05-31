#!/bin/bash
set -e
DEV_DIR="$(cd "$(dirname "$0")" && pwd)"

echo "=== Full upgrade: rebase + rebuild + restart ==="

"$DEV_DIR/dev-update.sh"

echo ""
echo "Rebuilding and restarting..."
"$DEV_DIR/dev-restart.sh"

echo ""
echo "✅ Upgrade complete. Running latest main + your proxy management features on 8317."
