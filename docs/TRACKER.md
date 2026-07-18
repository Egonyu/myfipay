# myFiBase — Project Tracker
**Last updated:** 2026-07-18
**Server:** 170.64.177.20 (DigitalOcean, Sydney — dev/staging)
**Target prod region:** DigitalOcean Nairobi (BLR1)

### Latest session (2026-07-18) — Agent Network shipped
- ✅ **Agent Network complete & deployed** — migration `004_agent_network.sql` applied (agent_referrals, commissions, payout_requests); API rebuilt + restarted
- ✅ Agent registration `POST /api/auth/register/agent` → tenant type `agent`, role `agent`, invite code = tenant slug
- ✅ Operator registration accepts `agent_code` → writes `agent_referrals` row (verified end-to-end live)
- ✅ Agent API: dashboard, invite link, operators list, commission history, payout request/list (`/api/agent/*`, RequireRole("agent"))
- ✅ Commission on confirmed mobile-money payment — 3% (`agentCommissionRate` const), keyed to the exact `payments` row via `RETURNING id` (duplicate webhooks and concurrent payments can't double/drop credit)
- ✅ Admin agent management: `/api/admin/agents`, `/api/admin/agent-payouts` list/approve/paid/reject
- ✅ Smoke-tested live: register agent → login → invite code → operator signup with code → referral row → dashboard count; 403 on agent→admin routes; test rows cleaned up
- ✅ Schema drift resolved: `sessions.mac_address` folded into `001_initial_schema.sql` (was live-only ALTER); migration numbering fixed (agent network renamed 003→004; 003 = payouts, already applied)

### Previous session (2026-06-25)
- ✅ White-label portal branding (per-location name/tagline/color/logo)
- ✅ KYC flow: self-signup → `pending_kyc` → admin approve/reject queue
- ✅ Admin panel: tenant list, platform revenue, KYC queue (super_admin)
- ✅ radacct bandwidth surfaced in dashboard cards + sessions table
- ✅ Voucher PDF export (A4 grid + 80mm thermal) with per-voucher QR codes
- ✅ Session expiry reaper (DB polling every 60s) — replaces per-session goroutines lost on restart
- ✅ **Payments persisted to DB** (cash + mobile-money), no longer Redis-only; `GET /api/payments` + dashboard page with method filter
- ✅ **Operator settlement / payouts** — request → admin approve → mark-paid; balance derived from mobile-money revenue minus 8% commission; `/payouts` + `/admin/payouts` pages
- ⏳ NextAuth login try/catch restored — verify in real browser (preview sandbox can't reach live API)

---

## Server Configuration — 170.64.177.20

### OS & Base
| Item | Value |
|---|---|
| OS | Ubuntu 24.04 LTS (Noble) |
| CPU | 1 vCPU |
| RAM | 1 GB |
| Disk | 25 GB SSD |
| SSH | `ssh myfibase` → root@170.64.177.20 (key-based auth) |
| App path | `/var/www/myfibase/` |
| SSH config | `~/.ssh/config` — Host `myfibase` configured |

### Native Services (systemd)
| Service | Status | Port | Notes |
|---|---|---|---|
| nginx | ✅ active | 80 (public) | Reverse proxy to API, serves captive portal |
| freeradius | ✅ active | UDP 1812/1813 | Auth + accounting via PostgreSQL rlm_sql |

### Docker Containers
| Container | Image | Port | Status |
|---|---|---|---|
| myfibase_api | local build (Go 1.25) | 127.0.0.1:8080 | ✅ running |
| myfibase_postgres | postgres:16-alpine | 127.0.0.1:5432 | ✅ healthy |
| myfibase_redis | redis:7-alpine | 127.0.0.1:6379 | ✅ healthy |
| myfibase_adminer | adminer:latest | 127.0.0.1:8081 | ✅ running |

### Nginx Config
- File: `/etc/nginx/sites-available/myfibase` (symlinked to sites-enabled)
- Routes: `/portal/*` → API, `/api/*` → API, `/webhooks/*` → API
- Root `/` → `302 /portal/demo/`
- Default site disabled

### FreeRADIUS Config
- Virtual server: `/etc/freeradius/3.0/sites-enabled/hotspot` (our custom config)
- SQL module: `/etc/freeradius/3.0/mods-enabled/sql` → connects to `127.0.0.1:5432`
- Clients config: `/etc/freeradius/3.0/clients.conf` — accepts `0.0.0.0/0`, secret on server (not in repo)
- EAP module disabled (not needed for captive portal)
- Default and inner-tunnel sites disabled

### Database
| Item | Value |
|---|---|
| DB name | myfibase |
| DB user | myfibase |
| Password | in `.env` on server |
| Migrations run | `001` initial, `002` freeradius, `003` payouts, `004` agent network (all applied) |

### Environment
- `.env` at `/var/www/myfibase/.env`
- `ZENGAPAY_API_URL` — `https://api.sandbox.zengapay.com` (sandbox)
- `ZENGAPAY_API_TOKEN` — sandbox public key in `.env` — wired and working
- `ZENGAPAY_WEBHOOK_SECRET` — empty (ZengaPay sandbox sends no signature headers; HMAC skipped when unset)

---

## Work Completed ✅

### Planning & Architecture
- [x] Product name, domain, business model locked (`DECISIONS.md`)
- [x] Full architecture document (`ARCHITECTURE.md`)
- [x] Product requirements doc (`PRD.md`)
- [x] Database schema design (`DATABASE_SCHEMA.md`)
- [x] API spec (`API_SPEC.md`)
- [x] Business model & pricing (`BUSINESS_MODEL.md`)
- [x] Hardware compatibility guide (`HARDWARE_COMPAT.md`)
- [x] All open questions answered and locked (`OPEN_QUESTIONS.md`)
- [x] ZengaPay real rates documented (flat fee warnings for micro-transactions)
- [x] XenFi competitive analysis complete
- [x] All docs uploaded to server at `/var/www/myfibase/docs/`
- [x] This tracker (`TRACKER.md`) created and maintained

### Go API — Core
- [x] Project scaffolded: `api/cmd/server/main.go`
- [x] Config loader: `api/internal/config/config.go` (reads `.env`)
- [x] Handler struct with DB + Redis + Config: `api/internal/handlers/handler.go`
- [x] Graceful shutdown (SIGINT/SIGTERM)
- [x] Chi router setup with all routes

### Go API — Captive Portal
- [x] Portal page handler: `GET /portal/:slug/` — renders phone-friendly HTML
- [x] Plan list embedded in template (1hr 500 UGX, All Day 2000 UGX, Weekly 8000 UGX)
- [x] No-JavaScript core flow (form POST works without JS)
- [x] JS polling for payment status every 3 seconds (progressive enhancement)
- [x] Session status handler: `GET /portal/:slug/session-status`
- [x] Locations list handler: `GET /api/locations`
- [x] Plans list handler: `GET /api/plans/:slug`

### Go API — Payments
- [x] Initiate payment: `POST /portal/:slug/pay` — validates, stores in Redis (10min TTL), calls ZengaPay
- [x] Location slug stored in Redis with payment (needed for session location_id lookup)
- [x] Payment status: `GET /portal/:slug/pay/:id/status`
- [x] ZengaPay webhook: `POST /webhooks/zengapay` — HMAC verification, deduplication via Redis SetNX
- [x] Dev mode (no token = simulate payment)
- [x] Idempotency keys (phone + plan + 5min window)

### Go API — ZengaPay Integration (fully wired)
- [x] Correct sandbox API URL: `https://api.sandbox.zengapay.com`
- [x] Collection request uses correct fields: `msisdn`, `amount`, `external_reference`, `narration`
- [x] No `callback_url` in request body — webhook URL is set globally in ZengaPay dashboard
- [x] Webhook parses real ZengaPay format: `{"event":"collection.success","data":{...}}`
- [x] `amount` handled as string (`"500.00"`) not integer
- [x] Status matched on both `event` field (`collection.success`) and `transactionStatus` (`SUCCEEDED`)
- [x] HMAC verification checks `X-Signature`, `X-Webhook-Signature`, `X-ZengaPay-Signature`
- [x] HMAC tries both raw secret and hex-decoded binary (ZengaPay format ambiguity handled)
- [x] Deduplication via Redis SetNX on `transactionReference`

### Go API — Session + RADIUS Integration
- [x] `createSessionAfterPayment` — writes session to PostgreSQL + radcheck + radreply
- [x] radcheck entry: `Auth-Type := Accept` (FreeRADIUS grants any password for this phone)
- [x] radreply entries: Session-Timeout, Idle-Timeout (300s), Mikrotik-Rate-Limit, WISPr bandwidth
- [x] All radreply values written as separate INSERTs (pgx strict typing — no multi-row VALUES with mixed types)
- [x] Per-plan bandwidth: 1h → 2/0.5 Mbps, Day → 5/1 Mbps, Week → 10/2 Mbps
- [x] Session written to PostgreSQL with correct `location_id` (via `portal_slug` lookup) and `plan_id` (via name lookup)
- [x] `expireSession` — removes radcheck/radreply rows, updates PostgreSQL session, clears Redis
- [x] Background goroutine auto-expires session at plan duration + 10s

### Database
- [x] Migration 001: tenants, users, locations, devices, plans, payments, sessions, voucher_batches, vouchers
- [x] Migration 002: FreeRADIUS standard tables (radcheck, radreply, radgroupcheck, radgroupreply, radusergroup, radpostauth, radacct)
- [x] Demo seed: tenant "Demo Operator", location `portal_slug="demo"`, 3 demo plans

### Infrastructure
- [x] Docker Compose: postgres, redis, api, adminer
- [x] API Dockerfile (Go 1.25 multi-stage build, static binary, ~12MB)
- [x] Nginx installed natively (apt) and configured — proxies `/portal/`, `/api/`, `/webhooks/`
- [x] FreeRADIUS 3.2.5 installed natively (apt) and configured
- [x] FreeRADIUS SQL module connecting to Docker postgres on `127.0.0.1:5432`
- [x] FreeRADIUS EAP module disabled (not needed for captive portal)
- [x] `clients.conf` accepts any NAS IP — RADIUS secret on server (not in repo)
- [x] UFW rules added (80/tcp, 443/tcp, 1812/udp, 1813/udp)
- [x] Git repository initialized locally

---

## Verified End-to-End ✅

| Test | Result |
|---|---|
| `http://170.64.177.20/` → redirect | ✅ 302 → /portal/demo/ |
| `http://170.64.177.20/portal/demo/` | ✅ 200, portal HTML rendered |
| RADIUS reject (unknown user) | ✅ Access-Reject returned |
| RADIUS accept (phone in radcheck) | ✅ Access-Accept returned |
| FreeRADIUS → PostgreSQL SQL query | ✅ rlm_sql connected and working |
| Docker postgres healthcheck | ✅ healthy |
| Docker redis healthcheck | ✅ healthy |
| ZengaPay sandbox payment initiation | ✅ 200 from `api.sandbox.zengapay.com/v1/collections` |
| ZengaPay sandbox webhook delivery | ✅ fires from `188.245.65.108` (ZENGAPAY/1.0), server returns 200 |
| Webhook deduplication (Redis SetNX) | ✅ duplicate webhook returns 200 without re-processing |
| `radcheck` written after payment | ✅ `Auth-Type := Accept` row inserted for phone number |
| `radreply` written after payment | ✅ Session-Timeout, Idle-Timeout, Mikrotik-Rate-Limit, WISPr-Bandwidth rows |
| PostgreSQL `sessions` row created | ✅ `status=active`, correct `location_id`, `plan_id`, `expires_at` |
| `radtest` on provisioned phone | ✅ `Access-Accept` (93 bytes) with full bandwidth attributes |
| Full flow (pay → webhook → WiFi grant) | ✅ confirmed 2026-06-25 |

---

## In Progress / Partially Done ⏳

| Item | Status | Notes |
|---|---|---|
| ZengaPay sandbox integration | ✅ Working — sandbox only | Need separate ZengaPay account for prod (current account shared with TesoTunes) |
| ZengaPay prod account | Pending | Short-term option: webhook forwarder on TesoTunes; long-term: separate myFiBase account |
| MAC address capture in portal | Missing | Router redirect URL has `?mac=XX:XX` — portal needs to read + store it |
| Plans loaded from DB | ✅ Done 2026-06-25 | `PortalPage` now queries locations + plans; `getPlanFromDB` replaces 3 hardcoded helpers; schema column fixes (duration_mins, active bool) across operator.go + portal.go + payment.go |
| SSL / HTTPS | Not configured | Need domain + Let's Encrypt cert |
| Domain pointing | Not configured | myfibase.ug DNS not pointed to 170.64.177.20 yet |

---

## Not Yet Started ❌

### Authentication & Operators
- [x] JWT auth for operator dashboard API — Bearer token via NextAuth v4 + Sanctum-style Go JWT
- [x] Operator registration / KYC flow — `POST /api/auth/register` → `pending_kyc` → admin approve/reject
- [x] Operator login endpoint — `POST /api/auth/login`, returns JWT + user; KYC error codes surfaced
- [ ] Password reset + email verification
- [ ] Session token storage (Redis)

### Operator Dashboard (Next.js) — ✅ M3 Complete 2026-06-25
- [x] Project scaffold (`dashboard/` directory) — Next.js 15, Tailwind v4, NextAuth v4
- [x] Login page — dark Greeva-style with password toggle
- [x] Overview/Dashboard page — stat cards + ApexCharts (area + bar)
- [x] Sessions page — dark table with status filter tabs, terminate action
- [x] Plans management — dark table, create/edit/deactivate modal
- [x] Locations management — dark card grid, add location modal, copy portal URL
- [x] Payments page — summary stat cards + revenue history table
- [x] Settings page — profile, password change, RADIUS config display
- [x] JWT auth end-to-end — login → Bearer token → protected API routes
- [x] Dark theme — Greeva standard (--bg-base #16191e, --accent #4361ee)
- [x] Manual session grant (cash payment)
- [x] MAC address captured from router redirect, stored in Redis + DB
- [x] Session extension (top-up without disconnect)
- [x] Voucher batch generation (create batch, list batches, view codes, copy-to-clipboard)
- [x] Voucher PDF export (A4 + thermal printer format with QR codes)
- [x] Operator settings: white-label portal — name, tagline, accent color, logo per location (`locations.branding` JSONB + BrandingModal)

### Operator Mobile App (React Native / Expo)
- [ ] Project scaffold
- [ ] Login screen
- [ ] Home dashboard (summary cards)
- [ ] Session list
- [ ] Grant session manually

### Agent Network — ✅ Complete 2026-07-18
- [x] Agent role and dashboard — `POST /api/auth/register/agent`, `GET /api/agent/dashboard` (RequireRole "agent")
- [x] Operator invite tool — `GET /api/agent/invite`; operator `POST /api/auth/register` accepts `agent_code` → `agent_referrals`
- [x] Commission tracker — 3% per confirmed payment (`agentCommissionRate`), `GET /api/agent/commissions`, `GET /api/agent/operators`
- [x] Agent payout request — `POST/GET /api/agent/payouts` (min UGX 5,000, balance-checked); admin queue `/api/admin/agent-payouts` approve/paid/reject
- [x] Admin agent list — `GET /api/admin/agents` (operator count + lifetime commission per agent)
- [x] DB: `004_agent_network.sql` — agent_referrals (UNIQUE operator), commissions (UNIQUE payment), payout_requests
- [ ] Agent dashboard UI (Next.js pages under `dashboard/`)
- [ ] Wholesale voucher purchase for agents (5% below retail, per BUSINESS_MODEL.md)

### Admin Panel
- [x] Super admin login — `admin@myfibase.ug`, role `super_admin` (credentials on server)
- [x] Tenant (operator) management — `/admin/tenants` page, aggregated stats
- [x] Platform-wide revenue view — `/admin/revenue` page, ApexCharts + per-tenant table
- [x] KYC approval queue — `/admin/kyc` page, approve/reject with reason modal
- [ ] Platform settings

### Payments — Full Flow
- [ ] ZengaPay live API token wired in `.env`
- [ ] Webhook HMAC secret wired in `.env`
- [ ] ZengaPay IP allowlist in webhook handler
- [x] Operator settlement / payout request — `payouts` table (migration `003_payouts.sql`); operator `GET /api/payouts`, `GET /api/payouts/balance`, `POST /api/payouts`; admin `GET /api/admin/payouts` + approve/reject/mark-paid; dashboard `/payouts` (balance cards + request modal) and `/admin/payouts` (queue with actions). Balance = mobile-money revenue × (1 − commission 8%) − requested; cash excluded (operator holds it). Min withdrawal UGX 5,000.
- [ ] ZengaPay disbursement API call on payout approval (currently manual mark-paid)
- [x] Cash payment recording (no ZengaPay call) — `GrantSession` writes a confirmed `payments` row (method=cash) with granted_by/session_id/note in metadata
- [x] Mobile-money payment recording to DB — `createSessionAfterPayment` writes confirmed `payments` row (method=mobile_money, zengapay_ref) — payments table no longer Redis-only
- [x] Payment history page (per operator) — `GET /api/payments` (+ `?method=cash|mobile_money`); dashboard `/payments` page with method tabs + cash-collected summary
- [ ] Refund handling

### Vouchers
- [x] Voucher batch generation (Go) — `POST /api/vouchers/batches`
- [x] Voucher list by batch — `GET /api/vouchers/batches/{id}`
- [x] Voucher redemption endpoint — `POST /portal/:slug/voucher` (full RADIUS session grant)
- [x] Dashboard vouchers page with create modal + copy-to-clipboard
- [x] PDF export (A4 + thermal printer format) — browser print window; A4 4-column grid or 80mm thermal roll
- [x] QR code per voucher — qrcode npm package generates data URLs, embedded in print sheet

### Edge Agent (Raspberry Pi / MikroTik CHR)
- [ ] Go binary scaffold for edge agent
- [ ] Embedded mini RADIUS server (proxies to cloud)
- [ ] SQLite local session cache
- [ ] Payment queue (retries on reconnect)
- [ ] Heartbeat + sync with cloud
- [ ] Cross-compile to ARM (`GOARCH=arm64`)
- [ ] Installer script for Pi
- [ ] OTA update mechanism

### Session Lifecycle — Gaps
- [ ] MAC address captured from router redirect and linked to phone session
- [ ] Plans loaded dynamically from DB (not hardcoded in Go)
- [ ] Session renewal (extend without disconnect)
- [ ] Multi-device detection (same phone, two devices)
- [x] RADIUS accounting data surfaced in dashboard — bandwidth (acctinputoctets+acctoutputoctets) in stat cards + sessions table

### Infrastructure
- [ ] SSL certificate (Let's Encrypt via certbot)
- [ ] Domain DNS: myfibase.ug → 170.64.177.20 (dev) then Nairobi droplet (prod)
- [ ] GitHub repository (remote) + CI/CD with GitHub Actions
- [ ] Production droplet in Nairobi (DO-BLR1)
- [ ] Separate DB droplet for production
- [ ] Automated daily DB backup to DigitalOcean Spaces
- [ ] Prometheus + Grafana monitoring
- [ ] Log rotation (freeradius, nginx, API)
- [ ] Fail2ban (SSH brute force protection)
- [ ] Uptime monitoring (Uptime Kuma or BetterStack)

### SMS Notifications
- [ ] Africa's Talking or Yo! Uganda account setup
- [ ] SMS on session start: "Your 1hr WiFi is active. Expires 3:45 PM"
- [ ] SMS on session expiry warning (15min before)
- [ ] SMS on top-up

### Testing
- [ ] Unit tests (Go) for payment + session logic
- [ ] Integration test: full pay → webhook → RADIUS flow
- [ ] Load test (k6): 100 concurrent portal users
- [ ] MikroTik live test: real router connecting to RADIUS

---

## Known Issues & Gaps

| Issue | Severity | Fix |
|---|---|---|
| Dev server is 1 vCPU / 1GB RAM in Sydney | Low (dev only) | Upgrade + move to Nairobi for production |
| `clients.conf` accepts any IP `0.0.0.0/0` | Medium | Lock to specific NAS IPs when real routers are registered |
| Plans are hardcoded in `payment.go` | Medium | Query `plans` table by slug at portal load time |
| Portal gets phone but not MAC address | High | Parse `?mac=` from router redirect URL, store in session |
| No SSL — portal served over HTTP | High (before prod) | certbot + Let's Encrypt on myfibase.ug |
| ZengaPay prod account shared with TesoTunes | High (before prod) | Get dedicated myFiBase ZengaPay account OR build webhook forwarder |
| ~~No rate limiting on `/portal/:slug/pay`~~ | ✅ Fixed | Redis INCR, 10 req per IP per 5 min |
| ~~Session expiry goroutine lost on restart~~ | ✅ Fixed | `StartSessionReaper` polls DB every 60s in background goroutine; goroutines removed from session creation paths |
| No operator auth on any API routes | High | JWT middleware needed before any operator routes go live |
| ~~`payment_id` not persisted to DB payments table~~ | ✅ Fixed | Both cash (`GrantSession`) and mobile-money (`createSessionAfterPayment`) now write confirmed `payments` rows; surfaced via `GET /api/payments` |

---

## Milestone Summary

| Milestone | Status | Target |
|---|---|---|
| M0: Decisions + Architecture | ✅ Done | Week 1 |
| M1: Core API + Portal + RADIUS flow | ✅ Done | Week 2 |
| M2: ZengaPay live + full payment cycle | ✅ Done (sandbox) | Week 3 |
| M3: Operator dashboard MVP | ✅ Done (2026-06-25) | Week 4–5 |
| M4: Vouchers + cash sessions | ❌ Not started | Week 5–6 |
| M5: SSL + domain + Nairobi prod | ❌ Not started | Week 6 |
| M6: Mobile app (Expo) | ❌ Not started | Week 7–9 |
| M7: Edge agent (Pi / CHR) | ❌ Not started | Week 9–11 |
| M8: Pilot launch — Soroti | ❌ Not started | Week 12 |
