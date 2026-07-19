# myFiBase — Project Tracker

**Repo:** `git@github.com:Egonyu/myfipay.git` (branch `main`)
**Server:** 170.64.177.20 (DigitalOcean Sydney — dev/staging) · **Prod target:** DO Nairobi (BLR1)
**Last updated:** 2026-07-19

> **Rules for this tracker:** update it in the same session as the work; never mark an item ✅ unless it is verified on this server (code present, running, or query-confirmed). Items built elsewhere get ⚠️ until they land here. Security items are never "later" once real money flows.
>
> **Binding since 2026-07-19:** `docs/ENGINEERING_STANDARDS.md` — tests + CI on money paths, no live-editing prod, monitoring before pilot, stability before external integrations. Priority order everywhere: correctness systems → security → operability → new features.

---

## 1. Milestones

| Milestone | Status | Notes |
|---|---|---|
| M0 Decisions + architecture | ✅ | `docs/` complete |
| M1 Core API + portal + RADIUS | ✅ | Verified end-to-end 2026-06-25 |
| M2 ZengaPay payment cycle | ✅ sandbox | Prod account blocked (shared with TesoTunes) |
| M3 Operator dashboard MVP | ✅ 2026-07-18 | **Rebuilt** as static SPA at `site/dashboard/` (vanilla JS + cookie JWT, no Node needed on server) — live at `myfipay.com/dashboard/`, all views smoke-tested against the live API. Sandbox Next.js code abandoned; root `dashboard/` dir unused. Restyled 2026-07-19 to "Modernize"-style light theme (inline SVG icons, no external assets) |
| M4 Vouchers + cash sessions | ✅ | Batches, redemption, PDF/QR, cash grant — all in API |
| M4.5 Agent network (API) | ✅ 2026-07-18 | Full API + DB live; UI pending |
| M5 SSL + domain (dev server) | ✅ 2026-07-18 | `https://myfipay.com` live: Cloudflare-proxied A records, certbot SSL, nginx serving `site/` + proxying `/api/`; HTTP→HTTPS redirect on domain; raw-IP HTTP kept for NAS portal + webhooks. Nairobi prod droplet still P3 |
| M5.5 Public site + self-serve onboarding | ⚠️ partial | **Live 2026-07-18:** landing, signup (operator+agent), login (KYC-aware), dashboard (M3), router self-onboarding wizard — verified e2e over HTTPS. **Missing:** KYC notifications, ToS/privacy, password reset |
| M6 Mobile app (Expo) | ❌ | Not started |
| M7 Edge agent (Pi/CHR) | ❌ | Not started |
| M8 Pilot launch — Soroti | ❌ | Blocked on: MikroTik live test, ZengaPay prod, **founder dry-run (§3)** |

---

## 2. Security Posture (payments platform — non-negotiable list)

### In place ✅
- [x] JWT auth (HS256) on all operator/agent/admin routes; role gates `operator` / `agent` / `admin` / `super_admin` (403 verified live)
- [x] bcrypt password hashing; KYC status gate on login
- [x] Webhook HMAC verification (3 header variants, raw+hex secret) — active once `ZENGAPAY_WEBHOOK_SECRET` set
- [x] Webhook replay protection — Redis SetNX on `transactionReference`, 24h
- [x] Commission/payment integrity — commission keyed to exact `payments` row via `RETURNING id`; UNIQUE(payment_id); duplicate webhook cannot double-credit
- [x] Payment rate limiting — 10/IP/5min on `/portal/:slug/pay`
- [x] Balance over-withdrawal guard on payouts (operator and agent), min UGX 5,000
- [x] Tenant isolation — every operator query scoped by `tenant_id` from JWT claims
- [x] Secrets hygiene — `.env` gitignored; credentials scrubbed from repo configs/docs (2026-07-18); `.env.example` template committed
- [x] All services bound to 127.0.0.1 except nginx :80 and SSH
- [x] Daily 2am backups (pg_dump + code, 7-day retention) via cron

### Required before real money (P0/P1 below) ❌
- [x] **UFW active** (verified 2026-07-18: allow 22/80/443 only, v4+v6) — public RADIUS exposure closed at the firewall. `clients.conf` lockdown + secret rotation still open below
- [x] **HTTPS on the domain** — `https://myfipay.com` live (certbot), domain HTTP 301s to HTTPS; auth cookie now `Secure` (deployed + verified 2026-07-18). ⚠️ Captive portal via raw IP is still HTTP until NAS/walled-garden and ZengaPay callback move to the domain
- [x] ZengaPay webhook IP allowlist — middleware live 2026-07-19 (`ZENGAPAY_WEBHOOK_IPS`, empty=disabled like the HMAC secret); ⚠️ set the prod IPs when ZengaPay prod lands
- [x] **`clients.conf` locked down** (verified 2026-07-18): the `0.0.0.0/0` shared-secret client is gone; NAS clients now come from the `nas` table (per-device random secrets, written by the router wizard), UFW opens 1812-1813 only per registered router IP (`radius-sync.sh` cron). Only a localhost test client remains in `clients.conf`. This also closes the RADIUS-secret exposure: the shared secret no longer authenticates any external client
- [x] Rotate super-admin password ✅ 2026-07-18 · RADIUS shared-secret exposure closed via `clients.conf` lockdown above (shared client removed entirely; per-device secrets now)
- [ ] Offsite backups — current backups live on the same disk they protect (DO Spaces, ~5 lines in backup.sh)
- [x] **Fail2ban + logrotate** ✅ verified 2026-07-19: `fail2ban-server` active since Jul 07, sshd jail has 3,497 total bans (26k failed attempts); logrotate configs present incl. freeradius
- [x] CORS allowlist ✅ 2026-07-19 — pinned to `CORS_ALLOWED_ORIGINS` (myfipay.com + www); unpinned origins get no CORS headers
- [x] Login rate limiting / lockout ✅ 2026-07-19 — Redis failure counters, 20/IP + 10/account per 15min, failures only (CGNAT-safe), success heals account counter
- [ ] Session token revocation (Redis denylist) — currently JWTs live 24h with no kill switch

---

## 3. Customer Journey Gaps (walkthrough 2026-07-18)

Framing: treat myFiBase as a pure self-serve billing SaaS — even Daniel signs up like any other operator. Journey: discover → sign up → get approved → set up → connect router → sell → get paid.

| Stage | Exists today | Gap |
|---|---|---|
| 1. Discover (`myfipay.com`) | Nothing — nginx `/` 302s to the **demo captive portal**; no A record yet | **No landing page at all** (pitch, how-it-works, pricing/fees, sign-up CTA). Never tracked before this walkthrough |
| 2. Sign up | `POST /api/auth/register` verified working | No web page to call it (dashboard empty); no email verification; no ToS/privacy to accept; no password reset |
| 3. KYC review | Account lands `pending_kyc`, login blocked; admin approve/reject API | Black hole: nothing to upload, nothing for admin to review against, and the promised "you will be notified" has **no notification mechanism** (zero email/SMS infra in the system) |
| 4. First login / setup | Location, plan, branding, voucher APIs all live | No onboarding UI or checklist ("create plan → connect router → test") |
| 5. Connect router (**activation**) | ✅ **Closed 2026-07-18** — Routers view in dashboard: register device → per-device RADIUS secret → generated MikroTik script + login.html download → connection test. `radius-sync.sh` cron syncs UFW + FreeRADIUS from the `nas` table every minute | Remaining: verify on a real MikroTik (P0 #3) |
| 6. First sale | Pay → webhook → WiFi grant verified end-to-end (sandbox) | ZengaPay prod blocked; no receipt to the WiFi buyer |
| 7. Get paid | Payout request + admin queue APIs | No UI; disbursement is manual mark-paid; 8% platform fee disclosed nowhere except source code — no statement/fee breakdown for the operator |
| 8. Ongoing trust | — | No support channel or help docs; no notifications of any kind |

**Litmus test (gates M8):** founder dry-run — Daniel signs up at myfipay.com as a normal operator and reaches a first paid WiFi session **without SSH-ing into the server**.

---

## 4. Prioritized Backlog

### P0 — correctness & stability systems (reprioritized 2026-07-19 per ENGINEERING_STANDARDS.md; these now come before external integrations)
| # | Task | Owner | Notes |
|---|---|---|---|
| A | ~~Unit tests: commission math, payout balance math, webhook dedup, HMAC verify~~ ✅ | Done | Verified 2026-07-19: 9 tests in `handlers/money_test.go` + `handlers/payment_test.go` (platform/agent commission, operator/agent available balance, commission-rate parsing, published rates, HMAC verify, ZengaPay event classification + payload decoding); `go vet` + `go test ./...` green locally and in CI |
| B | ~~CI: GitHub Actions — `go vet` + `go test` on every push~~ ✅ | Done | Verified 2026-07-19: `.github/workflows/ci.yml` ran on push of `e6072b6`, conclusion `success` (checked via GitHub API) |
| C | ~~Deploy step (git-driven) — stop live-editing prod~~ ✅ | Done | Verified 2026-07-19: `scripts/deploy.sh` (refuses dirty tree; site release symlink swap; api pulls CI-built GHCR image since `01acf30` — 1GB droplet OOMs on on-box builds); used for 2 successful deploys same day, /health + site 200s verified each time |
| D | ~~Uptime + error monitoring~~ ✅ | Done | Fully verified 2026-07-19: watchdog minute-cron live; external probe `uptime.yml` fired on schedule 3× (07:34/08:32/09:25 UTC), all `success` — **P0 A–E complete** |
| E | ~~Integration test: pay → webhook → session grant (ephemeral DB/Redis)~~ ✅ | Done | Verified 2026-07-19: `api/integration/integration_test.go` — pay 202→ZengaPay stub, signed webhook→active session + radcheck/radreply + confirmed payment + 3% commission, dedup, bad-sig 401, status endpoint. PASS locally (`scripts/integration-test.sh`, ephemeral PG+Redis) and in CI (`integration` job, service containers, run `d5a9032` success); GHCR image push now gated on it |

### P0.5 — pilot gate (M8) — after P0 systems are green
| # | Task | Owner | Notes |
|---|---|---|---|
| 1 | ~~Push repo to GitHub~~ ✅ | Done | Verified 2026-07-18: `origin/main` matches local HEAD (`89df9e6`); git history confirmed clean of RADIUS secret |
| 2 | ~~Recover or rebuild operator dashboard~~ ✅ | Done | Rebuilt 2026-07-18 as static app at `/dashboard/`: operator (overview+chart, sessions grant/extend/terminate, plans CRUD, locations+branding, payments, vouchers+print, payouts, settings), agent (invite/operators/commissions/payouts), admin (KYC queue, tenants, revenue, both payout queues, agents) |
| 3 | ~~MikroTik live test~~ ✅ | Done | **Verified 2026-07-19 on real RouterOS 7.16.2** (CHR at 170.64.169.239, kept as lab rig): self-serve registration → cron opened UFW+FreeRADIUS in 40s → wizard script + login.html → captive-portal intercept → branded redirect (mac/ip/link-login-only substituted) → walled garden → **Access-Accept** → Session-Timeout + rate-limit queue applied → accounting Start/Stop rows (after schema fix, migration 006). Remaining at pilot install only: physical router + real phone over WiFi (protocol path identical) |
| 4 | ZengaPay production account | Daniel | Then: live token + HMAC secret in `.env` |
| 5 | ~~Domain + SSL~~ ✅ | Done | `https://myfipay.com` live 2026-07-18 (Cloudflare proxy + certbot + UFW) |
| 6 | ~~Landing page + signup/login UI~~ ✅ | Done | `site/` live 2026-07-18; signup→KYC gate→login→stats verified e2e over HTTPS. Account page is a stub until dashboard (#2) lands |
| 7 | ~~Router self-onboarding wizard~~ ✅ | Done | Live + smoke-tested 2026-07-18: dashboard Routers view → device + `nas` row (per-device secret) → cron `radius-sync.sh` (UFW + FreeRADIUS reload ≤1min) → MikroTik script + login.html download → connection test via `radpostauth.nasipaddress`. Real-router verification folded into #3 |
| 8 | Founder dry-run | Daniel | Sign up → first paid session with zero server access. Gates M8 (§3 litmus test) |

### P1 — before real money flows
- [x] ~~Webhook IP allowlist middleware~~ ✅ 2026-07-19 (env-gated; activate with prod IPs)
- [x] ~~Rotate RADIUS secret~~ ✅ resolved 2026-07-18 via `clients.conf` lockdown — the exposed shared secret's `0.0.0.0/0` client no longer exists; routers use per-device random secrets (— admin password rotated earlier same day)
- [x] ~~CORS pinned origins~~ ✅ 2026-07-19
- [ ] Offsite backups to DO Spaces
- [ ] ~~NAS device registration flow~~ → folded into P0.5 #7 (router self-onboarding wizard)
- [ ] Email delivery infra (SMTP/SES/Resend): KYC approve/reject notification, password reset, receipts — prerequisite for several P2 items and for the KYC flow's "you will be notified" promise
- [ ] ToS + privacy policy + published fee schedule (8% platform, 3% agent) — trust/legal before real money
- [x] ~~Login attempt rate limiting~~ ✅ 2026-07-19
- [ ] ~~Unit tests (money paths)~~ → promoted to P0-A
- [ ] ~~Integration test pay→webhook→session~~ → promoted to P0-E
- [ ] Dashboard XSS audit — review every innerHTML interpolation for esc() coverage
- [x] ~~`radius-sync.sh` reload; remove Adminer~~ ✅ 2026-07-19 — Adminer container+image removed from box and compose. Reload is **not possible**: live HUP test showed FreeRADIUS 3.2 ignores SQL client changes ("HUP - No files changed"); restart kept (hash-gated, NAS retransmits) with a `freeradius -C` pre-check so a bad config can't become an outage. Revisit dynamic_clients at scale
- [x] ~~Fail2ban + logrotate~~ ✅ verified 2026-07-19 (see §2)

### P2 — product completeness
- [ ] Agent dashboard UI (backend shipped 2026-07-18, zero UI)
- [ ] Password reset + email verification (email infra lands in P1)
- [ ] KYC document upload + admin review UI (for pilot, Daniel knows applicants personally — doc upload can wait)
- [ ] Onboarding checklist in dashboard (create plan → connect router → test → go live)
- [ ] SMS/receipt to WiFi buyer on successful payment
- [ ] SMS notifications (Africa's Talking): session start / expiry warning / top-up
- [ ] ZengaPay disbursement API on payout approval (replaces manual mark-paid)
- [ ] Refund handling
- [ ] RADIUS CoA/Disconnect on session terminate — dashboard terminate kills authorization but the live hotspot session rides out its clock (verified on CHR 2026-07-19); routers already accept incoming RADIUS (wizard sets `/radius incoming accept=yes`, port 3799)
- [ ] Session renewal from portal (extend without disconnect — API exists, portal UI doesn't)
- [ ] Multi-device detection (same phone, two devices)
- [ ] Wholesale voucher purchase for agents (5% below retail — BUSINESS_MODEL §Tertiary)
- [ ] Platform settings page (admin)
- [ ] Load test (k6): 100 concurrent portal users

### P3 — scale phase
- [ ] Mobile app (Expo): scaffold, login, home, sessions, manual grant
- [ ] Edge agent (Pi/CHR): mini RADIUS proxy, SQLite cache, payment queue, heartbeat, ARM build, installer, OTA
- [ ] ~~CI/CD~~ → promoted to P0-B (2026-07-19)
- [ ] Nairobi prod droplet + separate DB droplet
- [ ] ~~Monitoring~~ → promoted to P0-D (2026-07-19); Prometheus/Grafana remains here for scale phase

---

## 5. Verified End-to-End (evidence log)

| Test | Result | Date |
|---|---|---|
| Full flow: pay → webhook → WiFi grant | ✅ | 2026-06-25 |
| ZengaPay sandbox collection + webhook (from 188.245.65.108) | ✅ | 2026-06-25 |
| Webhook dedup (SetNX) — duplicate returns 200, no re-process | ✅ | 2026-06-25 |
| RADIUS accept/reject via rlm_sql → PostgreSQL | ✅ | 2026-06-25 |
| `radtest` provisioned phone → Access-Accept + bandwidth attrs | ✅ | 2026-06-25 |
| Portal `/portal/demo/` renders, plans from DB | ✅ | 2026-06-25 |
| Agent register → login → invite → operator signup with code → referral row | ✅ | 2026-07-18 |
| Agent JWT hitting admin route → 403 | ✅ | 2026-07-18 |
| Migrations 001–004 applied, schema matches code (mac_address drift resolved) | ✅ | 2026-07-18 |
| API rebuilt + redeployed, `/health` OK | ✅ | 2026-07-18 |
| `https://myfipay.com` — landing/signup/login/assets all 200 via Cloudflare | ✅ | 2026-07-18 |
| Live e2e: signup → login blocked `PENDING_KYC` → DB approve → login sets `Secure` cookie → `/api/auth/me` + `/api/dashboard/stats` OK | ✅ | 2026-07-18 |
| UFW active (22/80/443 only); raw-IP HTTP portal still serves for NAS | ✅ | 2026-07-18 |
| Dashboard live e2e (operator): create location → plan → grant session → extend → terminate → voucher batch → branding → payments/balance/stats | ✅ | 2026-07-18 |
| Dashboard live e2e (admin): KYC queue lists pending signup → approve via API → operator login succeeds | ✅ | 2026-07-18 |
| Dashboard live e2e (agent): register → all 5 agent endpoints return; invite URL now myfipay.com/signup?agent= | ✅ | 2026-07-18 |
| Bug fix verified: revenue-chart `day` was empty (DATE→string scan swallowed); now returns ISO dates | ✅ | 2026-07-18 |
| Router onboarding e2e: register device via API → `nas` row → cron sync adds UFW rule + FreeRADIUS logs "Adding client 203.0.113.10 (mfb-…)" → script/status endpoints OK → delete → UFW rule + rows removed | ✅ | 2026-07-18 |
| `radpostauth.nasipaddress` populated by patched postauth query (radtest → row shows `127.0.0.1`) | ✅ | 2026-07-18 |
| Portal `?login=` (MikroTik `$(link-login-only)`) rendered into page; `javascript:` scheme rejected | ✅ | 2026-07-18 |
| Money-path unit tests: `go vet` + `go test ./...` green locally; CI run on `e6072b6` (push to main) concluded `success` | ✅ | 2026-07-19 |
| API rebuilt + redeployed from committed `e6072b6` (not working tree), container recreated, `/health` OK | ✅ | 2026-07-19 |
| Uptime probe `uptime.yml` fired on GitHub cron 3× (07:34/08:32/09:25 UTC), all success — P0-D closed | ✅ | 2026-07-19 |
| FreeRADIUS HUP experiment: SQL client inserted, reload → daemon "HUP - No files changed. Ignoring"; restart loads it — reload cannot sync NAS clients | ✅ | 2026-07-19 |
| Live: CORS pinned — `Origin: https://myfipay.com` gets ACAO, `evil.example` gets zero CORS headers | ✅ | 2026-07-19 |
| Live: login lockout — 10 failures on one account → 11th attempt 429 (Retry-After 900); test keys cleaned | ✅ | 2026-07-19 |
| Live: webhook endpoint unaffected with `ZENGAPAY_WEBHOOK_IPS` empty (no 403) | ✅ | 2026-07-19 |
| Deploy `a2e5c14` via CI-pulled image: /health + site + /dashboard/ + /login all 200 | ✅ | 2026-07-19 |
| **MikroTik live e2e (CHR, RouterOS 7.16.2)**: dashboard-style registration → UFW+FreeRADIUS client in 40s → hotspot intercept 302 → branded login.html (vars substituted) → walled garden to portal (200 via Cloudflare) → grant via API → hotspot login **Access-Accept** (radpostauth nasip=CHR) → dynamic queue 2048k/1024k + 59m timeout → authenticated browsing OK | ✅ | 2026-07-19 |
| radacct accounting Start (open row) + Stop (closed, terminate cause) from real NAS after migration 006 | ✅ | 2026-07-19 |
| Dashboard terminate on live session: radcheck removed instantly, hotspot session persists (no CoA) — recorded as P2 | ✅ | 2026-07-19 |

---

## 6. Completed Work (reference)

<details><summary><b>M0 — Planning & architecture</b></summary>

DECISIONS, ARCHITECTURE, PRD, DATABASE_SCHEMA, API_SPEC, BUSINESS_MODEL, HARDWARE_COMPAT, OPEN_QUESTIONS — all locked. ZengaPay real rates + XenFi competitive analysis documented.
</details>

<details><summary><b>M1/M2 — Core API, portal, payments, RADIUS</b></summary>

- Go 1.25 + chi + pgx + go-redis; graceful shutdown; Docker multi-stage (~12MB)
- Portal: server-rendered, no-JS core flow, JS status polling, white-label branding (per-location name/tagline/color/logo via `locations.branding` JSONB), plans from DB, MAC/IP captured from router redirect
- Payments: initiate → Redis pending (10min TTL) → ZengaPay collection → webhook (HMAC + dedup) → session grant; idempotency keys; dev mode simulation; cash + mobile-money persisted to `payments` table
- RADIUS: radcheck `Auth-Type := Accept`, radreply Session-Timeout/Idle-Timeout/Mikrotik-Rate-Limit/WISPr; per-plan bandwidth; session reaper (60s DB poll) replaces restart-lost goroutines; radacct bandwidth surfaced in stats
- Vouchers: batch generation, redemption endpoint (full RADIUS grant), QR + A4/thermal PDF export
</details>

<details><summary><b>M3 — Operator dashboard (⚠️ built in sandbox, NOT on this server)</b></summary>

Claimed complete 2026-06-25 in a preview sandbox: Next.js 15 + Tailwind v4 + NextAuth v4, dark Greeva theme; pages: login, overview (ApexCharts), sessions (filter/terminate), plans CRUD, locations + branding modal, payments, payouts + admin payout queue, vouchers + PDF, settings, admin (tenants/revenue/KYC). **Code never landed in this repo — recover or rebuild.**
</details>

<details><summary><b>M4.5 — Agent network API (2026-07-18)</b></summary>

- Migration `004_agent_network.sql`: `agent_referrals` (UNIQUE operator), `commissions` (UNIQUE payment_id, rate_pct), `payout_requests`
- `POST /api/auth/register/agent` → tenant type `agent`, invite code = slug
- Operator registration accepts `agent_code` → referral row
- Agent API (`RequireRole("agent")`): dashboard, invite, operators, commissions, payouts request/list
- Commission: 3% (`agentCommissionRate` const) on confirmed mobile-money payments, keyed to exact payment row via `RETURNING id` — concurrency- and replay-safe
- Admin: agents list, agent-payouts queue (approve → paid | reject)
- Operator settlement (separate `payouts` table, 003): balance = mobile-money × (1−8%) − requested; admin queue approve/reject/mark-paid
</details>

---

## 7. Server Reference — 170.64.177.20

| Layer | Detail |
|---|---|
| Host | Ubuntu 24.04, 1 vCPU / 1GB / 25GB, `ssh myfibase` (root, key auth) |
| Native | nginx :80 (proxies `/portal/` `/api/` `/webhooks/`, `/` → 302 demo); FreeRADIUS 3.2.5 UDP 1812/1813 (rlm_sql → 127.0.0.1:5432, EAP off, hotspot vhost) |
| Docker | `myfibase_api` :8080, `myfibase_postgres` :5432, `myfibase_redis` :6379, `myfibase_adminer` :8081 — all loopback-bound |
| DB | `myfibase`/`myfibase`, migrations 001–004 applied |
| Env | `.env` (gitignored): ZengaPay sandbox URL + token wired; webhook secret empty (sandbox sends none) |
| Backups | cron 2am daily → `backups/` (pg_dump + code, keep 7) — **local only** |
| Deploy | `scripts/deploy.sh` — site: git-archive → release symlink; api: pulls CI-built `ghcr.io/egonyu/myfipay/api:<sha>` (droplet is 1GB, on-box Go builds get OOM-killed; `--build` = emergency fallback) |
| Migrations | `docker exec -i myfibase_postgres psql -U myfibase -d myfibase < api/db/migrations/NNN_*.sql` |

---

## 8. Session Log (newest first)

### 2026-07-19 (afternoon) — MikroTik live test PASSED on rebuilt CHR
- Daniel rebuilt the CHR droplet (170.64.169.239, RouterOS 7.16.2); networking had to be set via DO console (no DHCP on DO — this is what killed attempt #1). Claude drove everything after via SSH
- **P0.5 #3 done.** Full journey verified against real RouterOS: register router (temp operator `chr-test`, product API path) → radius-sync opened UFW + loaded FreeRADIUS client in 40s → wizard script applied → hotspot (VXLAN lab: droplets are in different VPCs, so an L2 VXLAN tunnel over public makes this server a hotspot client) → intercept → branded login.html → walled garden → grant → **Access-Accept** → queue/timeout applied → real browsing; dashboard device status `online:true`
- **Bug found+fixed (migration `006`)**: radacct schema diverged from stock FreeRADIUS 3.2 — missing IPv6/connectinfo columns + 7 over-strict NOT NULLs made **every accounting packet fail** (invisible to radtest, which never sends accounting). Applied live; Start/Stop rows now land
- Recorded limitation → P2: no CoA/Disconnect on terminate (session rides out clock after auth revoked)
- CHR hardened: unused services disabled, SSH/Winbox restricted to mgmt address-list (server + Daniel's IPs); DO console is the recovery path. CHR kept as permanent lab rig with the `chr-test` tenant/device/plan and the vx1 tunnel (UFW rule `chr-vxlan-test`)
- M8 pilot now blocked only on: ZengaPay prod account (#4) + founder dry-run (#8)

### 2026-07-19 (midday) — P0 closed; P1 security batch shipped (a2e5c14)
- **Founder SSH timeout diagnosed:** `170.64.169.239` is the separate **MikroTik CHR test droplet** (not this server, which is `170.64.177.20`) — the CHR install died mid-conversion; Daniel is rebuilding it. Rebuild + live-test runbook written: `docs/MIKROTIK_CHR_TEST.md`. This server's SSH verified clean either way (sshd active, ufw 22 open, no fail2ban bans on his IPs)
- P0-D closed: `uptime.yml` fired on GitHub's cron 3× today, all success → **P0 A–E all verified done**
- P1 security batch (committed `a2e5c14`, CI-built image deployed):
  - Login lockout: Redis failure counters (20/IP, 10/acct per 15min, failures only, success heals acct); unit-tested
  - CORS pinned via `CORS_ALLOWED_ORIGINS` (unpinned origins get no headers; dev echo only with zero pins + development env)
  - Webhook source-IP allowlist middleware (`ZENGAPAY_WEBHOOK_IPS`, empty=disabled) — activate when ZengaPay prod IPs known
  - **Spoofing fix**: new `middleware.ClientIP` trusts only nginx-set `X-Real-IP`; payment rate limiter previously trusted the client-appendable first XFF element (forgeable to evade the 10/5min limit)
- Adminer removed (container + image + compose block) — P1 exposure closed; psql via docker exec documented in compose
- `radius-sync.sh`: live HUP experiment proved reload cannot pick up SQL clients ("HUP - No files changed. Ignoring" from MainPID; the journal "Adding client" lines were the `-C` checker process) → restart kept, documented, plus `freeradius -C` gate before restart
- Remaining P1 (need external inputs): offsite backups (DO Spaces creds), email infra (provider choice), ToS/privacy/fee pages, dashboard XSS audit

### 2026-07-19 (evening) — P0-E integration test shipped; P0-C verified; P0-D watchdog cron installed
- P0-E done: `api/integration/integration_test.go` (build tag `integration`) exercises the real handler stack against ephemeral Postgres+Redis — pay→webhook→session grant, RADIUS rows, confirmed payment, 3% agent commission, webhook dedup, bad-signature 401, status endpoint. Green locally (2s) and in CI (`d5a9032`)
- CI `integration` job added (PG+Redis service containers); **GHCR image push now requires unit + integration tests green** — unmergeable money-path breakage can't become a deployable image
- `scripts/integration-test.sh` for local runs: loopback high ports (55432/56379), tmpfs postgres, auto-teardown, `GOFLAGS=-p=1 GOMEMLIMIT=200MiB` so the 1GB box survives the compile
- P0-D gap from the OOM-killed session closed: watchdog minute-cron was never installed — now installed and test-run verified; uptime.yml awaiting first scheduled fire on GitHub
- Deployed `d5a9032` via pull-based deploy; /health + site 200s verified. P0 A–E now all shipped; only uptime.yml first-fire check remains

### 2026-07-19 (later still) — OOM diagnosis; deploys now pull CI-built images
- Repeated "Killed" session crashes diagnosed via kernel log: Linux OOM killer — 1GB droplet can't hold Claude Code + dockerd + a Go compile (`compile` in the deploy's docker build was killed at 04:44, `claude` at 04:59 and 05:42). Stale VS Code server killed by founder freed ~700Mi swap
- Fix: CI now builds and pushes `ghcr.io/egonyu/myfipay/api:<sha>` + `:latest` on every main push (`ci.yml` `image` job, needs tests green); `deploy.sh` pulls that image (waits up to 10min for CI) instead of compiling on-box; `--build` kept as emergency fallback; keeps 3 image tags for rollback
- **One-time manual step**: after the first CI image push, set the GHCR package `myfipay/api` to public (repo is public; anonymous pull then needs no PAT on the droplet)
- Noted: `myfibase_adminer` container up 3 weeks on prod — trivial memory but a security exposure; stop when not in active use (security posture is blocking)

### 2026-07-19 (later) — Engineering standards adopted; P0-A tests + P0-B CI shipped
- `docs/ENGINEERING_STANDARDS.md` written and made binding (tests+CI on money paths, no live-editing prod, monitoring before pilot, stability before MikroTik/ZengaPay); backlog reprioritized — new P0 is correctness/stability systems (A–E), old P0 items moved to P0.5 pilot gate
- Money logic extracted from `handlers/agent.go`/`payment.go` into `handlers/money.go` (pure functions) so it's testable without DB; behavior unchanged
- 9 unit tests (`money_test.go`, `payment_test.go`): platform/agent commission, operator/agent available balance, commission-rate parsing, published 8%/3% rates, HMAC verify, ZengaPay event classification + payload decoding — `go vet` + `go test ./...` green
- CI: `.github/workflows/ci.yml` (vet + test on every push) — first run on `e6072b6` concluded `success` (verified via GitHub API; `gh` CLI not installed on droplet)
- Deployed per the new standard: committed first, then rebuilt image from the committed tree; container recreated, `/health` OK (initial deploy attempt was cut off by a session break — container was still 12h old; caught and redone this session)
- P0 remaining: C (git-driven deploy step), D (uptime/error monitoring), E (integration test pay→webhook→session)

### 2026-07-19 — Dashboard visual restyle finished ("Modernize" theme)
- Picked up yesterday's uncommitted restyle of the dashboard toward the Modernize admin-template look (reference mockups in `dashboard/template/`, gitignored — not committed): CSS token remap scoped to `.dash-body` (blue `#5d87ff` brand, soft shadows, pastel accent palette; public site keeps green), stat tiles with pastel icon chips, welcome banner, card shadows, restyled tables/pills/modals — that part was already done and live
- Finished the two remaining pieces: sidebar nav now renders per-route inline SVG feather icons with section labels (Dashboard/Manage/Money/Account; admin regrouped Platform/Money); sidebar footer now avatar-initial + name + role chip. All icons inline SVG — zero external assets, still no build step
- Cache-busters bumped (`dashboard.css?v=3`, `dashboard.js?v=4`); `node --check` (via throwaway node:alpine container) passes; live URLs verified 200 serving the new code
- No API/backend changes; committed site + tracker
- Built + deployed across two sessions (context break mid-way; second session verified everything live rather than re-building): migration `005` (`nas` table + `radpostauth.nasipaddress`), `handlers/device.go` (CRUD + MikroTik script + connection test, tenant-scoped, platform-wide IP uniqueness), dashboard **Routers** view (add/edit/remove, setup modal with copy-paste RouterOS script + `login.html` download, connection test), `scripts/radius-sync.sh` (cron every minute: UFW per-router allow rules tagged `myfibase-nas` + FreeRADIUS restart, hash-gated no-op)
- Host config (mirrored into `freeradius/` in repo, secrets scrubbed, + new `freeradius/README.md`): `clients.conf` reduced to localhost-only — **the `0.0.0.0/0` shared-secret client is gone**; `mods-enabled/sql` `read_clients=yes` from `nas` table; `queries.conf` postauth patched to record packet source IP
- Portal: accepts MikroTik `$(link-login-only)` as `?login=` (scheme-validated) and, after payment/voucher, logs the device into the hotspot via RADIUS instead of bouncing to google.com; voucher redemption now sends phone+MAC
- Smoke-tested live end-to-end (temp operator in demo tenant, removed after): register 203.0.113.10 → cron picked it up in <1min (UFW rule + FreeRADIUS "Adding client"), script/status endpoints, delete → full cleanup. DB back to 2 tenants / 2 users / 0 devices
- Security items closed: `clients.conf` lockdown + RADIUS shared-secret exposure (P1)
- Founder dry-run now blocked only on: real MikroTik test (P0 #3), ZengaPay prod (P0 #4)

### 2026-07-18 (late night) — Operator dashboard rebuilt and live
- M3 rebuilt from scratch as a static SPA (`site/dashboard/` + `assets/dashboard.js/.css`) instead of the lost sandbox Next.js app — no Node runtime needed on the 1GB droplet, served by existing nginx `site/` root, cookie-JWT auth against the existing API
- Role-aware views — operator: overview + 30-day SVG revenue chart, sessions (filter/grant/extend/terminate), plans CRUD, locations + branding, payments, vouchers (create/view/print sheet), payouts (balance + request + history), settings (profile/password). Agent: overview, invite link + copy, operators, commissions, payouts. Admin: KYC queue (approve/reject), tenants, platform revenue, operator payout queue (approve/reject/mark-paid), agents + agent payout queue
- Login now redirects to `/dashboard/`; `/account` 301-style redirects there (stub retired)
- **3 API bugs found by smoke testing and fixed:** (1) KYC queue always returned empty — `AppliedAt string` scanning a timestamptz made every `rows.Scan` fail silently; (2) same DATE→string scan bug zeroed `revenue-chart` days (operator + admin); (3) stale hardcoded URLs — agent invite pointed at dead `myfibase.ug`, CreateLocation returned raw-IP portal URL
- Everything smoke-tested live over HTTPS as all three roles (see evidence log); temp admin user created via pgcrypto bcrypt and removed after; all test tenants/users/sessions/RADIUS rows cleaned — DB back to 2 demo tenants + super_admin
- Remaining for founder dry-run: router self-onboarding wizard (P0 #7), MikroTik live test, ZengaPay prod

### 2026-07-18 (night) — Site + SSL live; session continuity catch-up
- Found substantial work from the previous session that was live but uncommitted and untracked: `site/` (landing, signup, login, account stub), certbot SSL nginx config for `myfipay.com`, UFW enabled, `Secure` cookie flag in `auth.go`
- Verified live: Cloudflare-proxied A records resolve, `https://myfipay.com` serves the site (200 on /, /signup, /assets/style.css); UFW active with 22/80/443 only; raw-IP HTTP portal (`/portal/demo/`) still 200 for the NAS walled-garden path
- Deployed the `Secure` cookie change (API container was still running the old build) and verified full flow live: signup → `PENDING_KYC` login block → DB approve → login (cookie now `HttpOnly; Secure; SameSite=Lax`) → `/api/auth/me` → `/api/dashboard/stats`
- Cleaned 3 test tenants out of the DB (`conn-test`, `tracker-test-wifi`, `-2`); back to the 2 demo tenants
- Synced repo `nginx/conf.d/myfibase.conf` with the live certbot config; committed site + auth + nginx + tracker
- Tracker updated: M5 ✅, M5.5 partial, UFW + HTTPS security items closed, P0 #5/#6 done

### 2026-07-18 (evening) — Customer-journey gap walkthrough
- Walked the product end-to-end as a fresh self-serve customer (landing → signup → KYC → setup → router → sale → payout); added §3 journey gap table
- Biggest finds: **no landing page anywhere in the plan** (root 302s to demo portal); **router onboarding requires SSH to the server** (`devices` table unused); KYC promises a notification the system cannot send (no email/SMS infra exists); no ToS/privacy/fee disclosure
- Backlog: P0 + landing/signup UI (#6), router self-onboarding wizard (#7), founder dry-run gate for M8 (#8); P1 + email infra, legal pages; NAS lockdown folded into wizard; new milestone M5.5

### 2026-07-18 (later) — Verification audit; domain purchased
- `myfipay.com` purchased on Cloudflare (NS live: lady/dane.ns.cloudflare.com); **no A records yet** — M5 unblocked
- GitHub push confirmed complete: `origin/main` == local `89df9e6`; searched full git history — RADIUS secret never committed
- **Found: UFW inactive** (tracker had claimed active) while `clients.conf` allows `0.0.0.0/0` with the pre-scrub RADIUS secret and 1812/1813 bound publicly — escalated to top of pre-money security list
- Verified live: all 4 containers up (API 4h, DB/Redis/Adminer 3wk healthy), `/health` OK, nginx + FreeRADIUS active, portal `/portal/demo/` 200, daily backups current (last: 2026-07-18 02:00), migrations schema present (20 tables). `dashboard/` and `edge-agent/` confirmed still empty
- DB state: 2 tenants, 1 location, 3 plans, 1 payment, 2 sessions, 0 vouchers/commissions/referrals

### 2026-07-18 — Audit, agent network deploy, repo secured
- Full audit against live DB (not tracker claims). Found: `dashboard/` + `edge-agent/` **empty** despite M3 "complete"; `sessions.mac_address` live-only ALTER (no migration); duplicate `003_` migration numbers; no git remote, 24 files uncommitted
- Fixed: migration renumber (agent → `004`), `mac_address` folded into 001, `004` applied to live DB, API rebuilt + redeployed, `/health` OK
- Agent network smoke-tested live end-to-end (register → invite → referral → 403 role gates)
- Commission race fixed: payment row via `RETURNING id`; rate to single const; payout floor shared with operator payouts (`minPayoutUGX`)
- Repo secured: credentials scrubbed (RADIUS secret, admin password, ZengaPay key prefix), README + `.env.example` written, all work committed on `main`; deploy key generated — **push pending Daniel adding key to GitHub**

### 2026-06-25 — M3/M4 sprint (dashboard in sandbox)
- White-label branding, KYC flow, admin panel, payouts, payments-to-DB, session reaper, voucher PDF/QR, radacct in dashboard
- Dashboard pages built in preview sandbox (NextAuth) — never deployed to server

### 2026-06-25 (earlier) — M0–M2
- Docs locked; API + portal + ZengaPay + FreeRADIUS wired; full flow verified end-to-end
