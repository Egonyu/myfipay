-- 005: Router self-onboarding
-- FreeRADIUS reads NAS clients from the "nas" table (sql module read_clients=yes),
-- so registering a router in the dashboard makes it a valid RADIUS client after
-- the host sync script restarts freeradius. Standard FreeRADIUS schema.

CREATE TABLE IF NOT EXISTS nas (
    id          serial PRIMARY KEY,
    nasname     varchar(128) NOT NULL,          -- NAS IP address
    shortname   varchar(32)  NOT NULL,
    type        varchar(30)  NOT NULL DEFAULT 'other',
    ports       integer,
    secret      varchar(60)  NOT NULL,
    server      varchar(64),
    community   varchar(50),
    description varchar(200)
);
CREATE INDEX IF NOT EXISTS idx_nas_nasname ON nas (nasname);

-- Track which router each auth attempt came from so the dashboard can run a
-- per-device connection test (radpostauth has no NAS column by default; the
-- post-auth insert in queries.conf is extended to fill it).
ALTER TABLE radpostauth ADD COLUMN IF NOT EXISTS nasipaddress varchar(45);
CREATE INDEX IF NOT EXISTS idx_radpostauth_nasip ON radpostauth (nasipaddress, authdate DESC);
