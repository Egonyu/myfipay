# myFiBase

Hotspot billing platform for WiFi operators in Eastern Uganda — captive portal, mobile-money payments (ZengaPay), FreeRADIUS session control, operator dashboard, and an agent referral network.

## Architecture

```
Customer phone ──▶ Captive portal (Go, html/template)
                        │  POST /portal/{slug}/pay
                        ▼
                   ZengaPay collections ──▶ webhook ──▶ session grant
                        │                                   │
                        ▼                                   ▼
                   PostgreSQL (payments, sessions)     radcheck/radreply
                                                            │
                   MikroTik router ◀── FreeRADIUS ◀─────────┘
```

| Component | Tech | Path |
|---|---|---|
| API | Go 1.25, chi, pgx, go-redis | `api/` |
| Captive portal | Server-rendered HTML (no framework) | `api/internal/handlers/portal.go` |
| Operator dashboard | Next.js 15 (separate deploy) | `dashboard/` |
| RADIUS | FreeRADIUS 3.2 + rlm_sql → PostgreSQL | `freeradius/` (config mirror) |
| Reverse proxy | nginx (native) | `nginx/` (config mirror) |
| Infra | Docker Compose: postgres:16, redis:7, api, adminer | `docker-compose.yml` |

## Getting started (dev)

```bash
cp .env.example .env        # fill in secrets — never commit .env
docker compose up -d        # postgres, redis, api, adminer
# apply migrations in order:
for f in api/db/migrations/*.sql; do
  docker exec -i myfibase_postgres psql -U myfibase -d myfibase < "$f"
done
curl localhost:8080/health
```

## Migrations

Sequential SQL files in `api/db/migrations/` — `NNN_description.sql`, applied in order, additive only (no down-migrations). Current sequence:

1. `001_initial_schema.sql` — tenants, users, locations, devices, plans, payments, sessions, vouchers
2. `002_freeradius_tables.sql` — standard FreeRADIUS schema (radcheck, radreply, radacct, …)
3. `003_payouts.sql` — operator settlement (payouts)
4. `004_agent_network.sql` — agent referrals, commissions, payout requests

## Security notes

- All secrets live in `.env` (gitignored) or on the server — placeholder values in any committed config must be replaced at deploy time.
- JWT auth (HS256) with role gates: `operator`, `agent`, `admin`, `super_admin`.
- ZengaPay webhook: HMAC verification (when secret configured) + Redis SetNX deduplication.
- Rate limiting on payment initiation (10/IP/5min).

## Roles

| Role | Access |
|---|---|
| operator | Own tenant: dashboard, sessions, plans, locations, vouchers, payouts |
| agent | Referral network: invite operators, track 3% commissions, request payouts |
| admin / super_admin | KYC queue, tenant management, payout queues, platform revenue |

## Docs

`docs/` — architecture, PRD, API spec, DB schema, business model, hardware compatibility, and `TRACKER.md` (single source of truth for project status).
