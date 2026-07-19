#!/usr/bin/env bash
# radius-sync.sh — make the running system match the "nas" table.
#
# The dashboard's router onboarding writes RADIUS clients into the nas table.
# FreeRADIUS (read_clients=yes) only reads that table at startup, and UFW only
# allows RADIUS from registered router IPs. So whenever the table changes:
#   1. UFW: allow udp/1812:1813 from each registered IP (rules tagged
#      "myfibase-nas"); drop tagged rules whose IP is no longer registered.
#   2. Restart FreeRADIUS to load the new client list.
# Runs from cron every minute (/etc/cron.d/myfibase-radius-sync); no-op unless
# the table changed since the last run.
#
# Restart (not reload) is required: with read_clients=yes FreeRADIUS 3.2 loads
# SQL clients only at startup — HUP answers "No files changed. Ignoring"
# (verified live 2026-07-19). NAS retransmission covers the sub-second gap.
# Revisit with a dynamic_clients virtual server if per-add restarts hurt at scale.
set -euo pipefail

STATE_DIR=/var/lib/myfibase
STATE_FILE=$STATE_DIR/radius-nas.hash
mkdir -p "$STATE_DIR"

psql_q() {
  docker exec myfibase_postgres psql -U myfibase -d myfibase -tA -c "$1"
}

HASH=$(psql_q "SELECT COALESCE(md5(string_agg(nasname||':'||secret, '|' ORDER BY nasname)), 'empty') FROM nas")
if [[ -f "$STATE_FILE" && "$(cat "$STATE_FILE")" == "$HASH" ]]; then
  exit 0
fi

echo "$(date -Is) nas table changed — syncing UFW + FreeRADIUS"

mapfile -t WANT < <(psql_q "SELECT DISTINCT nasname FROM nas")

# Drop tagged rules whose IP is gone (delete by number, highest first)
while read -r line; do
  num=$(sed -E 's/^\[ *([0-9]+)\].*/\1/' <<<"$line")
  ip=$(grep -oE '([0-9]{1,3}\.){3}[0-9]{1,3}' <<<"$line" | tail -1)
  keep=no
  for w in "${WANT[@]:-}"; do [[ "$w" == "$ip" ]] && keep=yes; done
  if [[ "$keep" == no && -n "$num" ]]; then
    echo "  ufw: removing rule for $ip"
    ufw --force delete "$num" >/dev/null
  fi
done < <(ufw status numbered | grep 'myfibase-nas' | tac)

# Add rules for new IPs
for ip in "${WANT[@]:-}"; do
  [[ -z "$ip" ]] && continue
  if ! ufw status | grep -q "1812:1813/udp.*$ip"; then
    echo "  ufw: allowing RADIUS from $ip"
    ufw allow proto udp from "$ip" to any port 1812:1813 comment 'myfibase-nas' >/dev/null
  fi
done

# Never let a broken config turn the cron restart into an outage
if ! freeradius -C >/dev/null 2>&1; then
  echo "$(date -Is) ERROR: freeradius config check failed — skipping restart" >&2
  exit 1
fi
systemctl restart freeradius
echo "$HASH" > "$STATE_FILE"
echo "$(date -Is) sync complete ($(psql_q 'SELECT COUNT(*) FROM nas') clients)"
