# Local Dev Proxy Management

This document records the local macOS workflow for the `feature/management-proxy-control`
branch. It covers the 8317 development service, proxy on/off helpers, restart behavior,
and update/upgrade flow used to keep a local fork in sync with upstream.

## Goals

The local workflow is designed for these requirements:

- Keep the project usable when an external proxy server is unavailable.
- Run the local development service from the project root with `start`, `stop`,
  `restart`, `update`, and `upgrade` commands.
- Allow macOS system proxy and CLIProxyAPI global proxy settings to be toggled together.
- Keep local changes on a dedicated branch while rebasing on upstream updates.
- Avoid direct process killing during normal restart paths, because stopping the wrong
  proxy process can break active Codex or CLI network requests.
- Survive reboot/login by using a persistent LaunchAgent runtime, not `/private/tmp`.

## Files

Root commands:

- `./dev` - command dispatcher for the local development workflow.
- `./start.sh` - starts the 8317 dev service.
- `./stop.sh` - stops the 8317 dev service through launchd.
- `./restart.sh` - rebuilds, re-registers, and restarts the 8317 dev service.
- `./update.sh` - rebases the local feature branch on latest upstream.
- `./upgrade.sh` - runs update, rebuild, and restart.
- `./install-autostart.sh` - installs and starts the LaunchAgent.
- `./uninstall-autostart.sh` - removes the LaunchAgent.

Implementation scripts:

- `scripts/dev-8317/dev-start.sh`
- `scripts/dev-8317/dev-stop.sh`
- `scripts/dev-8317/dev-restart.sh`
- `scripts/dev-8317/dev-update.sh`
- `scripts/dev-8317/dev-upgrade.sh`
- `scripts/dev-8317/install-autostart.sh`
- `scripts/dev-8317/uninstall-autostart.sh`
- `scripts/dev-8317/proxy-web-on.sh`
- `scripts/dev-8317/proxy-web-off.sh`
- `scripts/dev-8317/local.cli-proxy-api-dev.plist`

Installed macOS helper scripts:

- `/usr/local/sbin/proxy-web-on.sh`
- `/usr/local/sbin/proxy-web-off.sh`

## Runtime Layout

The LaunchAgent runs from a persistent user runtime directory:

```bash
~/.local/share/cliproxyapi/dev-8317
```

The runtime directory contains:

- `cliproxyapi-dev` - binary built from the current branch.
- `config-8317.yaml` - generated dev config.
- `run.sh` - launchd runner script.
- `panel/management.html` - local management UI asset if available.
- `launchd.log` - stdout/stderr for the LaunchAgent.

Do not use `/private/tmp` as the long-term LaunchAgent runtime. `/private/tmp` may be
cleaned during reboot or maintenance, which can leave launchd pointing at missing files.

## LaunchAgent

The installed LaunchAgent is:

```bash
~/Library/LaunchAgents/local.cli-proxy-api-dev.plist
```

It should point to:

```bash
~/.local/share/cliproxyapi/dev-8317/run.sh
```

It should use:

- `RunAtLoad = true`
- `KeepAlive = true`
- `WorkingDirectory = ~/.local/share/cliproxyapi/dev-8317`
- stdout/stderr under `~/.local/share/cliproxyapi/dev-8317/launchd.log`

Check the live launchd state:

```bash
launchctl print "gui/$(id -u)/local.cli-proxy-api-dev"
```

Expected high-signal fields:

```text
state = running
program = /Users/<user>/.local/share/cliproxyapi/dev-8317/run.sh
working directory = /Users/<user>/.local/share/cliproxyapi/dev-8317
```

## Start, Stop, Restart

Start:

```bash
./start.sh
```

Stop:

```bash
./stop.sh
```

Restart:

```bash
./restart.sh
```

`restart` intentionally delegates to `dev-start.sh`. This rebuilds the current branch,
rewrites the runtime config and runner, rewrites the LaunchAgent plist, then uses
launchd to boot the service again.

The normal flow uses:

- `launchctl bootout`
- `launchctl bootstrap`
- `launchctl enable`
- `launchctl kickstart -k`

It does not use broad `kill`, `pkill`, or process-name termination. Keep this property:
directly killing proxy-related processes can interrupt active CLI/Codex network calls.

## Status and Logs

Status:

```bash
./dev status
```

Expected output:

```text
=== CLIProxyAPI Dev (8317) Status ===
✅ LaunchAgent loaded
✅ Port 8317 listening
```

Logs:

```bash
./dev logs
```

Or directly:

```bash
tail -f ~/.local/share/cliproxyapi/dev-8317/launchd.log
```

## Management UI

Open:

```text
http://localhost:8317/management.html
```

The local management password is controlled by:

```bash
CLIPROXY_DEV_MANAGEMENT_KEY
```

Default:

```text
cliproxy-dev-local
```

Verify the management proxy state:

```bash
curl -sS --noproxy '*' \
  --max-time 5 \
  -H 'X-Management-Key: cliproxy-dev-local' \
  http://127.0.0.1:8317/v0/management/proxy-settings
```

Expected direct-mode result after startup:

```json
{"enabled":false,"host":"172.20.10.1","password":"","port":3128,"protocol":"http","proxy-url":"direct","username":""}
```

## Default Proxy Behavior

The generated dev config always overlays:

```yaml
logging-to-file: false
proxy-url: "direct"

proxy-settings:
  enabled: false
  protocol: "http"
  host: "172.20.10.1"
  port: 3128
```

This means the 8317 dev instance starts in direct mode. It does not require the external
proxy server at `172.20.10.1:3128` to be alive.

Override defaults only when needed:

```bash
CLIPROXY_DEV_PROXY_HOST=192.168.1.10 CLIPROXY_DEV_PROXY_PORT=8080 ./restart.sh
```

## Proxy Toggle Helpers

The macOS helper scripts are:

```bash
/usr/local/sbin/proxy-web-on.sh
/usr/local/sbin/proxy-web-off.sh
```

They should match the repo copies:

```bash
cmp -s scripts/dev-8317/proxy-web-on.sh /usr/local/sbin/proxy-web-on.sh
cmp -s scripts/dev-8317/proxy-web-off.sh /usr/local/sbin/proxy-web-off.sh
```

### `proxy-web-on.sh`

This script:

1. Enables macOS HTTP and HTTPS proxy for the selected network service.
2. Patches CLIProxyAPI management proxy settings to enabled.

Defaults:

```bash
PROXY_NETWORK_SERVICE=Wi-Fi
PROXY_HOST=172.20.10.1
PROXY_PORT=3128
MANAGEMENT_URL=http://localhost:8317
MANAGEMENT_KEY=cliproxy-dev-local
```

The management API call uses `curl --noproxy '*'` so localhost management calls do not
accidentally go through a broken external proxy.

### `proxy-web-off.sh`

This script:

1. Disables macOS HTTP proxy.
2. Disables macOS HTTPS proxy.
3. Disables macOS SOCKS proxy.
4. Patches CLIProxyAPI management proxy settings to disabled.

The CLIProxyAPI patch is best-effort. If the 8317 service is not running, the macOS
system proxy change still applies.

### Safety Rule

Do not run `proxy-web-on.sh` or `proxy-web-off.sh` from an active Codex session if the
session network depends on that same proxy path. Toggling the system proxy can interrupt
the current network request. Prefer validating file contents, installed paths, and the
management API unless an explicit proxy transition test is safe.

## Update and Upgrade

The branch is expected to be:

```text
feature/management-proxy-control
```

Remotes:

```text
origin   https://github.com/jangisaac-dev/CLIProxyAPI.git
upstream https://github.com/router-for-me/CLIProxyAPI.git
```

Update:

```bash
./update.sh
```

Upgrade:

```bash
./upgrade.sh
```

`update` refuses to run with uncommitted tracked changes. Commit or stash first.

By default, `dev-update.sh` clears proxy environment variables for `git fetch`:

```bash
HTTP_PROXY
HTTPS_PROXY
ALL_PROXY
http_proxy
https_proxy
all_proxy
```

It also clears Git HTTP proxy config for that command:

```bash
git -c http.proxy= -c https.proxy= fetch ...
```

This prevents a stopped local proxy from blocking `update`/`upgrade`.

If a network requires proxy access for GitHub, opt in explicitly:

```bash
CLIPROXY_UPDATE_USE_PROXY=1 ./update.sh
CLIPROXY_UPDATE_USE_PROXY=1 ./upgrade.sh
```

## Reboot Checklist

After reboot or logout/login:

1. Check LaunchAgent:

   ```bash
   launchctl print "gui/$(id -u)/local.cli-proxy-api-dev"
   ```

2. Check local status:

   ```bash
   ./dev status
   ```

3. Open:

   ```text
   http://localhost:8317/management.html
   ```

4. Check proxy state:

   ```bash
   curl -sS --noproxy '*' \
     --max-time 5 \
     -H 'X-Management-Key: cliproxy-dev-local' \
     http://127.0.0.1:8317/v0/management/proxy-settings
   ```

Expected:

- LaunchAgent is loaded.
- Port 8317 is listening.
- Management page opens.
- Proxy settings are direct unless intentionally toggled.

If the LaunchAgent is missing:

```bash
./install-autostart.sh
```

If the LaunchAgent is loaded but the service is stale:

```bash
./restart.sh
```

## Troubleshooting

### `Operation not permitted` from launchd

Symptom:

```text
/bin/bash: ./start.sh: Operation not permitted
/bin/bash: .../scripts/dev-8317/dev-run.sh: Operation not permitted
```

Cause:

launchd was trying to execute scripts under `~/Documents/.../CLIProxyAPI`, which can be
blocked by macOS privacy restrictions.

Fix:

Use the persistent runtime under:

```bash
~/.local/share/cliproxyapi/dev-8317
```

Then run:

```bash
./restart.sh
```

### `Bootstrap failed: 5: Input/output error`

Likely causes:

- LaunchAgent points at a missing runner.
- LaunchAgent working directory is not accessible to launchd.
- Runtime path was cleaned or not generated.

Checks:

```bash
plutil -lint ~/Library/LaunchAgents/local.cli-proxy-api-dev.plist
ls -l ~/.local/share/cliproxyapi/dev-8317
launchctl print "gui/$(id -u)/local.cli-proxy-api-dev"
```

Fix:

```bash
./restart.sh
```

### `curl` to localhost fails while browser works

The shell may contain proxy environment variables:

```bash
env | grep -i proxy
```

Use:

```bash
curl --noproxy '*' http://localhost:8317/management.html
```

The scripts use `--noproxy '*'` for local management calls for this reason.

### 8317 page does not open

Check launchd:

```bash
launchctl print "gui/$(id -u)/local.cli-proxy-api-dev"
```

Check port:

```bash
lsof -i :8317 -sTCP:LISTEN -n -P
```

Check logs:

```bash
tail -n 120 ~/.local/share/cliproxyapi/dev-8317/launchd.log
```

Restart safely:

```bash
./restart.sh
```

## Validation Commands

Run these after changing local dev scripts:

```bash
bash -n dev start.sh stop.sh restart.sh update.sh upgrade.sh \
  install-autostart.sh uninstall-autostart.sh \
  scripts/dev-8317/dev-start.sh \
  scripts/dev-8317/dev-stop.sh \
  scripts/dev-8317/dev-restart.sh \
  scripts/dev-8317/dev-update.sh \
  scripts/dev-8317/dev-upgrade.sh \
  scripts/dev-8317/install-autostart.sh \
  scripts/dev-8317/uninstall-autostart.sh \
  scripts/dev-8317/proxy-web-on.sh \
  scripts/dev-8317/proxy-web-off.sh
```

```bash
git diff --check
```

```bash
plutil -lint \
  scripts/dev-8317/local.cli-proxy-api-dev.plist \
  ~/Library/LaunchAgents/local.cli-proxy-api-dev.plist
```

```bash
./restart.sh
./dev status
```

```bash
curl -sS --noproxy '*' \
  --max-time 5 \
  -H 'X-Management-Key: cliproxy-dev-local' \
  http://127.0.0.1:8317/v0/management/proxy-settings
```

```bash
cmp -s scripts/dev-8317/proxy-web-on.sh /usr/local/sbin/proxy-web-on.sh
cmp -s scripts/dev-8317/proxy-web-off.sh /usr/local/sbin/proxy-web-off.sh
```

## Known Boundaries

- The proxy toggle scripts intentionally change macOS system proxy settings. Do not run
  them casually from automation when the current session depends on that proxy.
- `/usr/local/sbin` installation requires administrator privileges.
- `origin` currently points to the user's fork URL. Pushing requires valid GitHub
  authentication and repository access.
- `update`/`upgrade` rebase the feature branch; they will refuse to run with
  uncommitted tracked changes.
