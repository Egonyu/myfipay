# API Specification
## Hotspot Billing Platform — Go REST API

Base URL: `https://api.<domain>.com/v1`

All authenticated routes require: `Authorization: Bearer <jwt_token>`

---

## Authentication

```
POST   /auth/login              — Operator/agent login → JWT
POST   /auth/refresh            — Refresh JWT token
POST   /auth/logout             — Invalidate token
POST   /auth/forgot-password    — Send OTP to phone/email
POST   /auth/reset-password     — Reset with OTP
```

---

## Captive Portal (Public — No Auth)

```
GET    /portal/:location_slug             — Get location info + plans (renders portal)
POST   /portal/:location_slug/pay         — Initiate payment
         body: { plan_id, phone, method: "mtn_momo"|"airtel_money" }
         returns: { payment_id, status: "pending" }

GET    /portal/:location_slug/pay/:payment_id/status  — Poll payment status
         returns: { status: "pending"|"successful"|"failed" }

POST   /portal/:location_slug/voucher     — Redeem voucher code
         body: { code, mac_address }
         returns: { session_id, expires_at, data_remaining_mb }

GET    /portal/:location_slug/session     — Check session status for MAC
         query: ?mac=<mac_address>
         returns: { active: bool, expires_at, data_used_mb, data_limit_mb }

POST   /portal/:location_slug/topup      — Top up existing session
         body: { plan_id, phone, method }
```

---

## Webhooks (Public — HMAC verified)

```
POST   /webhooks/zengapay          — ZengaPay payment confirmation
         headers: X-ZengaPay-Signature: <hmac>
         body: { reference, status, amount, phone, metadata }
```

---

## Operators

```
GET    /operators/me               — Current operator profile
PUT    /operators/me               — Update profile
GET    /operators/me/stats         — Revenue + session summary
         query: ?from=2026-06-01&to=2026-06-30
```

---

## Locations

```
GET    /locations                  — List my locations
POST   /locations                  — Create location
GET    /locations/:id              — Get location detail
PUT    /locations/:id              — Update location (name, branding, SSID)
DELETE /locations/:id              — Deactivate location

GET    /locations/:id/stats        — Revenue + session stats for location
GET    /locations/:id/sessions     — Active + recent sessions
GET    /locations/:id/devices      — Registered devices
```

---

## Plans

```
GET    /locations/:id/plans        — List plans for location
POST   /locations/:id/plans        — Create plan
         body: { name, price_ugx, duration_mins, data_mb, speed_down_kbps, speed_up_kbps }
PUT    /locations/:id/plans/:plan_id   — Update plan
DELETE /locations/:id/plans/:plan_id   — Delete plan (soft)
```

---

## Devices (Routers)

```
GET    /locations/:id/devices              — List devices
POST   /locations/:id/devices              — Register device
         body: { name, type, nas_ip, nas_identifier }
         returns: { ...device, radius_secret }   — one-time secret reveal

GET    /locations/:id/devices/:device_id/provision  — Get RouterOS provisioning script
DELETE /locations/:id/devices/:device_id            — Remove device
POST   /locations/:id/devices/:device_id/rotate-secret  — Rotate RADIUS secret
```

---

## Sessions

```
GET    /locations/:id/sessions             — List sessions (paginated)
         query: ?status=active&from=&to=&page=1
GET    /sessions/:session_id              — Get session detail
POST   /sessions/:session_id/terminate    — Force-terminate a session
GET    /sessions/export                   — CSV export
```

---

## Vouchers

```
GET    /locations/:id/vouchers             — List vouchers
POST   /locations/:id/vouchers/generate    — Generate voucher batch
         body: { plan_id, quantity, expires_days, batch_note }
         returns: { batch_id, count, vouchers: [{ code, ... }] }

GET    /locations/:id/vouchers/batches     — List batches
GET    /voucher-batches/:batch_id/pdf      — Download printable PDF
GET    /voucher-batches/:batch_id/csv      — Download CSV
POST   /vouchers/:id/void                  — Void a voucher
POST   /vouchers/sms                       — Send voucher code by SMS
         body: { voucher_id, phone }
```

---

## Payments

```
GET    /locations/:id/payments             — Payment history
GET    /payments/:id                       — Payment detail
GET    /payments/export                    — CSV export
GET    /locations/:id/payments/summary     — Totals by method + period
```

---

## Reports

```
GET    /reports/revenue          — Revenue breakdown (daily/weekly/monthly)
GET    /reports/sessions         — Session analytics
GET    /reports/plans            — Plan popularity
GET    /reports/devices          — Device uptime report
GET    /reports/export/pdf       — Full PDF report
```

---

## Agents (Agent role only)

```
GET    /agents/me/operators      — My onboarded operators
POST   /agents/invite-operator   — Send invite to new operator
GET    /agents/me/commissions    — Commission history
GET    /agents/me/commissions/pending  — Unpaid commissions
GET    /agents/me/stats          — Total revenue from my operators
```

---

## Admin (Super Admin only)

```
GET    /admin/tenants             — All operators/agents
POST   /admin/tenants             — Create tenant
PUT    /admin/tenants/:id         — Update tenant (commission %, status)
GET    /admin/transactions        — Platform-wide transactions
GET    /admin/settlements         — Settlement queue
POST   /admin/settlements/:id/pay — Mark settlement as paid
GET    /admin/stats               — Platform KPIs
GET    /admin/devices/offline     — All offline devices
```

---

## Edge Agent API (Internal — API key auth)

```
POST   /edge/register             — Register edge agent on first boot
         headers: X-Device-Secret: <secret>
         body: { device_id, location_id, agent_version, ip }

POST   /edge/heartbeat            — Keep-alive ping
         body: { device_id, active_sessions, uptime_seconds }

GET    /edge/sync                 — Pull delta (new sessions, plan changes)
         query: ?since=<timestamp>
         returns: { sessions: [], plans: [], config: {} }

POST   /edge/sync/sessions        — Push locally-created sessions to cloud
         body: { sessions: [{ username, plan_id, payment_ref, started_at }] }

POST   /edge/payment-queue        — Push queued payments (from offline period)
         body: { payments: [{ phone, amount, plan_id, queued_at }] }
```

---

## Common Response Format

```json
// Success
{
  "success": true,
  "data": { ... },
  "meta": { "page": 1, "per_page": 25, "total": 143 }
}

// Error
{
  "success": false,
  "error": {
    "code": "PAYMENT_FAILED",
    "message": "Mobile money payment was declined",
    "details": { "zengapay_code": "INSUFFICIENT_FUNDS" }
  }
}
```

---

## Error Codes

| Code | HTTP | Description |
|---|---|---|
| `UNAUTHORIZED` | 401 | Missing or invalid JWT |
| `FORBIDDEN` | 403 | Insufficient role permissions |
| `NOT_FOUND` | 404 | Resource does not exist |
| `VALIDATION_ERROR` | 422 | Request body failed validation |
| `PAYMENT_PENDING` | 202 | Payment initiated, awaiting USSD confirmation |
| `PAYMENT_FAILED` | 402 | ZengaPay payment declined |
| `PAYMENT_DUPLICATE` | 409 | Duplicate idempotency key |
| `SESSION_ACTIVE` | 409 | Session already active for this MAC |
| `VOUCHER_INVALID` | 404 | Voucher code not found |
| `VOUCHER_EXPIRED` | 410 | Voucher expired or exhausted |
| `RATE_LIMITED` | 429 | Too many requests |
| `DEVICE_OFFLINE` | 503 | Device not reachable |
| `INTERNAL_ERROR` | 500 | Unexpected server error |
