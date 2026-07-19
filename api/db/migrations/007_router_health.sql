-- 007: Router health heartbeat
-- devices.online used to be derived only from RADIUS traffic (radpostauth/
-- radacct), so a healthy router with no customers looked offline. The host
-- cron scripts/router-heartbeat.sh now pings every registered router each
-- minute and records the result here; online = recent ping OR recent RADIUS.

ALTER TABLE devices ADD COLUMN IF NOT EXISTS last_ping TIMESTAMPTZ;
