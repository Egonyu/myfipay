#!/usr/bin/env bash
# Minute-cron watchdog (P0-D, ENGINEERING_STANDARDS §5). Alert-only — never
# restarts anything. Alerts on state transitions (DOWN/RECOVERED) so a sustained
# outage doesn't spam, plus a per-window alert when the API served any 5xx.
#
# Alerts go to https://ntfy.sh/$WATCHDOG_NTFY_TOPIC (var read from .env;
# subscribe in the ntfy mobile/web app). Everything is also appended to
# /var/log/myfibase-watchdog.log. External uptime (droplet-dead case) is
# covered separately by .github/workflows/uptime.yml.
set -u

ENV_FILE=/var/www/myfibase/.env
STATE_DIR=/var/lib/myfibase/watchdog
LOG=/var/log/myfibase-watchdog.log
mkdir -p "$STATE_DIR"

NTFY_TOPIC=$(grep -oP '^WATCHDOG_NTFY_TOPIC=\K.*' "$ENV_FILE" 2>/dev/null || true)

log() { echo "$(date '+%F %T') $*" >> "$LOG"; }

alert() { # $1 = message
    log "ALERT: $1"
    if [ -n "$NTFY_TOPIC" ]; then
        curl -sf -m 10 -H "Title: myfipay watchdog" -d "$1" \
            "https://ntfy.sh/$NTFY_TOPIC" >/dev/null 2>&1 || log "ntfy push failed"
    fi
}

# check <name> <human detail on failure>; reads $ok (0/1) set by caller
transition() { # $1=name $2=ok(0|1) $3=detail
    local prev
    prev=$(cat "$STATE_DIR/$1" 2>/dev/null || echo OK)
    if [ "$2" -eq 1 ]; then
        [ "$prev" = FAIL ] && alert "RECOVERED: $1"
        echo OK > "$STATE_DIR/$1"
    else
        [ "$prev" = OK ] && alert "DOWN: $1 — $3"
        echo FAIL > "$STATE_DIR/$1"
    fi
}

# --- API health (direct, bypasses nginx) ---
body=$(curl -sf -m 5 http://127.0.0.1:8080/health 2>/dev/null || true)
case "$body" in *'"status":"ok"'*) ok=1 ;; *) ok=0 ;; esac
transition api "$ok" "/health returned: ${body:-<no response>}"

# --- Site over nginx+TLS ---
code=$(curl -so /dev/null -w '%{http_code}' -m 10 \
    --resolve myfipay.com:443:127.0.0.1 https://myfipay.com/ 2>/dev/null || true)
[ "$code" = 200 ] && ok=1 || ok=0
transition site "$ok" "GET / returned HTTP ${code:-<none>}"

# --- FreeRADIUS ---
systemctl is-active --quiet freeradius && ok=1 || ok=0
transition freeradius "$ok" "systemd unit not active"

# --- Containers ---
for c in myfibase_postgres myfibase_redis myfibase_api; do
    state=$(docker inspect -f '{{.State.Status}} {{if .State.Health}}{{.State.Health.Status}}{{end}}' "$c" 2>/dev/null || echo missing)
    case "$state" in
        "running healthy"|"running ") ok=1 ;;
        *) ok=0 ;;
    esac
    transition "container-$c" "$ok" "state: $state"
done

# --- Disk ---
pct=$(df --output=pcent / | tail -1 | tr -dc '0-9')
[ "${pct:-100}" -lt 90 ] && ok=1 || ok=0
transition disk "$ok" "/ at ${pct}% (threshold 90%)"

# --- API 5xx surfacing (event, not state: alert per window it occurs) ---
errs=$(docker logs myfibase_api --since 70s 2>&1 | grep -cE ' - 5[0-9][0-9] |panic' || true)
if [ "${errs:-0}" -gt 0 ]; then
    sample=$(docker logs myfibase_api --since 70s 2>&1 | grep -E ' - 5[0-9][0-9] |panic' | tail -3)
    alert "API errors: $errs 5xx/panic line(s) in last 70s: $sample"
fi

exit 0
