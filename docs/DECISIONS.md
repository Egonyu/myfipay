# Locked Decisions
## myFiBase — Hotspot Billing Platform

All open questions answered. This document is the source of truth for build decisions.

---

## Identity

| Decision | Value |
|---|---|
| **Product name** | myFiBase |
| **Domain** | myfibase.ug (primary) + myfibase.com (redirect) |
| **White-label** | Yes — from Phase 1. Operators set their own logo, colors, portal name |

---

## Infrastructure

| Decision | Value |
|---|---|
| **Hosting** | DigitalOcean — dedicated droplets for myFiBase only |
| **Region** | Nairobi (BLR1) — lowest latency to Uganda |
| **Self-managed** | Yes — owner is comfortable with Linux operations |
| **Initial server spec** | 2x droplets: App (4GB/2vCPU), DB (4GB/2vCPU) |

---

## Technology

| Decision | Value |
|---|---|
| **Captive portal** | Custom-built (Go templates) — not CoovaChilli |
| **Billing UI** | Built from ground up (Next.js) — not any existing billing UI |
| **RADIUS** | FreeRADIUS (non-negotiable standard) |
| **Backend** | Go (not Laravel, not PHP) |

---

## Payments — ZengaPay Actual Rates

### Collections (incoming from customers)
| Range (UGX) | Charge |
|---|---|
| 500 – 5,000 | UGX 200 flat |
| 5,001 – 25,000 | UGX 600 flat |
| 25,001 – 5,000,000 | 2% + 2% standard network charge = ~4% effective |

### Disbursements (payouts to operators)
| Range (UGX) | Charge |
|---|---|
| 500 – 5,000 | UGX 200 flat |
| 5,001 – 25,000 | UGX 600 flat |
| 25,001+ | UGX 800 flat + UGX 1,200 network = UGX 2,000 per payout |

### Bank settlements
| Method | Cost |
|---|---|
| EFT | UGX 20,000 |
| RTGS | UGX 30,000 |

### Margin recalculation with real rates
- Platform commission: 8% (starter) to 4% (enterprise)
- ZengaPay collection cost on a 500 UGX transaction: UGX 200 = **40% cost** on micro-transactions
- ZengaPay collection cost on a 2,000 UGX transaction: UGX 600 = **30% cost** — still high
- ZengaPay collection cost on a 10,000 UGX transaction: ~4% = manageable
- **Implication**: Micro-transactions (< 2,000 UGX) are margin-negative if ZengaPay flat fees apply
- **Mitigation**: Minimum plan price set at 2,000 UGX (daily plan), or absorb flat fee on low plans and compensate on bundles

---

## Operator Settlements

| Decision | Value |
|---|---|
| **Method** | Option A (manual request) as default + Option B (automatic weekly) for premium tier operators |
| **Minimum withdrawal** | 50,000 UGX base; package-based tiers can have different minimums |
| **Payout method** | MTN MoMo / Airtel Money disbursement via ZengaPay |
| **Disbursement cost** | UGX 2,000 per payout (absorbed by platform or deducted from settlement) |

---

## Payments — Cash Support

| Decision | Value |
|---|---|
| **Cash payments** | Yes — operators can manually grant sessions for cash |
| **MoMo** | Primary (preferred) |
| **Cash** | Secondary — recorded as "cash" payment type, no ZengaPay call |
| **Trust model** | Cash sessions are auditable (admin can see cash vs MoMo ratio per operator) |

---

## Offline Resilience

| Decision | Value |
|---|---|
| **Max offline duration** | 48 hours — edge agent grants sessions from local cache up to 48h without cloud |
| **After 48h** | New sessions rejected; renewals only for cached users |
| **Payment latency target** | < 10 seconds from MoMo PIN confirmation to WiFi access |

---

## Operator & User Experience

| Decision | Value |
|---|---|
| **Personas supported** | Both: technical operators (web dashboard) AND non-technical (mobile app) |
| **Router setup model** | Tier-based: basic tier = you/agent sets up on-site; premium tier = operator self-serves via dashboard script |
| **Multi-location** | Supported from Phase 1; UI complexity tied to operator tier |
| **Top-up without reconnect** | Yes — operators can extend active sessions from dashboard; end-user SMS top-up in Phase 2 |
| **Portal language** | English only — Ateso/Lugisu language support in Phase 3 |

---

## User Identity

| Decision | Value |
|---|---|
| **Primary identity** | Phone number (same phone = same identity across sessions) |
| **RADIUS username** | Phone number (used as RADIUS User-Name attribute) |
| **MAC binding** | Secondary — optional device binding per phone number |
| **Benefit** | User can switch devices and retain session history; family plans possible |

---

## Agent Network

| Decision | Value |
|---|---|
| **"Agent" definition** | Field reseller — a person who onboards operators in a district |
| **First target locations** | Soroti, Mbale, Lira, Tororo — hotels, sports venues, markets, schools, hospitals |
| **Agent payout** | Option A (weekly MoMo disbursement) primary; Option B (dashboard balance withdrawal) also supported |
| **Agent dashboard** | Yes — agents get their own login with operator list, commission tracker, invite tool |

---

## Legal & Compliance

| Decision | Value |
|---|---|
| **UCC license** | Not required for MVP — pursue when growth demands it |
| **Data retention** | 90 days for session logs; phone numbers hashed after 90 days |
| **KYC** | Handled by myFiBase (not delegated to ZengaPay) |
| **KYC docs needed** | National ID + business registration (or LC1 letter for informal operators) |

---

## Timeline & Resources

| Decision | Value |
|---|---|
| **MVP target** | 3 months from build start |
| **Team** | Solo developer (owner) |
| **Budget** | UGX 2,000,000 (~$540 USD) — raised within 2 months |
| **Budget allocation** | ~600K hosting (6 months DO) + ~800K hardware (10 MikroTik pilots) + ~600K buffer |

---

## Competitive Context — XenFi

XenFi is the primary competitor already operating in Uganda:
- Has: Agent module, MoMo payments, vouchers, usage analytics, Campaigns, Remote Winbox access
- Active deployments confirmed (1,865+ vouchers seen in a single tenant)
- Tenant model: operators sign up as tenants (e.g. "optimum")
- Weakness observed: dark/dense UI, no visible offline-first capability, unclear ZengaPay integration, no white-label portal branding per location

**myFiBase differentiation vs XenFi:**
1. Offline-first edge agent — works when internet is down
2. White-label captive portal per location (not just per tenant)
3. Phone-based identity (not MAC-based)
4. ZengaPay native from day 1 (vs unclear MoMo integration in XenFi)
5. Clean modern UI built from scratch
6. Commission-based (zero upfront) vs XenFi likely subscription
7. Eastern Uganda field agent network — XenFi has no known physical presence there
