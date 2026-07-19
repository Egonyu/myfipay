#!/usr/bin/env bash
# Deploy committed code to production (P0-C, ENGINEERING_STANDARDS.md).
#
# Deploys HEAD only — refuses a dirty tree. Two independent parts:
#   site  : git-archive site/ into /var/www/myfipay-site/releases/<sha>,
#           atomically swap the `current` symlink nginx serves
#   api   : docker compose build+up, skipped when api/ is unchanged since
#           the last deployed API sha (force with --api)
#
# Usage: scripts/deploy.sh [--api] [--site-only]
set -euo pipefail

REPO=/var/www/myfibase
WEBROOT=/var/www/myfipay-site
RELEASES=$WEBROOT/releases
STATE=/var/www/.myfibase-deploy
KEEP_RELEASES=3

FORCE_API=0
SITE_ONLY=0
for arg in "$@"; do
    case "$arg" in
        --api) FORCE_API=1 ;;
        --site-only) SITE_ONLY=1 ;;
        *) echo "usage: $0 [--api] [--site-only]" >&2; exit 2 ;;
    esac
done

cd "$REPO"

dirty=$(git status --porcelain | grep -cv '^??' || true)
if [ "$dirty" -ne 0 ]; then
    echo "ABORT: working tree has $dirty uncommitted tracked change(s) — commit first, then deploy." >&2
    git status --short | grep -v '^??' >&2
    exit 1
fi
untracked=$(git status --porcelain | grep -c '^??' || true)
[ "$untracked" -ne 0 ] && echo "note: $untracked untracked file(s) present — they will NOT be deployed."

SHA=$(git rev-parse HEAD)
SHORT=$(git rev-parse --short HEAD)
mkdir -p "$RELEASES" "$STATE"

# ---- site ----
if [ ! -d "$RELEASES/$SHA" ]; then
    tmp=$(mktemp -d "$RELEASES/.tmp-$SHORT-XXXX")
    git archive HEAD site | tar -x --strip-components=1 -C "$tmp"
    # mktemp creates mode 700; nginx (www-data) must be able to read the release
    chmod -R u=rwX,go=rX "$tmp"
    mv "$tmp" "$RELEASES/$SHA"
fi
ln -s "$RELEASES/$SHA" "$WEBROOT/current.new"
mv -T "$WEBROOT/current.new" "$WEBROOT/current"
echo "site: $SHORT -> $WEBROOT/current"

# prune old releases (keep newest $KEEP_RELEASES)
ls -1t "$RELEASES" | tail -n "+$((KEEP_RELEASES + 1))" | while read -r old; do
    rm -rf "${RELEASES:?}/$old"
    echo "site: pruned old release $old"
done

# ---- api ----
if [ "$SITE_ONLY" -eq 1 ]; then
    echo "api: skipped (--site-only)"
else
    last_api=$(cat "$STATE/api-sha" 2>/dev/null || true)
    need_api=1
    if [ "$FORCE_API" -eq 0 ] && [ -n "$last_api" ] && git cat-file -e "$last_api" 2>/dev/null; then
        if git diff --quiet "$last_api" HEAD -- api/ docker-compose.yml; then
            need_api=0
        fi
    fi
    if [ "$need_api" -eq 1 ]; then
        echo "api: building from $SHORT ..."
        docker compose build api 2>&1 | tail -2
        docker compose up -d api 2>&1 | tail -2
        sleep 3
        echo "$SHA" > "$STATE/api-sha"
        echo "api: deployed $SHORT"
    else
        echo "api: unchanged since ${last_api:0:7}, skipped (force with --api)"
    fi
fi

# ---- verify ----
fail=0
health=$(curl -sf -m 10 http://127.0.0.1:8080/health || true)
case "$health" in
    *'"status":"ok"'*) echo "verify: API /health OK" ;;
    *) echo "verify: FAIL — API /health returned: ${health:-<no response>}" >&2; fail=1 ;;
esac
for path in / /dashboard/ /login; do
    code=$(curl -so /dev/null -w '%{http_code}' -m 10 --resolve myfipay.com:443:127.0.0.1 "https://myfipay.com$path" || true)
    if [ "$code" = 200 ]; then
        echo "verify: site $path 200"
    else
        echo "verify: FAIL — site $path returned $code" >&2
        fail=1
    fi
done

if [ "$fail" -ne 0 ]; then
    echo "DEPLOY FAILED verification — previous site releases are in $RELEASES; roll back by repointing $WEBROOT/current." >&2
    exit 1
fi
echo "deploy complete: $SHORT"
