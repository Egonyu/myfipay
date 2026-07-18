-- myFiBase payouts (operator settlement) domain
-- Operators withdraw mobile-money revenue held in the platform's ZengaPay account.
-- Cash payments are collected directly by operators and are NOT withdrawable here.

CREATE TABLE payouts (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    requested_by      UUID REFERENCES users(id),
    amount_ugx        INTEGER NOT NULL CHECK (amount_ugx > 0),
    momo_phone        VARCHAR(20) NOT NULL,
    momo_name         VARCHAR(255),
    status            VARCHAR(20) NOT NULL DEFAULT 'pending', -- pending | approved | paid | rejected
    reference         VARCHAR(255),    -- ZengaPay disbursement reference once paid
    note              TEXT,
    rejection_reason  TEXT,
    requested_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    reviewed_by       UUID REFERENCES users(id),
    reviewed_at       TIMESTAMPTZ,
    paid_at           TIMESTAMPTZ
);

CREATE INDEX idx_payouts_tenant ON payouts(tenant_id);
CREATE INDEX idx_payouts_status ON payouts(status);
CREATE INDEX idx_payouts_requested ON payouts(requested_at);
