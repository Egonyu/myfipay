#!/usr/bin/env bash
# wg-sync.sh — make wg0's peer list match the devices table.
#
# Router onboarding (API ensureWG) assigns each device a WireGuard keypair and
# tunnel IP; this cron (every minute, /etc/cron.d/myfibase-wg-sync) adds a wg0
# peer per device and removes peers whose device is gone. Diff-based against
# `wg show` (not a state file), so it self-heals after wg0 restarts or reboots.
set -euo pipefail

psql_q() {
  docker exec myfibase_postgres psql -U myfibase -d myfibase -tA -c "$1"
}

declare -A WANT
while IFS='|' read -r pub ip; do
  [[ -z "$pub" || -z "$ip" ]] && continue
  WANT[$pub]=$ip
done < <(psql_q "SELECT wg_public_key, host(wg_ip) FROM devices WHERE wg_public_key IS NOT NULL AND wg_ip IS NOT NULL")

# Remove peers no longer in the table
while read -r pub; do
  [[ -z "$pub" ]] && continue
  if [[ -z "${WANT[$pub]:-}" ]]; then
    echo "$(date -Is) removing stale peer $pub"
    wg set wg0 peer "$pub" remove
  fi
done < <(wg show wg0 peers)

# Add/update peers from the table (wg set is idempotent)
HAVE=$(wg show wg0 peers)
for pub in "${!WANT[@]}"; do
  if ! grep -qF "$pub" <<<"$HAVE"; then
    echo "$(date -Is) adding peer $pub -> ${WANT[$pub]}"
  fi
  wg set wg0 peer "$pub" allowed-ips "${WANT[$pub]}/32"
done
