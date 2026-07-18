# Database Schema Design
## PostgreSQL 16

---

## Core Tables

```sql
-- ────────────────────────────────────────────────────────────────
-- TENANCY
-- ────────────────────────────────────────────────────────────────

CREATE TABLE tenants (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(255) NOT NULL,
    slug        VARCHAR(100) UNIQUE NOT NULL,      -- subdomain: slug.portal.com
    type        VARCHAR(20) NOT NULL DEFAULT 'operator', -- 'platform','agent','operator'
    parent_id   UUID REFERENCES tenants(id),       -- agent → operator hierarchy
    status      VARCHAR(20) DEFAULT 'active',
    settings    JSONB DEFAULT '{}',                -- branding, preferences
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW()
);

-- ────────────────────────────────────────────────────────────────
-- USERS (operator staff, agents, admins)
-- ────────────────────────────────────────────────────────────────

CREATE TABLE users (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL REFERENCES tenants(id),
    email       VARCHAR(255) UNIQUE NOT NULL,
    phone       VARCHAR(20),
    name        VARCHAR(255) NOT NULL,
    role        VARCHAR(30) NOT NULL,             -- 'super_admin','agent','operator','staff'
    password    VARCHAR(255) NOT NULL,            -- bcrypt
    status      VARCHAR(20) DEFAULT 'active',
    last_login  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW()
);

-- ────────────────────────────────────────────────────────────────
-- LOCATIONS (hotspot sites)
-- ────────────────────────────────────────────────────────────────

CREATE TABLE locations (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    name            VARCHAR(255) NOT NULL,
    district        VARCHAR(100),                 -- Soroti, Mbale, Tororo, etc.
    address         TEXT,
    lat             DECIMAL(10,7),
    lng             DECIMAL(10,7),
    ssid            VARCHAR(100),                 -- WiFi name shown to users
    portal_slug     VARCHAR(100) UNIQUE,          -- custom portal URL segment
    branding        JSONB DEFAULT '{}',           -- logo_url, primary_color, tagline
    status          VARCHAR(20) DEFAULT 'active',
    timezone        VARCHAR(50) DEFAULT 'Africa/Kampala',
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);

-- ────────────────────────────────────────────────────────────────
-- DEVICES (routers / edge agents)
-- ────────────────────────────────────────────────────────────────

CREATE TABLE devices (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    location_id     UUID NOT NULL REFERENCES locations(id),
    name            VARCHAR(255),
    type            VARCHAR(30) NOT NULL,          -- 'mikrotik','ubiquiti','tplink','edge_agent'
    nas_identifier  VARCHAR(255) UNIQUE,           -- RADIUS NAS-Identifier
    nas_ip          INET,
    radius_secret   VARCHAR(255) NOT NULL,
    mac_address     MACADDR,
    firmware_version VARCHAR(50),
    last_seen       TIMESTAMPTZ,
    online          BOOLEAN DEFAULT FALSE,
    edge_agent      BOOLEAN DEFAULT FALSE,
    agent_version   VARCHAR(20),
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);

-- ────────────────────────────────────────────────────────────────
-- PLANS (what operators sell)
-- ────────────────────────────────────────────────────────────────

CREATE TABLE plans (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    location_id     UUID NOT NULL REFERENCES locations(id),
    name            VARCHAR(100) NOT NULL,          -- "1 Hour", "Daily 2GB", "Weekly"
    description     TEXT,
    price_ugx       INTEGER NOT NULL,               -- price in UGX
    duration_mins   INTEGER,                        -- NULL = unlimited time
    data_mb         INTEGER,                        -- NULL = unlimited data
    speed_down_kbps INTEGER,                        -- NULL = unthrottled
    speed_up_kbps   INTEGER,
    simultaneous    INTEGER DEFAULT 1,              -- max devices on one session
    validity_days   INTEGER DEFAULT 1,              -- how long after purchase it's valid
    sort_order      INTEGER DEFAULT 0,
    active          BOOLEAN DEFAULT TRUE,
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);

-- ────────────────────────────────────────────────────────────────
-- PAYMENTS
-- ────────────────────────────────────────────────────────────────

CREATE TABLE payments (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    location_id     UUID NOT NULL REFERENCES locations(id),
    plan_id         UUID REFERENCES plans(id),
    customer_phone  VARCHAR(20) NOT NULL,
    amount_ugx      INTEGER NOT NULL,
    method          VARCHAR(20) NOT NULL,           -- 'mtn_momo','airtel_money','voucher'
    status          VARCHAR(20) DEFAULT 'pending',  -- pending/successful/failed/cancelled
    zengapay_ref    VARCHAR(255) UNIQUE,
    zengapay_status VARCHAR(50),
    idempotency_key VARCHAR(255) UNIQUE,
    initiated_at    TIMESTAMPTZ DEFAULT NOW(),
    confirmed_at    TIMESTAMPTZ,
    failed_at       TIMESTAMPTZ,
    failure_reason  TEXT,
    metadata        JSONB DEFAULT '{}'
);

CREATE INDEX idx_payments_location   ON payments(location_id);
CREATE INDEX idx_payments_phone      ON payments(customer_phone);
CREATE INDEX idx_payments_status     ON payments(status);
CREATE INDEX idx_payments_confirmed  ON payments(confirmed_at);

-- ────────────────────────────────────────────────────────────────
-- SESSIONS (active/past user sessions — also used by FreeRADIUS)
-- ────────────────────────────────────────────────────────────────

CREATE TABLE sessions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    location_id     UUID NOT NULL REFERENCES locations(id),
    device_id       UUID REFERENCES devices(id),
    plan_id         UUID REFERENCES plans(id),
    payment_id      UUID REFERENCES payments(id),
    voucher_id      UUID REFERENCES vouchers(id),   -- if paid by voucher
    username        VARCHAR(255) NOT NULL,           -- MAC address or phone
    mac_address     MACADDR,
    customer_phone  VARCHAR(20),
    ip_address      INET,
    nas_ip          INET,
    nas_id          VARCHAR(255),
    radius_session  VARCHAR(255),                   -- Acct-Session-Id from RADIUS
    status          VARCHAR(20) DEFAULT 'pending',  -- pending/active/expired/terminated
    data_used_mb    INTEGER DEFAULT 0,
    started_at      TIMESTAMPTZ,
    expires_at      TIMESTAMPTZ,
    last_seen       TIMESTAMPTZ,
    terminated_at   TIMESTAMPTZ,
    terminate_cause VARCHAR(50),                    -- User-Request, Session-Timeout, etc.
    created_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_sessions_username    ON sessions(username);
CREATE INDEX idx_sessions_location    ON sessions(location_id);
CREATE INDEX idx_sessions_status      ON sessions(status);
CREATE INDEX idx_sessions_mac         ON sessions(mac_address);
CREATE INDEX idx_sessions_expires     ON sessions(expires_at);

-- ────────────────────────────────────────────────────────────────
-- RADIUS ACCOUNTING (raw records from FreeRADIUS)
-- ────────────────────────────────────────────────────────────────

CREATE TABLE radius_accounting (
    id              BIGSERIAL PRIMARY KEY,
    session_id      UUID REFERENCES sessions(id),
    acct_session_id VARCHAR(255) NOT NULL,
    acct_status     VARCHAR(20) NOT NULL,           -- Start/Stop/Interim-Update
    nas_ip          INET,
    username        VARCHAR(255),
    framed_ip       INET,
    input_octets    BIGINT DEFAULT 0,
    output_octets   BIGINT DEFAULT 0,
    session_time    INTEGER DEFAULT 0,
    terminate_cause VARCHAR(50),
    recorded_at     TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_acct_session ON radius_accounting(acct_session_id);
CREATE INDEX idx_acct_recorded ON radius_accounting(recorded_at);

-- ────────────────────────────────────────────────────────────────
-- VOUCHERS
-- ────────────────────────────────────────────────────────────────

CREATE TABLE vouchers (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    location_id     UUID NOT NULL REFERENCES locations(id),
    plan_id         UUID NOT NULL REFERENCES plans(id),
    batch_id        UUID,                           -- group of vouchers printed together
    code            VARCHAR(20) UNIQUE NOT NULL,    -- e.g. "AB3-KX9-72P"
    status          VARCHAR(20) DEFAULT 'unused',   -- unused/active/expired/exhausted
    used_by_phone   VARCHAR(20),
    generated_at    TIMESTAMPTZ DEFAULT NOW(),
    expires_at      TIMESTAMPTZ,                    -- absolute expiry
    activated_at    TIMESTAMPTZ,
    exhausted_at    TIMESTAMPTZ
);

CREATE INDEX idx_vouchers_code     ON vouchers(code);
CREATE INDEX idx_vouchers_location ON vouchers(location_id);
CREATE INDEX idx_vouchers_batch    ON vouchers(batch_id);
CREATE INDEX idx_vouchers_status   ON vouchers(status);

CREATE TABLE voucher_batches (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    location_id UUID NOT NULL REFERENCES locations(id),
    plan_id     UUID NOT NULL REFERENCES plans(id),
    quantity    INTEGER NOT NULL,
    created_by  UUID REFERENCES users(id),
    note        TEXT,
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

-- ────────────────────────────────────────────────────────────────
-- AGENTS & COMMISSIONS
-- ────────────────────────────────────────────────────────────────

CREATE TABLE agent_operators (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_tenant_id UUID NOT NULL REFERENCES tenants(id),
    operator_tenant_id UUID NOT NULL REFERENCES tenants(id),
    commission_pct  DECIMAL(5,2) DEFAULT 5.00,     -- % of each transaction
    status          VARCHAR(20) DEFAULT 'active',
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(agent_tenant_id, operator_tenant_id)
);

CREATE TABLE agent_commissions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_tenant_id UUID NOT NULL REFERENCES tenants(id),
    payment_id      UUID NOT NULL REFERENCES payments(id),
    amount_ugx      INTEGER NOT NULL,
    status          VARCHAR(20) DEFAULT 'pending',  -- pending/settled
    settled_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ DEFAULT NOW()
);

-- ────────────────────────────────────────────────────────────────
-- OPERATOR SETTLEMENTS (payouts to operators)
-- ────────────────────────────────────────────────────────────────

CREATE TABLE settlements (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id),
    period_start    DATE NOT NULL,
    period_end      DATE NOT NULL,
    gross_ugx       BIGINT NOT NULL,
    platform_fee_ugx BIGINT NOT NULL,
    agent_fee_ugx   BIGINT DEFAULT 0,
    net_ugx         BIGINT NOT NULL,
    status          VARCHAR(20) DEFAULT 'pending',
    payout_phone    VARCHAR(20),
    payout_ref      VARCHAR(255),
    settled_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ DEFAULT NOW()
);

-- ────────────────────────────────────────────────────────────────
-- NOTIFICATIONS / SMS LOG
-- ────────────────────────────────────────────────────────────────

CREATE TABLE notifications (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID REFERENCES tenants(id),
    type        VARCHAR(50) NOT NULL,              -- 'payment_confirm','session_expiry','voucher_sms'
    recipient   VARCHAR(20) NOT NULL,              -- phone number
    message     TEXT NOT NULL,
    status      VARCHAR(20) DEFAULT 'queued',
    sent_at     TIMESTAMPTZ,
    error       TEXT,
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

-- ────────────────────────────────────────────────────────────────
-- PLATFORM SETTINGS & AUDIT
-- ────────────────────────────────────────────────────────────────

CREATE TABLE audit_log (
    id          BIGSERIAL PRIMARY KEY,
    tenant_id   UUID,
    user_id     UUID,
    action      VARCHAR(100) NOT NULL,
    resource    VARCHAR(100),
    resource_id UUID,
    old_value   JSONB,
    new_value   JSONB,
    ip_address  INET,
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_audit_tenant   ON audit_log(tenant_id);
CREATE INDEX idx_audit_user     ON audit_log(user_id);
CREATE INDEX idx_audit_created  ON audit_log(created_at);
```

---

## Redis Key Patterns

```
# Active session state (TTL = session expiry)
session:{session_id}        → HASH { status, data_used_mb, expires_at, mac }

# Pending payment (TTL = 5 minutes, auto-expire)
payment:pending:{idempotency_key}  → HASH { plan_id, phone, amount, location_id }

# Rate limiting (captive portal)
ratelimit:pay:{ip}          → INCR with TTL 60s

# Device online heartbeat (TTL = 90s, set by edge agent)
device:heartbeat:{device_id} → "1"

# Real-time revenue counter (per location, reset daily)
revenue:today:{location_id} → INTEGER (UGX)

# ZengaPay webhook deduplication
webhook:zengapay:{ref}      → "processed" (TTL = 24h)
```

---

## FreeRADIUS SQL Queries

These queries are used by FreeRADIUS `rlm_sql` module:

```sql
-- Auth query: does this session exist and is it valid?
-- Called on Access-Request
SELECT 'Cleartext-Password' AS attribute,
       username AS value,
       ':=' AS op
FROM sessions
WHERE username = '%{User-Name}'
  AND status = 'active'
  AND expires_at > NOW();

-- Session check query (simultaneous-use check)
SELECT COUNT(*) FROM sessions
WHERE username = '%{User-Name}'
  AND status = 'active';

-- Accounting start
INSERT INTO radius_accounting
    (acct_session_id, acct_status, nas_ip, username, framed_ip, recorded_at)
VALUES
    ('%{Acct-Session-Id}', 'Start', '%{NAS-IP-Address}',
     '%{User-Name}', '%{Framed-IP-Address}', NOW());

-- Accounting stop (update data used)
UPDATE sessions
SET data_used_mb = (%{Acct-Input-Octets} + %{Acct-Output-Octets}) / 1048576,
    terminated_at = NOW(),
    terminate_cause = '%{Acct-Terminate-Cause}',
    status = 'terminated'
WHERE radius_session = '%{Acct-Session-Id}';
```
