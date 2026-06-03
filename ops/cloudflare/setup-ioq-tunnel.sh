#!/bin/sh
set -eu

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
ENV_FILE="$ROOT/.env"

if [ ! -r "$ENV_FILE" ]; then
  echo "missing .env" >&2
  exit 1
fi

set -a
. "$ENV_FILE"
set +a

: "${CLOUDFLARE_ZONE_NAME:?missing CLOUDFLARE_ZONE_NAME}"
: "${CLOUDFLARE_TUNNEL_NAME:?missing CLOUDFLARE_TUNNEL_NAME}"
: "${CLOUDFLARE_TUNNEL_HOSTNAME:?missing CLOUDFLARE_TUNNEL_HOSTNAME}"
: "${CLOUDFLARE_TUNNEL_SERVICE:?missing CLOUDFLARE_TUNNEL_SERVICE}"

CF_RUN_TOKEN="${CLOUDFLARE_TUNNEL_RUN_TOKEN:-}"
CF_TUNNEL_TOKEN="${CLOUDFLARE_TUNNEL_API_TOKEN:-${CLOUDFLARE_API_TOKEN:-}}"
CF_DNS_TOKEN="${CLOUDFLARE_DNS_API_TOKEN:-${CLOUDFLARE_API_TOKEN:-}}"
: "${CF_DNS_TOKEN:?missing CLOUDFLARE_DNS_API_TOKEN or CLOUDFLARE_API_TOKEN}"
if [ -z "$CF_RUN_TOKEN" ]; then
  : "${CF_TUNNEL_TOKEN:?missing CLOUDFLARE_TUNNEL_RUN_TOKEN, CLOUDFLARE_TUNNEL_API_TOKEN, or CLOUDFLARE_API_TOKEN}"
fi

API="https://api.cloudflare.com/client/v4"
RUNTIME="$HOME/.local/share/cliproxyapi/cloudflare-tunnel"
CONFIG="$RUNTIME/config.yml"
CREDENTIALS="$RUNTIME/credentials.json"
TOKEN_FILE="$RUNTIME/token"
RUNNER="$RUNTIME/run.sh"
PLIST_SRC="$ROOT/ops/cloudflare/local.cli-proxy-api-cloudflared.plist"
PLIST_DST="$HOME/Library/LaunchAgents/local.cli-proxy-api-cloudflared.plist"

mkdir -p "$RUNTIME" "$HOME/Library/LaunchAgents"

api_with_token() {
  token="$1"
  method="$2"
  path="$3"
  data="${4:-}"
  echo "cloudflare_api=$method $path" >&2
  if [ -n "$data" ]; then
    curl -sS --fail-with-body \
      -X "$method" \
      -H "Authorization: Bearer $token" \
      -H "Content-Type: application/json" \
      --data "$data" \
      "$API$path"
  else
    curl -sS --fail-with-body \
      -X "$method" \
      -H "Authorization: Bearer $token" \
      -H "Content-Type: application/json" \
      "$API$path"
  fi
}

api_tunnel() {
  method="$1"
  path="$2"
  data="${3:-}"
  api_with_token "$CF_TUNNEL_TOKEN" "$method" "$path" "$data"
}

api_dns() {
  method="$1"
  path="$2"
  data="${3:-}"
  api_with_token "$CF_DNS_TOKEN" "$method" "$path" "$data"
}

decode_run_token_tunnel_id() {
  token="$1"
  decode_run_token_payload "$token" | jq -r '.t // empty' 2>/dev/null || true
}

decode_run_token_payload() {
  token="$1"
  padded="$token"
  case $((${#padded} % 4)) in
    2) padded="${padded}==" ;;
    3) padded="${padded}=" ;;
  esac
  printf '%s' "$padded" | base64 -D 2>/dev/null || true
}

upsert_env_value() {
  key="$1"
  value="$2"
  tmp="$ENV_FILE.tmp.$$"
  if grep -q "^$key=" "$ENV_FILE"; then
    awk -v key="$key" -v value="$value" '
      $0 ~ "^" key "=" { print key "=" value; found=1; next }
      { print }
      END { if (!found) print key "=" value }
    ' "$ENV_FILE" > "$tmp"
  else
    awk '1; END { print "" }' "$ENV_FILE" > "$tmp"
    printf '%s=%s\n' "$key" "$value" >> "$tmp"
  fi
  mv "$tmp" "$ENV_FILE"
  chmod 600 "$ENV_FILE"
}

CLOUDFLARE_ZONE_ID="${CLOUDFLARE_ZONE_ID:-}"
CLOUDFLARE_ACCOUNT_ID="${CLOUDFLARE_ACCOUNT_ID:-}"

if [ -z "$CLOUDFLARE_ZONE_ID" ] || [ -z "$CLOUDFLARE_ACCOUNT_ID" ]; then
  zone_name_encoded="$(jq -rn --arg value "$CLOUDFLARE_ZONE_NAME" '$value|@uri')"
  zone_json="$(api_dns GET "/zones?name=$zone_name_encoded")"
  detected_zone_id="$(printf '%s' "$zone_json" | jq -r '.result[0].id // empty')"
  detected_account_id="$(printf '%s' "$zone_json" | jq -r '.result[0].account.id // empty')"
  if [ -z "$detected_zone_id" ]; then
    echo "could not find Cloudflare zone for $CLOUDFLARE_ZONE_NAME" >&2
    exit 1
  fi
  if [ -z "$CLOUDFLARE_ZONE_ID" ]; then
    CLOUDFLARE_ZONE_ID="$detected_zone_id"
    upsert_env_value "CLOUDFLARE_ZONE_ID" "$CLOUDFLARE_ZONE_ID"
  fi
  if [ -z "$CLOUDFLARE_ACCOUNT_ID" ] && [ -n "$detected_account_id" ]; then
    CLOUDFLARE_ACCOUNT_ID="$detected_account_id"
    upsert_env_value "CLOUDFLARE_ACCOUNT_ID" "$CLOUDFLARE_ACCOUNT_ID"
  fi
fi

: "${CLOUDFLARE_ACCOUNT_ID:?missing CLOUDFLARE_ACCOUNT_ID}"
: "${CLOUDFLARE_ZONE_ID:?missing CLOUDFLARE_ZONE_ID}"

if [ -n "$CF_RUN_TOKEN" ]; then
  tunnel_token="$CF_RUN_TOKEN"
  run_token_payload="$(decode_run_token_payload "$tunnel_token")"
  tunnel_id="${CLOUDFLARE_TUNNEL_ID:-}"
  if [ -z "$tunnel_id" ]; then
    tunnel_id="$(printf '%s' "$run_token_payload" | jq -r '.t // empty' 2>/dev/null || true)"
  fi
  if [ -z "$tunnel_id" ]; then
    echo "missing CLOUDFLARE_TUNNEL_ID and could not decode tunnel id from CLOUDFLARE_TUNNEL_RUN_TOKEN" >&2
    exit 1
  fi
  account_tag="$(printf '%s' "$run_token_payload" | jq -r '.a // empty' 2>/dev/null || true)"
  tunnel_secret="$(printf '%s' "$run_token_payload" | jq -r '.s // empty' 2>/dev/null || true)"
  if [ -z "$account_tag" ] || [ -z "$tunnel_secret" ]; then
    echo "could not decode tunnel credentials from CLOUDFLARE_TUNNEL_RUN_TOKEN" >&2
    exit 1
  fi
  jq -nc \
    --arg account_tag "$account_tag" \
    --arg tunnel_id "$tunnel_id" \
    --arg tunnel_secret "$tunnel_secret" \
    '{AccountTag:$account_tag,TunnelID:$tunnel_id,TunnelSecret:$tunnel_secret}' > "$CREDENTIALS"
else
  tunnel_name_encoded="$(jq -rn --arg value "$CLOUDFLARE_TUNNEL_NAME" '$value|@uri')"
  tunnel_json="$(api_tunnel GET "/accounts/$CLOUDFLARE_ACCOUNT_ID/cfd_tunnel?is_deleted=false&name=$tunnel_name_encoded")"
  tunnel_id="$(printf '%s' "$tunnel_json" | jq -r '.result[0].id // empty')"

  if [ -z "$tunnel_id" ]; then
    create_json="$(api_tunnel POST "/accounts/$CLOUDFLARE_ACCOUNT_ID/cfd_tunnel" "$(jq -nc --arg name "$CLOUDFLARE_TUNNEL_NAME" '{name:$name,config_src:"cloudflare"}')")"
    tunnel_id="$(printf '%s' "$create_json" | jq -r '.result.id')"
    printf '%s' "$create_json" | jq -c '.result.credentials_file' > "$CREDENTIALS"
  fi

  token_json="$(api_tunnel GET "/accounts/$CLOUDFLARE_ACCOUNT_ID/cfd_tunnel/$tunnel_id/token")"
  tunnel_token="$(printf '%s' "$token_json" | jq -r '.result // empty')"
  if [ -z "$tunnel_token" ]; then
    echo "failed to fetch tunnel run token" >&2
    exit 1
  fi
fi
printf '%s\n' "$tunnel_token" > "$TOKEN_FILE"
chmod 600 "$TOKEN_FILE"
if [ -s "$CREDENTIALS" ]; then
  chmod 600 "$CREDENTIALS"
fi

if [ -z "$CF_RUN_TOKEN" ]; then
  config_body="$(jq -nc \
    --arg hostname "$CLOUDFLARE_TUNNEL_HOSTNAME" \
    --arg service "$CLOUDFLARE_TUNNEL_SERVICE" \
    '{
      config: {
        ingress: [
          {hostname:$hostname,path:"^/codex/.*",service:$service,originRequest:{}},
          {hostname:$hostname,path:"^/management\\.html$",service:$service,originRequest:{}},
          {hostname:$hostname,path:"^/v0/management/.*",service:$service,originRequest:{}},
          {service:"http_status:404"}
        ]
      }
    }')"
  api_tunnel PUT "/accounts/$CLOUDFLARE_ACCOUNT_ID/cfd_tunnel/$tunnel_id/configurations" "$config_body" >/dev/null
fi

record_name="$CLOUDFLARE_TUNNEL_HOSTNAME"
record_content="$tunnel_id.cfargotunnel.com"
record_name_encoded="$(jq -rn --arg value "$record_name" '$value|@uri')"
dns_json="$(api_dns GET "/zones/$CLOUDFLARE_ZONE_ID/dns_records?name=$record_name_encoded")"
record_id="$(printf '%s' "$dns_json" | jq -r '.result[0].id // empty')"
record_body="$(jq -nc --arg type CNAME --arg name "$record_name" --arg content "$record_content" '{type:$type,name:$name,content:$content,proxied:true}')"
if [ -n "$record_id" ]; then
  api_dns PUT "/zones/$CLOUDFLARE_ZONE_ID/dns_records/$record_id" "$record_body" >/dev/null
else
  api_dns POST "/zones/$CLOUDFLARE_ZONE_ID/dns_records" "$record_body" >/dev/null
fi

cat > "$CONFIG" <<EOF
tunnel: $tunnel_id
credentials-file: $CREDENTIALS
ingress:
  - hostname: $CLOUDFLARE_TUNNEL_HOSTNAME
    path: ^/codex/.*
    service: $CLOUDFLARE_TUNNEL_SERVICE
  - hostname: $CLOUDFLARE_TUNNEL_HOSTNAME
    path: ^/management\.html$
    service: $CLOUDFLARE_TUNNEL_SERVICE
  - hostname: $CLOUDFLARE_TUNNEL_HOSTNAME
    path: ^/v0/management/.*
    service: $CLOUDFLARE_TUNNEL_SERVICE
  - service: http_status:404
EOF

cat > "$RUNNER" <<EOF
#!/bin/sh
set -eu
exec /opt/homebrew/bin/cloudflared tunnel --no-autoupdate --config "$CONFIG" run "$tunnel_id"
EOF
chmod 755 "$RUNNER"

cp "$PLIST_SRC" "$PLIST_DST"
chmod 644 "$PLIST_DST"

if launchctl print "gui/$(id -u)/local.cli-proxy-api-cloudflared" >/dev/null 2>&1; then
  launchctl kickstart -k "gui/$(id -u)/local.cli-proxy-api-cloudflared"
else
  launchctl bootstrap "gui/$(id -u)" "$PLIST_DST"
fi

printf 'tunnel_id=%s\n' "$tunnel_id"
printf 'hostname=%s\n' "$CLOUDFLARE_TUNNEL_HOSTNAME"
