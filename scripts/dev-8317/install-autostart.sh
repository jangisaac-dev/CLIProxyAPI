#!/bin/bash
set -e
DEV_DIR="$(cd "$(dirname "$0")" && pwd)"

"$DEV_DIR/dev-start.sh"

echo "✅ Auto-start installed. The 8317 dev instance will start on login."
echo "To disable: ./scripts/dev-8317/uninstall-autostart.sh"
