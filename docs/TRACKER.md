# myFiBase — Project Tracker

**Repo:** `git@github.com:Egonyu/myfipay.git` (branch `main`)
**Server:** 170.64.177.20 (DigitalOcean Sydney — dev/staging) · **Prod target:** DO Nairobi (BLR1)
**Last updated:** 2026-07-18

> **Rules for this tracker:** update it in the same session as the work; never mark an item ✅ unless it is verified on this server (code present, running, or query-confirmed). Items built elsewhere get ⚠️ until they land here. Security items are never "later" once real money flows.

---

## 1. Milestones

| Milestone | Status | Notes |
|---|---|---|
| M0 Decisions + architecture | ✅ | `docs/` complete |
| M1 Core API + portal + RADIUS | ✅ | Verified end-to-end 2026-06-25 |
| M2 ZengaPay payment cycle | ✅ sandbox | Prod account blocked (shared with TesoTunes) |
| M3 Operator dashboard MVP | ⚠️ **code missing** | Built in preview sandbox 2026-06-25; `dashboard/` on server is **empty** — must be recovered or rebuilt |
| M4 Vouchers + cash sessions | ✅ | Batches, redemption, PDF/QR, cash grant — all in API |
| M4.5 Agent network (API) | ✅ 2026-07-18 | Full API + DB live; UI pending |
| M5 SSL + domain (dev server) | ✅ 2026-07-18 | `https://myfipay.com` live: Cloudflare-proxied A records, certbot SSL, nginx serving `site/` + proxying `/api/`; HTTP→HTTPS redirect on domain; raw-IP HTTP kept for NAS portal + webhooks. Nairobi prod droplet still P3 |
| M5.5 Public site + self-serve onboarding | ⚠️ partial | **Live 2026-07-18:** landing, signup (operator+agent), login (KYC-aware), account stub — verified e2e over HTTPS. **Missing:** real dashboard (M3), router wizard, KYC notifications, ToS/privacy, password reset |
| M6 Mobile app (Expo) | ❌ | Not started |
| M7 Edge agent (Pi/CHR) | ❌ | Not started |
| M8 Pilot launch — Soroti | ❌ | Blocked on: M3 recovery, MikroTik live test, ZengaPay prod, **founder dry-run (§3)** |

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
- [ ] ZengaPay webhook IP allowlist (fires from known IPs; one middleware)
- [ ] `clients.conf` locked to registered NAS IPs + per-device secrets (`devices` table exists, unused)
- [ ] Rotate super-admin password (current one was documented in this repo pre-scrub) + rotate RADIUS secret (same reason)
- [ ] Offsite backups — current backups live on the same disk they protect (DO Spaces, ~5 lines in backup.sh)
- [ ] Fail2ban (SSH is on a public IP) + logrotate (25GB disk)
- [ ] CORS allowlist — currently echoes any Origin with credentials=true; must pin to dashboard origin(s) in prod
- [ ] Login rate limiting / lockout (bcrypt slows brute force but nothing counts failures)
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
| 5. Connect router (**activation**) | `devices` table exists, **unused**; `clients.conf` hand-edited on server | **Biggest gap**: no device self-registration, no per-device RADIUS secret, no generated MikroTik config script, no walled-garden instructions, no connection test. Today this step requires Daniel SSH-ing into the server — consultancy, not SaaS |
| 6. First sale | Pay → webhook → WiFi grant verified end-to-end (sandbox) | ZengaPay prod blocked; no receipt to the WiFi buyer |
| 7. Get paid | Payout request + admin queue APIs | No UI; disbursement is manual mark-paid; 8% platform fee disclosed nowhere except source code — no statement/fee breakdown for the operator |
| 8. Ongoing trust | — | No support channel or help docs; no notifications of any kind |

**Litmus test (gates M8):** founder dry-run — Daniel signs up at myfipay.com as a normal operator and reaches a first paid WiFi session **without SSH-ing into the server**.

---

## 4. Prioritized Backlog

### P0 — blocking pilot (M8)
| # | Task | Owner | Notes |
|---|---|---|---|
| 1 | ~~Push repo to GitHub~~ ✅ | Done | Verified 2026-07-18: `origin/main` matches local HEAD (`89df9e6`); git history confirmed clean of RADIUS secret |
| 2 | Recover or rebuild operator dashboard | Daniel decides | `dashboard/` empty on server; if sandbox code is lost, rebuild from the 16-item feature list (§5 M3) |
| 3 | MikroTik live test | Daniel + Claude | Real router → RADIUS; everything so far is `radtest` only |
| 4 | ZengaPay production account | Daniel | Then: live token + HMAC secret in `.env` |
| 5 | ~~Domain + SSL~~ ✅ | Done | `https://myfipay.com` live 2026-07-18 (Cloudflare proxy + certbot + UFW) |
| 6 | ~~Landing page + signup/login UI~~ ✅ | Done | `site/` live 2026-07-18; signup→KYC gate→login→stats verified e2e over HTTPS. Account page is a stub until dashboard (#2) lands |
| 7 | Router self-onboarding wizard | Claude | Register device in dashboard → per-device RADIUS secret → generated MikroTik setup script → connection test. Uses the dormant `devices` table; also closes the P1 `clients.conf` lockdown security item (§3 stage 5) |
| 8 | Founder dry-run | Daniel | Sign up → first paid session with zero server access. Gates M8 (§3 litmus test) |

### P1 — before real money flows
- [ ] Webhook IP allowlist middleware
- [ ] Rotate admin password + RADIUS secret (pre-scrub exposure)
- [ ] CORS pinned origins (prod mode)
- [ ] Offsite backups to DO Spaces
- [ ] ~~NAS device registration flow~~ → folded into P0 #7 (router self-onboarding wizard)
- [ ] Email delivery infra (SMTP/SES/Resend): KYC approve/reject notification, password reset, receipts — prerequisite for several P2 items and for the KYC flow's "you will be notified" promise
- [ ] ToS + privacy policy + published fee schedule (8% platform, 3% agent) — trust/legal before real money
- [ ] Login attempt rate limiting
- [ ] Unit tests: commission math, payout balance math, webhook dedup, HMAC verify — the money paths
- [ ] Integration test: pay → webhook → session → RADIUS accept
- [ ] Fail2ban + logrotate

### P2 — product completeness
- [ ] Agent dashboard UI (backend shipped 2026-07-18, zero UI)
- [ ] Password reset + email verification (email infra lands in P1)
- [ ] KYC document upload + admin review UI (for pilot, Daniel knows applicants personally — doc upload can wait)
- [ ] Onboarding checklist in dashboard (create plan → connect router → test → go live)
- [ ] SMS/receipt to WiFi buyer on successful payment
- [ ] SMS notifications (Africa's Talking): session start / expiry warning / top-up
- [ ] ZengaPay disbursement API on payout approval (replaces manual mark-paid)
- [ ] Refund handling
- [ ] Session renewal from portal (extend without disconnect — API exists, portal UI doesn't)
- [ ] Multi-device detection (same phone, two devices)
- [ ] Wholesale voucher purchase for agents (5% below retail — BUSINESS_MODEL §Tertiary)
- [ ] Platform settings page (admin)
- [ ] Load test (k6): 100 concurrent portal users

### P3 — scale phase
- [ ] Mobile app (Expo): scaffold, login, home, sessions, manual grant
- [ ] Edge agent (Pi/CHR): mini RADIUS proxy, SQLite cache, payment queue, heartbeat, ARM build, installer, OTA
- [ ] CI/CD (GitHub Actions: build + vet + test on PR)
- [ ] Nairobi prod droplet + separate DB droplet
- [ ] Prometheus/Grafana or Uptime Kuma monitoring

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
| Deploy | `docker compose build api && docker compose up -d api` |
| Migrations | `docker exec -i myfibase_postgres psql -U myfibase -d myfibase < api/db/migrations/NNN_*.sql` |

---

## 8. Session Log (newest first)

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
