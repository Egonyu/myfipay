-- myFiBase initial schema
-- Run once on a fresh database

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Tenants (operators, agents, platform)
CREATE TABLE tenants (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(255) NOT NULL,
    slug        VARCHAR(100) UNIQUE NOT NULL,
    type        VARCHAR(20) NOT NULL DEFAULT 'operator',
    parent_id   UUID REFERENCES tenants(id),
    status      VARCHAR(20) NOT NULL DEFAULT 'active',
    settings    JSONB NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Users (operator staff, agents, admins)
CREATE TABLE users (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    email       VARCHAR(255) UNIQUE NOT NULL,
    phone       VARCHAR(20),
    name        VARCHAR(255) NOT NULL,
    role        VARCHAR(30) NOT NULL DEFAULT 'operator',
    password    VARCHAR(255) NOT NULL,
    status      VARCHAR(20) NOT NULL DEFAULT 'active',
    last_login  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Locations (hotspot sites)
CREATE TABLE locations (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name         VARCHAR(255) NOT NULL,
    district     VARCHAR(100),
    address      TEXT,
    lat          DECIMAL(10,7),
    lng          DECIMAL(10,7),
    ssid         VARCHAR(100),
    portal_slug  VARCHAR(100) UNIQUE NOT NULL,
    branding     JSONB NOT NULL DEFAULT '{}',
    status       VARCHAR(20) NOT NULL DEFAULT 'active',
    timezone     VARCHAR(50) NOT NULL DEFAULT 'Africa/Kampala',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_locations_tenant ON locations(tenant_id);
CREATE INDEX idx_locations_slug   ON locations(portal_slug);

-- Devices (routers)
CREATE TABLE devices (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    location_id      UUID NOT NULL REFERENCES locations(id) ON DELETE CASCADE,
    name             VARCHAR(255),
    type             VARCHAR(30) NOT NULL DEFAULT 'mikrotik',
    nas_identifier   VARCHAR(255) UNIQUE,
    nas_ip           INET,
    radius_secret    VARCHAR(255) NOT NULL,
    online           BOOLEAN NOT NULL DEFAULT FALSE,
    last_seen        TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Plans
CREATE TABLE plans (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    location_id      UUID NOT NULL REFERENCES locations(id) ON DELETE CASCADE,
    name             VARCHAR(100) NOT NULL,
    description      TEXT,
    price_ugx        INTEGER NOT NULL CHECK (price_ugx >= 0),
    duration_mins    INTEGER,
    data_mb          INTEGER,
    speed_down_kbps  INTEGER,
    speed_up_kbps    INTEGER,
    sort_order       INTEGER NOT NULL DEFAULT 0,
    active           BOOLEAN NOT NULL DEFAULT TRUE,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_plans_location ON plans(location_id);

-- Payments
CREATE TABLE payments (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    location_id      UUID NOT NULL REFERENCES locations(id),
    plan_id          UUID REFERENCES plans(id),
    customer_phone   VARCHAR(20) NOT NULL,
    amount_ugx       INTEGER NOT NULL,
    method           VARCHAR(20) NOT NULL DEFAULT 'mtn_momo',
    status           VARCHAR(20) NOT NULL DEFAULT 'pending',
    zengapay_ref     VARCHAR(255) UNIQUE,
    idempotency_key  VARCHAR(255) UNIQUE,
    initiated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    confirmed_at     TIMESTAMPTZ,
    failed_at        TIMESTAMPTZ,
    failure_reason   TEXT,
    metadata         JSONB NOT NULL DEFAULT '{}'
);

CREATE INDEX idx_payments_location  ON payments(location_id);
CREATE INDEX idx_payments_phone     ON payments(customer_phone);
CREATE INDEX idx_payments_status    ON payments(status);
CREATE INDEX idx_payments_confirmed ON payments(confirmed_at);

-- Sessions
CREATE TABLE sessions (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    location_id      UUID NOT NULL REFERENCES locations(id),
    plan_id          UUID REFERENCES plans(id),
    payment_id       UUID REFERENCES payments(id),
    username         VARCHAR(255) NOT NULL,
    customer_phone   VARCHAR(20),
    ip_address       INET,
    nas_ip           INET,
    nas_id           VARCHAR(255),
    radius_session   VARCHAR(255),
    status           VARCHAR(20) NOT NULL DEFAULT 'pending',
    data_used_mb     INTEGER NOT NULL DEFAULT 0,
    started_at       TIMESTAMPTZ,
    expires_at       TIMESTAMPTZ,
    last_seen        TIMESTAMPTZ,
    terminated_at    TIMESTAMPTZ,
    terminate_cause  VARCHAR(50),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_sessions_username  ON sessions(username);
CREATE INDEX idx_sessions_location  ON sessions(location_id);
CREATE INDEX idx_sessions_status    ON sessions(status);
CREATE INDEX idx_sessions_expires   ON sessions(expires_at);

-- Vouchers
CREATE TABLE voucher_batches (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    location_id UUID NOT NULL REFERENCES locations(id),
    plan_id     UUID NOT NULL REFERENCES plans(id),
    quantity    INTEGER NOT NULL,
    note        TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE vouchers (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    batch_id     UUID REFERENCES voucher_batches(id),
    location_id  UUID NOT NULL REFERENCES locations(id),
    plan_id      UUID NOT NULL REFERENCES plans(id),
    code         VARCHAR(20) UNIQUE NOT NULL,
    status       VARCHAR(20) NOT NULL DEFAULT 'unused',
    used_by_phone VARCHAR(20),
    expires_at   TIMESTAMPTZ,
    activated_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_vouchers_code     ON vouchers(code);
CREATE INDEX idx_vouchers_location ON vouchers(location_id);
CREATE INDEX idx_vouchers_status   ON vouchers(status);

-- Seed: demo location for development
INSERT INTO tenants (name, slug, type) VALUES ('Demo Operator', 'demo', 'operator');

INSERT INTO locations (tenant_id, name, district, portal_slug, branding)
VALUES (
    (SELECT id FROM tenants WHERE slug = 'demo'),
    'Demo Hotspot', 'Soroti', 'demo',
    '{"primary_color": "#0f7a5a"}'
);

INSERT INTO plans (location_id, name, description, price_ugx, duration_mins, data_mb, sort_order)
VALUES
    ((SELECT id FROM locations WHERE portal_slug = 'demo'), '1 Hour',  '500 MB data',  500,  60,    500,   1),
    ((SELECT id FROM locations WHERE portal_slug = 'demo'), 'All Day', '2 GB data',   2000, 1440,  2048,   2),
    ((SELECT id FROM locations WHERE portal_slug = 'demo'), 'Weekly',  '10 GB data',  8000, 10080, 10240,  3);
