# Public Codex OAuth and DuckDNS Operations

This document records the external-device Codex OAuth flow, the current
`cpa.ioq.kr` Cloudflare Tunnel runtime, the earlier `iscdx.duckdns.org`
DuckDNS runtime assumptions, the `cpa.iscdx.duckdns.org` alias, and the DuckDNS
update controls exposed through the CLIProxyAPI Management Web-UI.

## Current Cloudflare Tunnel Entry Points

Current Codex OAuth page:

```text
https://cpa.ioq.kr/codex/auth
```

Equivalent start page:

```text
https://cpa.ioq.kr/codex/oauth
```

Management page:

```text
https://cpa.ioq.kr/management.html
```

The Cloudflare Tunnel ingress should route only the intended public paths to the
local CLIProxyAPI service on `127.0.0.1:8317`:

```yaml
ingress:
  - hostname: cpa.ioq.kr
    path: ^/codex/.*
    service: http://127.0.0.1:8317
  - hostname: cpa.ioq.kr
    path: ^/management\.html$
    service: http://127.0.0.1:8317
  - hostname: cpa.ioq.kr
    path: ^/v0/management/.*
    service: http://127.0.0.1:8317
  - service: http_status:404
```

Expected public checks:

```bash
curl -sS --noproxy '*' -o /dev/null -w 'codex_auth=%{http_code}\n' https://cpa.ioq.kr/codex/auth
curl -sS --noproxy '*' -o /dev/null -w 'management=%{http_code}\n' https://cpa.ioq.kr/management.html
curl -sS --noproxy '*' -o /dev/null -w 'blocked=%{http_code}\n' https://cpa.ioq.kr/v1/models
```

Expected:

```text
codex_auth=200
management=200
blocked=404
```

## Current Public Entry Points

Current Codex OAuth page:

```text
http://iscdx.duckdns.org:8317/codex/oauth
```

Preferred alias:

```text
http://cpa.iscdx.duckdns.org:8317/codex/oauth
```

Equivalent start page:

```text
http://iscdx.duckdns.org:8317/codex/oauth/start
```

Equivalent alias start page:

```text
http://cpa.iscdx.duckdns.org:8317/codex/oauth/start
```

Health check:

```bash
curl -sS --noproxy '*' --max-time 5 http://iscdx.duckdns.org:8317/healthz
```

After TLS is actually reachable on 8317, the HTTPS form should also work:

```bash
curl -sS --noproxy '*' --max-time 5 https://iscdx.duckdns.org:8317/healthz
```

For the alias during local diagnosis:

```bash
curl -sS --noproxy '*' --max-time 5 http://cpa.iscdx.duckdns.org:8317/healthz
```

The portless form, `https://cpa.iscdx.duckdns.org/codex/oauth`, requires both a
trusted TLS certificate for the alias and either CLIProxyAPI listening on 443 or
a reverse proxy/port-forward from 443 to 8317. Until that is configured, keep
`:8317` in the public URL.

Local public reverse proxy runtime:

```text
~/.local/share/cliproxyapi/public-proxy
```

LaunchAgent:

```text
local.cli-proxy-api-public-proxy
```

The public proxy listens on ports 80 and 443 and forwards to the local
CLIProxyAPI runtime on 8317. Port 80 also serves ACME HTTP-01 challenge files
from:

```text
~/.local/share/cliproxyapi/public-proxy/acme-challenge
```

When a trusted certificate for `cpa.iscdx.duckdns.org` is installed at the
expected runtime path, port 80 redirects to HTTPS. Until then, port 80 proxies
directly to 8317 and port 443 uses the `iscdx.duckdns.org` fallback certificate,
which is not trusted for the `cpa.iscdx.duckdns.org` alias.

## Codex OAuth Flow

The public Codex page is intentionally outside the Management API. External
devices can open it without a management key.

The flow uses Codex device authentication:

- The browser opens `/codex/oauth`.
- The page calls `POST /codex/oauth/device/start`.
- CLIProxyAPI starts Codex device authentication on the server.
- The page displays the user code and a copy button.
- The user completes authentication at the OpenAI device URL.
- CLIProxyAPI persists the resulting Codex auth record on the server side.

This is not implemented as an iframe redirect capture. The device flow avoids
third-party iframe restrictions, cross-device callback problems, and browser
tunnel/proxy errors that can appear when trying to force a normal OAuth redirect
through an external browser.

## DuckDNS API

CLIProxyAPI uses the official DuckDNS update endpoint:

```text
https://www.duckdns.org/update
```

Required query parameters:

- `domains` - DuckDNS subdomain only, without `.duckdns.org`.
- `token` - DuckDNS account token.

Optional query parameters used here:

- `ip` - IPv4 address. If omitted, DuckDNS detects the public IPv4 from the
  request source.
- `verbose=true` - returns a more useful response such as `OK <ipv4> <ipv6> UPDATED`.

Official reference:

```text
https://www.duckdns.org/spec.jsp
```

## DuckDNS Config

The settings live in `config.yaml` under:

```yaml
duckdns:
  domain: "iscdx"
  token: ""
  ip: ""
```

Use `domain: "iscdx"`, not `iscdx.duckdns.org`.

Do not set `domain: "cpa.iscdx"`. DuckDNS rejects dotted domain names in the
update API. `cpa.iscdx.duckdns.org` is an alias under the `iscdx` DuckDNS
record, so updating `iscdx` is the correct operation.

Leave `ip` empty when the server's outbound public IPv4 should be detected by
DuckDNS automatically. Set `ip` only when a specific IPv4 must be forced.

The token is stored in the local config file, but the Management API does not
return the raw token. It returns `token_set` and a short preview only.

## Management API

All DuckDNS management endpoints require the normal Management API key.

Read settings:

```bash
curl -sS --noproxy '*' \
  -H 'X-Management-Key: cliproxy-dev-local' \
  https://iscdx.duckdns.org:8317/v0/management/duckdns
```

Save settings:

```bash
curl -sS --noproxy '*' \
  -X PATCH \
  -H 'Content-Type: application/json' \
  -H 'X-Management-Key: cliproxy-dev-local' \
  -d '{"domain":"iscdx","token":"<duckdns-token>","ip":""}' \
  https://iscdx.duckdns.org:8317/v0/management/duckdns
```

Update DuckDNS:

```bash
curl -sS --noproxy '*' \
  -X POST \
  -H 'X-Management-Key: cliproxy-dev-local' \
  https://iscdx.duckdns.org:8317/v0/management/duckdns/update
```

Successful response example:

```json
{"ok":true,"status":"UPDATED","ipv4":"203.0.113.10"}
```

Failure response example:

```json
{"error":"duckdns_update_failed","message":"KO"}
```

## Management Web-UI

The DuckDNS control appears as a small injected panel on:

```text
https://iscdx.duckdns.org:8317/management.html
```

The upstream React management bundle is not edited. CLIProxyAPI injects the
DuckDNS panel into the served HTML response, so the control remains available
even if the downloaded `management.html` asset is refreshed.

The panel is injected into the existing management page content. It reuses the
management key already stored by the Web-UI login flow and does not display a
separate management-key input.

The panel only mounts when the existing Management Web-UI content container is
present. It does not fall back to the app root on the login screen.

Panel fields:

- `Domain` - DuckDNS subdomain. The injected panel defaults to `iscdx`.
- `Token` - DuckDNS token. Leave blank on save to keep the existing token.
- `IP` - optional IPv4 override. The panel asks the server for its current public
  IPv4 on load and fills this field automatically.
- `Auto IP` - fetches the server's current public IPv4 again.
- `Save` - persists config.
- `Update IP` - calls DuckDNS immediately using the current field values.

Public IPv4 lookup API:

```bash
curl -sS --noproxy '*' \
  -H 'X-Management-Key: cliproxy-dev-local' \
  https://iscdx.duckdns.org:8317/v0/management/duckdns/public-ip
```

## TLS Notes

Chrome showing a danger warning on the management page is normally a certificate
trust issue. A public DuckDNS hostname still needs a certificate trusted by the
browser.

With `acme.sh`, the safe target is a normal Let's Encrypt certificate for:

```text
iscdx.duckdns.org
```

For the preferred alias, issue and install:

```text
cpa.iscdx.duckdns.org
```

HTTP-01 issuance for `cpa.iscdx.duckdns.org` requires public DNS resolvers and
DuckDNS authoritative nameservers to resolve the alias to the current public IP.
If any nameserver still returns an old IP, ACME validation can hit the wrong
host and fail with a 404.

After certificate issuance, configure CLIProxyAPI:

```yaml
tls:
  enable: true
  cert: "/path/to/fullchain.cer"
  key: "/path/to/iscdx.duckdns.org.key"
```

Then restart the launchd service once and verify `/healthz` over HTTPS.

If port 80 cannot be reached from the internet, use an ACME DNS challenge or a
temporary standalone/webroot flow that matches the network environment. Do not
replace the runtime binary and restart repeatedly while certificate issuance is
still being debugged.

## Safe Runtime Update

Current local runtime layout:

```text
~/.local/share/cliproxyapi/dev-8317
```

Important files:

```text
~/.local/share/cliproxyapi/dev-8317/cliproxyapi-dev
~/.local/share/cliproxyapi/dev-8317/config-8317.yaml
~/.local/share/cliproxyapi/dev-8317/run.sh
~/.local/share/cliproxyapi/dev-8317/launchd.log
```

LaunchAgent:

```text
local.cli-proxy-api-dev
```

Safe replacement pattern:

1. Build the new binary outside the live path.
2. Back up the current runtime binary with a timestamp.
3. Move the new binary into place atomically.
4. Restart launchd once with `kickstart -k`.
5. Verify status and `/healthz`.

Useful checks:

```bash
./dev status
```

```bash
launchctl print "gui/$(id -u)/local.cli-proxy-api-dev"
```

```bash
curl -sS --noproxy '*' --max-time 5 https://iscdx.duckdns.org:8317/healthz
```

```bash
tail -n 120 ~/.local/share/cliproxyapi/dev-8317/launchd.log
```

## Troubleshooting

`ERR_TUNNEL_CONNECTION_FAILED` usually points to a browser/system proxy path
trying to tunnel to the public host incorrectly. For local verification, always
start with `curl --noproxy '*'` so the health check bypasses inherited proxy
environment variables.

`duckdns_update_failed` with `KO` means DuckDNS rejected the request. Re-check
the subdomain and token first. The domain must be `iscdx`, not
`iscdx.duckdns.org`.

If `/management.html` loads but the DuckDNS panel reports `missing management key`
or `invalid management key`, provide the Management API key in the panel. The
public Codex OAuth page is intentionally unauthenticated, but DuckDNS updates
remain protected by management auth.

If the public OAuth page loads but device authentication fails, inspect the
launchd log and verify that the server can make outbound requests to OpenAI.
The public page itself does not need a management key.
