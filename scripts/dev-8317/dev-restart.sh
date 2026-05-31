#!/bin/bash
DEV_DIR="$(cd "$(dirname "$0")" && pwd)"

echo "Restarting dev instance..."
"$DEV_DIR/dev-stop.sh"
sleep 1
"$DEV_DIR/dev-start.sh"
