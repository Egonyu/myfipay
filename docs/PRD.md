# Product Requirements Document (PRD)
## Hotspot Billing Platform — Eastern Uganda

**Version**: 0.1 (Pre-build)
**Date**: June 2026
**Author**: Architecture Phase

---

## 1. Problem Statement

### 1.1 The Market Reality
Eastern Uganda (Soroti, Mbale, Tororo, Jinja, Iganga, Lira corridors) has
thousands of hotspot operators — kiosks, shops, hotels, schools, hospitals,
market stalls — running WiFi on MikroTik routers with no billing system,
or using makeshift voucher books and manual cash collection.

Existing cloud platforms (XenFi, NextFi, Hotspot.ug) are:
- Not offline-resilient: if the upstream internet drops, authentication fails
- Not locally supported: no agent in Mbale, Soroti, or Lira
- Priced in USD or built for Kampala operators
- Generic: no regional identity, language, or payment feel

### 1.2 The Operator Pain Points
1. Manual cash collection → leakage, theft, no records
2. No usage analytics → operators can't price correctly
3. Voucher books run out at 2am, no way to generate more
4. Mobile money collected manually, no reconciliation
5. When internet is down, hotspot stops working entirely
6. Technical setup requires Kampala-based technician (expensive)

### 1.3 The End User Pain Points
1. Have to walk to operator to buy voucher with physical cash
2. Voucher codes scratched off, unreadable
3. No way to check remaining data/time
4. No self-service top-up

---

## 2. Goals

### Primary Goals
- **G1**: Enable any hotspot operator in Eastern Uganda to accept MTN MoMo / Airtel Money payments automatically — zero manual collection
- **G2**: Keep authentication working even when upstream internet fails (offline mode)
- **G3**: Give operators real-time revenue and usage visibility from a smartphone
- **G4**: Build an agent/reseller network that earns commission — organic growth engine

### Secondary Goals
- **G5**: Support every major router brand via RADIUS (no vendor lock-in)
- **G6**: Provide voucher printing with operator branding
- **G7**: Enable school/hospital/hotel-specific plan templates
- **G8**: Support SMS-based session codes for feature phone users

---

## 3. User Personas

### Persona 1 — Sarah, the Kiosk Operator (Primary)
- Runs a small shop in Soroti market with a MikroTik hAP router
- Sells internet access by the hour to market traders
- Collects cash manually, loses ~30% to uncollected vouchers
- Has a smartphone but is not technical
- **Needs**: Self-running payment collection, SMS alerts when someone pays

### Persona 2 — Moses, the School ICT Coordinator
- Manages WiFi at a secondary school in Mbale
- Students buy data bundles weekly
- Needs bandwidth quotas per student
- **Needs**: Pre-paid session plans, student ID login, usage reports for administration

### Persona 3 — Grace, the Hotel Manager
- Runs a mid-range hotel in Jinja
- Gives guests WiFi vouchers on check-in
- Loses voucher books, has no idea how many are used
- **Needs**: Branded vouchers, per-room login, daily revenue report

### Persona 4 — Ivan, the Reseller Agent
- Tech-savvy youth in Lira
- Wants to resell the platform to local businesses and earn commissions
- Deploys and supports hotspot operators in his area
- **Needs**: Agent dashboard, commission tracking, bulk voucher management

### Persona 5 — David, the End User
- Market trader in Tororo
- Buys 1-hour internet to send money and browse
- Pays with MTN MoMo from his phone
- **Needs**: Simple payment on the login page, session timer, balance top-up

---

## 4. Feature Requirements

### Phase 1 — Core (MVP)

#### 4.1 Captive Portal
- [ ] Branded login page (operator logo, name, color)
- [ ] Plan selection (e.g. 1hr/1GB/500 UGX, Daily/5GB/2,000 UGX)
- [ ] MTN MoMo payment via ZengaPay
- [ ] Airtel Money payment via ZengaPay
- [ ] Auto-grant access after confirmed payment (< 5 seconds)
- [ ] Session timer displayed to user
- [ ] Remaining data/time visible without re-logging in
- [ ] "Top up" without re-entering credentials
- [ ] Works on all browsers — mobile-first, no JS requirement for basic flow

#### 4.2 Voucher System
- [ ] Generate voucher codes (alphanumeric, configurable length)
- [ ] Bulk voucher generation (1–500 per batch)
- [ ] Print-ready voucher sheets (A4, thermal 58mm, thermal 80mm)
- [ ] Operator-branded voucher design (logo, hotspot name, wifi name)
- [ ] Voucher expiry (days after generation, or days after first use)
- [ ] Voucher status tracking (unused / active / expired / exhausted)
- [ ] Voucher redemption via captive portal
- [ ] SMS delivery of voucher code to customer phone

#### 4.3 RADIUS Authentication
- [ ] FreeRADIUS integration (cloud-hosted, managed)
- [ ] MikroTik RouterOS RADIUS client support
- [ ] Ubiquiti UniFi RADIUS support
- [ ] TP-Link Omada RADIUS support
- [ ] Generic NAS RADIUS support (any RFC 2865-compliant device)
- [ ] MAC address binding (optional per plan)
- [ ] Simultaneous session limit per plan
- [ ] Bandwidth throttling via RADIUS attributes (rate-limit)
- [ ] Session accounting (start/stop/interim-update)

#### 4.4 Offline Edge Agent
- [ ] Lightweight Go binary deployable on Raspberry Pi or MikroTik CHR
- [ ] Caches active sessions locally (up to 7 days)
- [ ] Authenticates users from local cache when cloud unreachable
- [ ] Syncs with cloud when connection restored
- [ ] Accepts ZengaPay webhooks via local tunnel (or SMS fallback)
- [ ] Heartbeat monitoring — cloud knows when edge is offline

#### 4.5 Operator Dashboard (Web)
- [ ] Revenue summary (today / week / month)
- [ ] Active sessions count (real-time)
- [ ] Session history with search/filter
- [ ] Plan management (create, edit, disable plans)
- [ ] Voucher management (generate, print, void)
- [ ] Payment history (MoMo/Airtel/Voucher breakdown)
- [ ] Connected devices list
- [ ] Bandwidth usage charts
- [ ] Export reports (CSV, PDF)
- [ ] Multiple admin users per location
- [ ] Multi-location support (one account, many hotspots)

#### 4.6 Operator Mobile App (React Native)
- [ ] Revenue at a glance (today's earnings)
- [ ] Active session count
- [ ] Push notification on new payment
- [ ] Generate voucher (emergency, single)
- [ ] Pause / resume hotspot
- [ ] Basic session history

#### 4.7 Payment Integration (ZengaPay)
- [ ] Collection: MTN MoMo (USSD push to customer)
- [ ] Collection: Airtel Money (USSD push to customer)
- [ ] Webhook receiver for payment confirmation
- [ ] Payment retry on failure
- [ ] Receipt generation (SMS + on-screen)
- [ ] Operator payout (settled to operator MoMo weekly/on-demand)
- [ ] Platform revenue share (commission on each transaction)

#### 4.8 SMS Integration
- [ ] Payment confirmation SMS to end user
- [ ] Low balance / session expiry warning SMS
- [ ] Voucher code delivery via SMS
- [ ] Operator SMS alerts (new payment, offline device)
- [ ] Feature phone USSD flow (future — Phase 2)

### Phase 2 — Growth

#### 4.9 Agent / Reseller System
- [ ] Agent accounts with own dashboard
- [ ] Agents onboard operators (referral tracking)
- [ ] Commission per operator revenue (configurable %)
- [ ] Agent payout tracking
- [ ] Bulk voucher purchasing at wholesale price
- [ ] Territory management (agents assigned districts)

#### 4.10 Advanced Plans
- [ ] Time-based (hourly, daily, weekly, monthly)
- [ ] Data-based (MB/GB cap)
- [ ] Combined (time + data, whichever expires first)
- [ ] Social media pass (YouTube-only, WhatsApp-only)
- [ ] Loyalty passes (buy 5 days, get 1 free)
- [ ] Group/family plans (up to N devices on one payment)
- [ ] SSID-segregated plans (premium SSID vs basic SSID)

#### 4.11 Integrations
- [ ] WhatsApp Business (operator support bot)
- [ ] Google Analytics (captive portal traffic)
- [ ] Webhook outbound (notify operator's own systems)
- [ ] ZTE / Huawei router support
- [ ] Starlink router RADIUS support
- [ ] POS printer integration (Bluetooth thermal)

### Phase 3 — Dominance

#### 4.12 Operator Marketplace
- [ ] Operators can sell branded top-up via shared USSD shortcode
- [ ] National roaming (user pays once, connects at any partner hotspot)
- [ ] Ad-supported free tier (operator earns from impressions)
- [ ] Student bundles (partner with schools for subsidised access)

---

## 5. Non-Functional Requirements

| Requirement | Target |
|---|---|
| Captive portal load time | < 2 seconds on 2G |
| Payment confirmation latency | < 5 seconds end-to-end |
| RADIUS auth response time | < 200ms |
| Uptime (cloud) | 99.5% monthly |
| Offline mode duration | Indefinite (cached sessions) |
| Max concurrent sessions per location | 500 |
| Max locations per deployment | Unlimited (multi-tenant) |
| Data retention | 24 months |
| RADIUS packets/sec | 1,000+ |
| API response time (p95) | < 300ms |

---

## 6. Constraints

- ZengaPay is the exclusive payment processor (already contracted)
- Must work with MikroTik RouterOS 6.x and 7.x without custom firmware
- Captive portal must function without JavaScript (for older phones)
- Server must be deployable in Uganda (DigitalOcean Nairobi or AWS Africa Cape Town)
- Edge agent binary must run on Raspberry Pi 3B+ (512MB RAM minimum)
- No dependency on Google Services for core authentication flow (offline risk)

---

## 7. Success Metrics

| Metric | 3 Months | 6 Months | 12 Months |
|---|---|---|---|
| Active operator locations | 50 | 200 | 500 |
| Monthly transactions processed | 10,000 | 50,000 | 200,000 |
| Agent resellers active | 10 | 30 | 80 |
| UGX processed monthly | 5M | 25M | 100M |
| Operator churn rate | < 10% | < 8% | < 5% |
| NPS (operator) | > 40 | > 50 | > 60 |
