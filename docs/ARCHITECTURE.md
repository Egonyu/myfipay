# System Architecture
## Hotspot Billing Platform

---

## 1. Architecture Overview

The system is built on three tiers:

```
┌─────────────────────────────────────────────────────────────────────┐
│                         CLOUD LAYER (VPS)                            │
│                                                                      │
│  ┌─────────────┐  ┌─────────────┐  ┌───────────────────────────┐   │
│  │   Operator  │  │   Admin /   │  │     Public Captive        │   │
│  │  Dashboard  │  │  Super-Admin│  │        Portal             │   │
│  │  (Next.js)  │  │  (Next.js)  │  │  (Go HTML templates)      │   │
│  └──────┬──────┘  └──────┬──────┘  └─────────────┬─────────────┘   │
│         │                │                         │                 │
│  ┌──────▼────────────────▼─────────────────────────▼─────────────┐  │
│  │                   Go API Server (REST + WebSocket)             │  │
│  │                                                                │  │
│  │  /auth  /sessions  /plans  /vouchers  /payments  /reports     │  │
│  │  /operators  /agents  /devices  /webhooks  /radius-proxy      │  │
│  └──────┬──────────────────────────┬─────────────────────────────┘  │
│         │                          │                                  │
│  ┌──────▼──────┐          ┌────────▼──────┐  ┌──────────────────┐  │
│  │ PostgreSQL  │          │  Redis        │  │  FreeRADIUS      │  │
│  │ (primary)   │          │  Sessions +   │  │  (auth server)   │  │
│  │             │          │  Cache + Queue│  │                  │  │
│  └─────────────┘          └───────────────┘  └────────┬─────────┘  │
│                                                        │ RADIUS     │
│  ┌──────────────────────────────────────────┐          │            │
│  │   ZengaPay Webhook Receiver              │          │            │
│  │   (payment confirmation handler)         │          │            │
│  └──────────────────────────────────────────┘          │            │
└────────────────────────────────────────────────────────┼────────────┘
                                                         │ RADIUS UDP 1812/1813
              ┌──────────────────────────────────────────┤
              │                                          │
    ┌─────────▼──────────┐                   ┌──────────▼──────────┐
    │  EDGE LAYER        │                   │   DIRECT RADIUS     │
    │  (offline-first)   │                   │   (online only)     │
    │                    │                   │                     │
    │  Raspberry Pi /    │                   │  MikroTik / Ubiquiti│
    │  MikroTik CHR      │                   │  TP-Link Omada      │
    │  (Go edge agent)   │                   │  (RADIUS client)    │
    │                    │                   │                     │
    │  - Local RADIUS    │                   └──────────┬──────────┘
    │  - Session cache   │                              │
    │  - Payment queue   │                              │
    └─────────┬──────────┘                              │
              │ 802.11 WiFi                             │ 802.11 WiFi
    ┌─────────▼────────────────────────────────────────▼──────────┐
    │                        END USERS                              │
    │              (captive portal login flow)                      │
    └──────────────────────────────────────────────────────────────┘
```

---

## 2. Component Deep Dives

### 2.1 Go API Server

**Why Go**: Single binary, 10MB RAM idle, handles 50,000 concurrent connections,
native goroutine concurrency maps perfectly to session management,
excellent RADIUS and network library ecosystem, compiles to ARM for edge deployment.

**Responsibilities**:
- REST API for all dashboard and mobile app operations
- WebSocket for real-time session updates and revenue dashboard
- RADIUS proxy (forwards auth to FreeRADIUS, intercepts accounting)
- ZengaPay webhook receiver and payment state machine
- Session lifecycle management (create → active → expired → archived)
- Voucher generation (cryptographically random, collision-checked)
- Rate limiting, authentication (JWT), multi-tenancy

**Key packages**:
- `net/http` + `chi` router (lightweight)
- `layeh/gopher-radius` — RADIUS protocol
- `jackc/pgx` — PostgreSQL driver
- `redis/go-redis`
- `golang-jwt/jwt`
- `skip2/go-qrcode` — QR codes for captive portal
- `jung-kurt/gofpdf` — PDF voucher generation

### 2.2 FreeRADIUS Server

Handles actual authentication decisions. The Go API server reads from PostgreSQL
and writes session/accounting data; FreeRADIUS talks directly to PostgreSQL
via `rlm_sql` module.

**Auth flow**:
```
NAS (router) ──Access-Request──► FreeRADIUS
                                      │ SQL query
                                      ▼
                                 PostgreSQL
                                 (sessions table)
                                      │ match found?
                                      ▼
FreeRADIUS ──Access-Accept──────► NAS (router)
  with: Rate-Limit, Session-Timeout, Idle-Timeout
```

**Accounting flow**:
```
NAS ──Accounting-Request──► FreeRADIUS ──► PostgreSQL
  (Start/Stop/Interim)           │         (usage_records)
                                 └────────► Go API (webhook)
                                           (real-time updates)
```

### 2.3 Edge Agent (Go Binary)

The most differentiated component. Runs on:
- Raspberry Pi 3B+ (Raspbian/Ubuntu)
- MikroTik CHR (x86 VM on existing hardware)
- Any Linux ARM device (Orange Pi, NanoPi)

**Capabilities**:
- Embedded SQLite database for local session cache
- Mini RADIUS server (proxies to cloud, falls back to local)
- Payment queue (stores pending ZengaPay requests, retries on reconnect)
- Captive portal HTML server (serves login page locally)
- Heartbeat to cloud (every 30s)
- Auto-sync on reconnect (delta sync, not full)
- Remote management (SSH tunnel or WireGuard)
- OTA updates from cloud

**Offline authentication logic**:
```
User connects → Captive portal served locally → User pays with MoMo
   │
   ├─ Cloud reachable? → Forward payment to ZengaPay → Grant session → Sync
   │
   └─ Cloud offline?  → Queue payment → Grant session from local cache
                                │
                                └─ On reconnect → reconcile queue → sync
```

### 2.4 Captive Portal

Served by the Go API server (cloud) or edge agent (local).
**Must work on**: Android WebView, iOS Safari, MTK feature phone browsers, UC Browser.
**No JavaScript required** for the core pay + connect flow (progressive enhancement).

**Flow**:
```
1. User connects to WiFi
2. Router intercepts HTTP (walled garden)
3. Browser redirected to captive portal URL
4. Portal shows: hotspot name, plans, payment options
5. User selects plan → enters phone number → taps "Pay with MoMo"
6. ZengaPay pushes USSD prompt to user's phone
7. User confirms on phone (USSD PIN)
8. ZengaPay webhook fires → API grants session
9. Router opens internet access (RADIUS Access-Accept)
10. User's browser auto-redirects to success page
```

### 2.5 Multi-Tenancy Model

```
Platform (Super Admin)
    │
    ├── Agent (reseller) — earns % commission
    │       │
    │       └── Operator (hotspot business)
    │               │
    │               └── Location (hotspot site)
    │                       │
    │                       └── Device (router/edge)
    │                               │
    │                               └── Session (end user)
    │
    └── Operator (direct, no agent)
```

Every request is scoped by `tenant_id`. No data bleeds between tenants.

---

## 3. Data Flow — Payment to Session

```
[User selects 1hr / 1GB plan at 500 UGX]
         │
         ▼
[Captive Portal — POST /api/pay]
  { phone: "0772xxxxxx", plan_id: "...", location_id: "..." }
         │
         ▼
[Go API creates pending_payment record in PostgreSQL]
         │
         ▼
[Go API calls ZengaPay collection API]
  POST https://api.zengapay.com/collections
  { amount: 500, currency: "UGX", phone: "256772xxxxxx" }
         │
         ▼
[ZengaPay sends USSD push to user's phone]
         │
         ▼
[User confirms with MoMo PIN on their phone]
         │
         ▼
[ZengaPay fires webhook → POST /webhooks/zengapay]
  { status: "SUCCESSFUL", reference: "...", amount: 500 }
         │
         ▼
[Go API validates signature, updates payment status]
         │
         ▼
[Go API creates session in PostgreSQL]
  { username: "MAC_address", plan: 1hr/1GB, expires_at: ... }
         │
         ▼
[FreeRADIUS queries sessions table → Access-Accept]
         │
         ▼
[Router opens internet access for user's device]
         │
         ▼
[User's browser redirects to portal success page]
         │
Total time target: < 5 seconds from PIN confirmation
```

---

## 4. Deployment Architecture

```
                    ┌─────────────────────────────────────┐
                    │   DigitalOcean Nairobi (DO-BLR)      │
                    │                                      │
                    │  ┌─────────────┐  ┌───────────────┐ │
                    │  │  App Server │  │  DB Server    │ │
                    │  │  (Go API +  │  │  PostgreSQL   │ │
                    │  │  FreeRADIUS)│  │  Redis        │ │
                    │  │  4GB RAM    │  │  4GB RAM      │ │
                    │  │  2 vCPU     │  │  2 vCPU       │ │
                    │  └──────┬──────┘  └───────────────┘ │
                    │         │                            │
                    │  ┌──────▼──────┐                    │
                    │  │   Nginx     │                    │
                    │  │   (reverse  │                    │
                    │  │   proxy +   │                    │
                    │  │   SSL term) │                    │
                    │  └─────────────┘                    │
                    └──────────────────────────────────────┘
                                    │
                    ┌───────────────┼──────────────────┐
                    │               │                  │
          ┌─────────▼─────┐  ┌─────▼──────┐  ┌───────▼───────┐
          │ Soroti Hotspot │  │Mbale Hotel │  │ Lira School   │
          │                │  │            │  │               │
          │ Raspberry Pi   │  │ Direct     │  │ Edge Agent    │
          │ (edge agent)   │  │ MikroTik   │  │ on CHR VM     │
          │                │  │ RADIUS     │  │               │
          │ MikroTik hAP   │  │ hEX PoE    │  │ RB750Gr3      │
          └────────────────┘  └────────────┘  └───────────────┘
```

**Why Nairobi**: Lowest latency to Uganda, RADIUS UDP timing matters.
DigitalOcean Nairobi or AWS af-south-1 (Cape Town as fallback).

---

## 5. Security Architecture

### 5.1 API Security
- JWT tokens (RS256, 24hr expiry) for operator dashboard
- API keys (SHA-256 hashed) for edge agents
- HMAC-SHA256 signature verification on all ZengaPay webhooks
- Rate limiting: 100 req/min per IP on captive portal, 10 req/min on /pay
- RADIUS shared secret per NAS device (auto-rotated quarterly)

### 5.2 Payment Security
- ZengaPay webhook IP allowlist
- Idempotency keys on all payment requests (prevent double-charge)
- Pending payment timeout (5 minutes — auto-cancelled if no webhook)
- Payment amount server-side validation (never trust client-sent amount)

### 5.3 Multi-Tenancy Isolation
- Row-level security in PostgreSQL (`tenant_id` on every table)
- Edge agents authenticated with per-device signed certificates
- Operators cannot access other operators' data — enforced at DB layer

### 5.4 Captive Portal
- HTTPS enforced (Let's Encrypt)
- HSTS for dashboard domains
- CSP headers on captive portal
- No PII stored beyond phone number (hashed after session expires)

---

## 6. Technology Stack Summary

| Layer | Technology | Rationale |
|---|---|---|
| Core API | Go 1.22 | Performance, concurrency, RADIUS libraries, single binary |
| RADIUS | FreeRADIUS 3.x | Industry standard, MikroTik/Ubiquiti certified, rlm_sql |
| Edge Agent | Go (ARM binary) | Runs on Pi, MikroTik CHR, OpenWRT with musl |
| Operator Dashboard | Next.js 15 (App Router) | Team familiarity, SSR, fast |
| Operator Mobile App | React Native (Expo) | Cross-platform, team familiarity |
| Database | PostgreSQL 16 | ACID, row-level security, JSONB for flexible attribs |
| Cache / Queue | Redis 7 | Session state, job queue, pub/sub for realtime |
| Reverse Proxy | Nginx | SSL termination, static assets, upstream routing |
| Payments | ZengaPay REST API | MTN MoMo + Airtel Money, already contracted |
| SMS | Yo! Uganda / Africa's Talking | Local SMS delivery, low cost |
| Local storage (edge) | SQLite (embedded) | Zero-dependency, works offline |
| Monitoring | Prometheus + Grafana | Custom RADIUS and session metrics |
| CI/CD | GitHub Actions | Build Go binaries, deploy to VPS |
