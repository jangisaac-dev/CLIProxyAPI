#!/bin/bash
DEV_DIR="$(cd "$(dirname "$0")" && pwd)"

echo "Restarting dev instance..."
exec "$DEV_DIR/dev-start.sh"
