#!/usr/bin/env bash
# router-heartbeat.sh — active health check for registered routers.
#
# The dashboard's "online" badge used to be passive (last RADIUS packet seen),
# which can't tell a dead router from a quiet one. This cron (every minute,
# /etc/cron.d/myfibase-router-heartbeat) pings each device's IP and refreshes:
#   last_ping — set when the router answers ICMP
#   last_seen — latest RADIUS activity (radpostauth/radacct) for its IP
#   online    — ping answered now, or RADIUS activity in the last 10 minutes
#
# Note: ICMP to the router's public IP; once the WireGuard management tunnel
# lands, ping the tunnel address instead (works behind CGNAT).
set -euo pipefail

psql_q() {
  docker exec myfibase_postgres psql -U myfibase -d myfibase -tA -c "$1"
}

while IFS='|' read -r id ip; do
  [[ -z "$id" || -z "$ip" ]] && continue
  ok=false
  if ping -c1 -W2 -q "$ip" >/dev/null 2>&1; then
    ok=true
  fi
  psql_q "
    UPDATE devices d SET
      last_ping = CASE WHEN $ok THEN NOW() ELSE d.last_ping END,
      last_seen = NULLIF(GREATEST(
        COALESCE(d.last_seen, 'epoch'),
        COALESCE((SELECT MAX(authdate) FROM radpostauth WHERE nasipaddress = host(d.nas_ip)), 'epoch'),
        COALESCE((SELECT MAX(COALESCE(acctupdatetime, acctstarttime)) FROM radacct WHERE nasipaddress = d.nas_ip), 'epoch')
      ), 'epoch'),
      online = $ok OR COALESCE(GREATEST(
        (SELECT MAX(authdate) FROM radpostauth WHERE nasipaddress = host(d.nas_ip)),
        (SELECT MAX(COALESCE(acctupdatetime, acctstarttime)) FROM radacct WHERE nasipaddress = d.nas_ip)
      ) > NOW() - INTERVAL '10 minutes', FALSE),
      updated_at = NOW()
    WHERE d.id = '$id'
  " >/dev/null
done < <(psql_q "SELECT id, host(nas_ip) FROM devices WHERE nas_ip IS NOT NULL")
