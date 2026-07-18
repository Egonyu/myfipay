-- Agent network tables: referrals, commissions, payout requests

-- Links each operator tenant to the agent that recruited them (one agent per operator)
CREATE TABLE agent_referrals (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id     UUID NOT NULL REFERENCES tenants(id),
    operator_id  UUID NOT NULL REFERENCES tenants(id),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(operator_id)
);

CREATE INDEX idx_agent_referrals_agent ON agent_referrals(agent_id);

-- Commission earned by an agent for each confirmed payment from their operators
CREATE TABLE commissions (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id     UUID NOT NULL REFERENCES tenants(id),
    operator_id  UUID NOT NULL REFERENCES tenants(id),
    payment_id   UUID NOT NULL REFERENCES payments(id),
    amount_ugx   INTEGER NOT NULL,
    rate_pct     DECIMAL(5,2) NOT NULL DEFAULT 3.00,
    status       VARCHAR(20) NOT NULL DEFAULT 'pending',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(payment_id)
);

CREATE INDEX idx_commissions_agent    ON commissions(agent_id);
CREATE INDEX idx_commissions_operator ON commissions(operator_id);
CREATE INDEX idx_commissions_status   ON commissions(status);

-- Agent withdrawal requests
CREATE TABLE payout_requests (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id       UUID NOT NULL REFERENCES tenants(id),
    amount_ugx     INTEGER NOT NULL,
    method         VARCHAR(30) NOT NULL DEFAULT 'mtn_momo',
    phone          VARCHAR(20) NOT NULL,
    status         VARCHAR(20) NOT NULL DEFAULT 'pending',
    notes          TEXT,
    admin_notes    TEXT,
    requested_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at   TIMESTAMPTZ
);

CREATE INDEX idx_payout_requests_agent  ON payout_requests(agent_id);
CREATE INDEX idx_payout_requests_status ON payout_requests(status);
