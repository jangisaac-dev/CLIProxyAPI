#!/bin/sh
set -eu

ENV_FILE="${CLIPROXYAPI_ENV_FILE:-/Users/isaacjang/Documents/simple_tools/CLIProxyAPI/.env}"

if [ ! -r "$ENV_FILE" ]; then
  echo "missing .env" >&2
  exit 1
fi

set -a
. "$ENV_FILE"
set +a

if [ -n "${CLOUDFLARE_TUNNEL_RUN_TOKEN:-}" ]; then
  echo "cloudflare tunnel run token is configured; skipping direct A-record DDNS update" >&2
  exit 0
fi

API="https://api.cloudflare.com/client/v4"
CF_DNS_TOKEN="${CLOUDFLARE_DNS_API_TOKEN:-${CLOUDFLARE_API_TOKEN:-}}"
HOSTNAME="${CLOUDFLARE_DDNS_HOSTNAME:-${CLOUDFLARE_TUNNEL_HOSTNAME:-}}"
IP_URL="${CLOUDFLARE_DDNS_IP_URL:-https://api.ipify.org}"

: "${CF_DNS_TOKEN:?missing CLOUDFLARE_DNS_API_TOKEN or CLOUDFLARE_API_TOKEN}"
: "${CLOUDFLARE_ZONE_NAME:?missing CLOUDFLARE_ZONE_NAME}"
: "${HOSTNAME:?missing CLOUDFLARE_DDNS_HOSTNAME or CLOUDFLARE_TUNNEL_HOSTNAME}"

api_dns() {
  method="$1"
  path="$2"
  data="${3:-}"
  echo "cloudflare_api=$method $path" >&2
  if [ -n "$data" ]; then
    curl -sS --fail-with-body \
      -X "$method" \
      -H "Authorization: Bearer $CF_DNS_TOKEN" \
      -H "Content-Type: application/json" \
      --data "$data" \
      "$API$path"
  else
    curl -sS --fail-with-body \
      -X "$method" \
      -H "Authorization: Bearer $CF_DNS_TOKEN" \
      -H "Content-Type: application/json" \
      "$API$path"
  fi
}

public_ip="$(curl -sS --fail --noproxy '*' --max-time 10 "$IP_URL" | tr -d '[:space:]')"
if ! printf '%s\n' "$public_ip" | awk -F. '
  NF != 4 { exit 1 }
  {
    for (i = 1; i <= 4; i++) {
      if ($i !~ /^[0-9]+$/ || $i < 0 || $i > 255) exit 1
    }
  }
'; then
  echo "invalid public IPv4: $public_ip" >&2
  exit 1
fi

CLOUDFLARE_ZONE_ID="${CLOUDFLARE_ZONE_ID:-}"
if [ -z "$CLOUDFLARE_ZONE_ID" ]; then
  zone_name_encoded="$(jq -rn --arg value "$CLOUDFLARE_ZONE_NAME" '$value|@uri')"
  zone_json="$(api_dns GET "/zones?name=$zone_name_encoded")"
  CLOUDFLARE_ZONE_ID="$(printf '%s' "$zone_json" | jq -r '.result[0].id // empty')"
  if [ -z "$CLOUDFLARE_ZONE_ID" ]; then
    echo "could not find Cloudflare zone for $CLOUDFLARE_ZONE_NAME" >&2
    exit 1
  fi
fi

hostname_encoded="$(jq -rn --arg value "$HOSTNAME" '$value|@uri')"
records_json="$(api_dns GET "/zones/$CLOUDFLARE_ZONE_ID/dns_records?name=$hostname_encoded")"
record_id="$(printf '%s' "$records_json" | jq -r '.result[0].id // empty')"
record_body="$(jq -nc \
  --arg name "$HOSTNAME" \
  --arg content "$public_ip" \
  '{type:"A",name:$name,content:$content,proxied:true,ttl:1}')"

if [ -n "$record_id" ]; then
  api_dns PUT "/zones/$CLOUDFLARE_ZONE_ID/dns_records/$record_id" "$record_body" >/dev/null
else
  api_dns POST "/zones/$CLOUDFLARE_ZONE_ID/dns_records" "$record_body" >/dev/null
fi

printf 'hostname=%s\n' "$HOSTNAME"
printf 'public_ip=%s\n' "$public_ip"
