#!/bin/sh
set -eu

BASE="/Users/isaacjang/.local/share/cliproxyapi/public-proxy"
TARGET="${CLIPROXY_PUBLIC_PROXY_TARGET:-http://127.0.0.1:8317}"
CHALLENGE_DIR="$BASE/acme-challenge"
CPA_CERT="/Users/isaacjang/.local/share/cliproxyapi/certs/cpa.iscdx.duckdns.org/fullchain.cer"
CPA_KEY="/Users/isaacjang/.local/share/cliproxyapi/certs/cpa.iscdx.duckdns.org/cpa.iscdx.duckdns.org.key"
FALLBACK_CERT="/Users/isaacjang/.local/share/cliproxyapi/certs/iscdx.duckdns.org/fullchain.cer"
FALLBACK_KEY="/Users/isaacjang/.local/share/cliproxyapi/certs/iscdx.duckdns.org/iscdx.duckdns.org.key"

mkdir -p "$CHALLENGE_DIR"

CERT="$CPA_CERT"
KEY="$CPA_KEY"
HTTP_MODE="redirect"
if [ ! -r "$CERT" ] || [ ! -r "$KEY" ]; then
  CERT="$FALLBACK_CERT"
  KEY="$FALLBACK_KEY"
  HTTP_MODE="proxy"
fi

exec "$BASE/cliproxy-public-proxy" \
  -http ":80" \
  -https ":443" \
  -target "$TARGET" \
  -cert "$CERT" \
  -key "$KEY" \
  -challenge-dir "$CHALLENGE_DIR" \
  -http-mode "$HTTP_MODE"
