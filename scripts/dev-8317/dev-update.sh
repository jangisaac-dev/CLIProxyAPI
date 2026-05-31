#!/bin/bash
set -e
REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
BRANCH="feature/management-proxy-control"
UPSTREAM_REMOTE="${CLIPROXY_UPSTREAM_REMOTE:-upstream}"

cd "$REPO_ROOT"

git_fetch() {
  if [ "${CLIPROXY_UPDATE_USE_PROXY:-0}" = "1" ]; then
    git fetch "$@"
    return
  fi

  env -u HTTP_PROXY -u HTTPS_PROXY -u ALL_PROXY -u http_proxy -u https_proxy -u all_proxy \
    git -c http.proxy= -c https.proxy= fetch "$@"
}

echo "=== Updating feature branch with latest upstream ==="
CURRENT=$(git branch --show-current)

if ! git diff --quiet || ! git diff --cached --quiet; then
  echo "Refusing to update with uncommitted tracked changes."
  echo "Commit or stash them first, then run ./update.sh again."
  exit 1
fi

if [ "$CURRENT" != "$BRANCH" ]; then
  git switch "$BRANCH"
fi

if git remote get-url "$UPSTREAM_REMOTE" >/dev/null 2>&1; then
  git_fetch "$UPSTREAM_REMOTE"
  git rebase "$UPSTREAM_REMOTE/main"
else
  git_fetch origin
  git rebase origin/main
fi

echo "✅ Branch updated (rebased on latest main)."
echo "Run ./dev-restart.sh if you want to use the new code."
