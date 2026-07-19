-- 006: radacct columns missing vs the stock FreeRADIUS 3.2 postgres schema.
-- FreeRADIUS 3.2's stock postgres accounting queries write these on every
-- Accounting-Request; without them every accounting packet fails with
-- UNDEFINED COLUMN and the NAS retries forever. Surfaced by the first real
-- MikroTik accounting Start (2026-07-19) — radtest never sends accounting.

ALTER TABLE radacct ADD COLUMN IF NOT EXISTS framedipv6address inet;
ALTER TABLE radacct ADD COLUMN IF NOT EXISTS framedipv6prefix inet;
ALTER TABLE radacct ADD COLUMN IF NOT EXISTS framedinterfaceid text;
ALTER TABLE radacct ADD COLUMN IF NOT EXISTS delegatedipv6prefix inet;
ALTER TABLE radacct ADD COLUMN IF NOT EXISTS connectinfo_start text;
ALTER TABLE radacct ADD COLUMN IF NOT EXISTS connectinfo_stop text;

-- Our schema declared these NOT NULL; stock has them nullable, and an
-- Accounting Start legitimately carries no terminate cause / session time,
-- so Start inserts violated the constraint (Stops worked — hence sessions
-- appearing only after disconnect).
ALTER TABLE radacct ALTER COLUMN username DROP NOT NULL;
ALTER TABLE radacct ALTER COLUMN acctsessiontime DROP NOT NULL;
ALTER TABLE radacct ALTER COLUMN acctinputoctets DROP NOT NULL;
ALTER TABLE radacct ALTER COLUMN acctoutputoctets DROP NOT NULL;
ALTER TABLE radacct ALTER COLUMN calledstationid DROP NOT NULL;
ALTER TABLE radacct ALTER COLUMN callingstationid DROP NOT NULL;
ALTER TABLE radacct ALTER COLUMN acctterminatecause DROP NOT NULL;
