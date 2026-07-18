# Business Model & Go-To-Market Strategy
## Eastern Uganda Hotspot Billing Platform

---

## 1. Revenue Streams

### Primary — Transaction Commission
Take a percentage of every payment processed through the platform.

| Tier | Operator Monthly Revenue | Platform Commission |
|---|---|---|
| Starter | 0 – 500,000 UGX/mo | 8% per transaction |
| Growth | 500K – 2M UGX/mo | 6% per transaction |
| Business | 2M – 10M UGX/mo | 5% per transaction |
| Enterprise | 10M+ UGX/mo | 4% (negotiated) |

**Why commission not SaaS subscription?**
- Operators in Eastern Uganda resist upfront fees
- Commission aligns our growth with operator growth
- Zero friction to start — operators only pay when they earn
- ZengaPay already handles collection — we just take our cut before settlement

### Secondary — Hardware Pre-configuration
- Pre-configured MikroTik routers sold or bundled at 20,000 – 50,000 UGX markup
- Raspberry Pi edge agent kits: 50,000 UGX setup fee
- On-site installation in Soroti/Mbale/Tororo: 100,000 – 150,000 UGX

### Tertiary — Agent Wholesale Vouchers
- Agents buy voucher batches at 5% below retail price
- Sell at retail → margin is theirs
- We earn the ZengaPay processing fee on the agent's purchase

### Future — Ad Revenue (Phase 3)
- Operators offer a free 30-minute tier supported by ads
- Advertisers (telcos, FMCG, banks) buy impressions on captive portal
- Operator gets 60%, platform gets 40%
- Eastern Uganda's captive portal = high-dwell, high-intent audience

---

## 2. Pricing Illustration

**Scenario: Sarah's kiosk, Soroti market**
- 100 customers/day × 500 UGX average = 50,000 UGX/day
- 1,500,000 UGX/month in gross revenue
- Platform commission (6%): **90,000 UGX/month**
- Sarah nets: **1,410,000 UGX/month** (from what was previously 0 — cash leaked)

**Scale: 200 operators at this level**
- Platform monthly revenue: **18,000,000 UGX (~$4,800 USD)**
- At 500 operators: **45,000,000 UGX (~$12,000 USD/month)**

---

## 3. Go-To-Market Strategy

### Phase 0 — Pre-Launch (Month 1–2)
1. **Identify 10 pilot operators** personally in Soroti and Mbale
   - Target: market kiosks, small hotels, butcheries with power sockets and foot traffic
   - Offer: **free for 3 months** in exchange for feedback + testimonial
2. **Pre-configure hardware** — buy 10 MikroTik hAP lites, set up, deliver
3. **Build waiting list** via WhatsApp broadcast and Facebook groups
   - "Uganda WiFi Operators" Facebook group
   - Soroti Business Network WhatsApp groups
4. **Document pilot results** — screenshots of revenue, operator quotes

### Phase 1 — Eastern Uganda First (Month 3–6)
1. **Agent recruitment**: Hire 5 field agents in:
   - Soroti (2 agents — biggest market)
   - Mbale (1 agent)
   - Lira (1 agent)
   - Tororo/Busia border (1 agent — high traffic, cross-border users)
2. **Agent commission**: 3% of every transaction from their operators (lifetime)
3. **Weekly targets**: Each agent onboards 5 operators/month = 25 new operators/month
4. **WhatsApp support channel**: Agents have direct line to technical support
5. **Voucher reselling**: Agents sell pre-printed branded voucher books at markup

### Phase 2 — Regional Dominance (Month 6–12)
1. **School campaign**: Partner with 20 secondary schools for student internet
   - Student weekly bundles (2,000 UGX/week for daily 500MB)
   - School admin dashboard, student ID integration
2. **Hotel campaign**: Target Mbale, Jinja, Tororo hotels
   - Branded guest WiFi, room-based access, check-in integration
3. **Market vendor network**: Set up hotspot mesh at major markets
   - Soroti main market, Mbale market, Iganga market
4. **Referral program**: Every operator who refers another gets 1% of referee's revenue for 6 months

### Phase 3 — Scale & Defend (Month 12+)
1. Expand to West Nile, Northern Uganda, Central
2. USSD shortcode for feature phone top-up
3. National roaming: users can roam across partner hotspots
4. Telco partnerships (MTN/Airtel reseller deal for bulk data)

---

## 4. Agent Network Structure

```
Platform (you)
    │ 3% commission of agent's total operator transactions
    ├── Agent: Soroti Central (Ivan)
    │       │ 3% commission of their operators' transactions
    │       ├── Operator: Sarah's Kiosk — 1.5M UGX/month → Ivan earns 45,000 UGX
    │       ├── Operator: Hotel Aloet — 3M UGX/month → Ivan earns 90,000 UGX
    │       └── Operator: Total Petrol Soroti — 2M UGX/month → Ivan earns 60,000 UGX
    │                      Ivan's monthly income: 195,000 UGX (passive)
    │
    └── Agent: Mbale City (Grace)
            ├── Operator: Mbale Resort Hotel
            ├── Operator: UMI Cafe
            └── Operator: St. Andrew's School
```

**Agent benefits**:
- Recurring income without active selling after onboarding
- Provided: branded tablet/laptop, agent polo shirt, business cards
- Monthly agent meetup + leaderboard + cash prizes

---

## 5. Competitive Moat

| Moat | Description |
|---|---|
| **Regional presence** | Physical agents in Soroti/Mbale/Lira — competitors have none |
| **Offline resilience** | Only system that works when internet is down |
| **ZengaPay native** | No competitor uses ZengaPay — UGX settlements without USD friction |
| **Pre-configured hardware** | Operators get a plug-and-play device — zero technical setup |
| **Agent loyalty** | Agents earn forever from their operators — they actively defend the platform |
| **Data advantage** | After 12 months: usage patterns, pricing intelligence unique to Eastern Uganda |

---

## 6. Cost Structure

### Fixed Monthly (Year 1)
| Cost | UGX/month |
|---|---|
| VPS (DigitalOcean Nairobi, 2 servers) | 120,000 |
| Domain + SSL | 8,000 |
| ZengaPay fixed fee | 0 (% only) |
| SMS (avg 5,000 messages/month) | 50,000 |
| Monitoring (Grafana Cloud free tier) | 0 |
| **Total fixed** | **~178,000 UGX** |

### Variable (% of transaction volume)
| Cost | Rate |
|---|---|
| ZengaPay processing fee | ~1.5% of transaction |
| Agent commissions | 3% of operator transactions |
| Total variable cost | ~4.5% |

### Gross Margin at Scale
- Platform takes 6% from operators
- Costs: 1.5% (ZengaPay) + 3% (agent) = 4.5%
- **Gross margin: 1.5% of all transactions processed**
- At 100M UGX/month processed: **1,500,000 UGX gross profit** + fixed overhead paid

---

## 7. Risks & Mitigations

| Risk | Likelihood | Mitigation |
|---|---|---|
| XenFi enters Eastern Uganda aggressively | Medium | Agent loyalty + offline mode + regional relationships |
| ZengaPay API downtime | Low | Voucher fallback always available |
| MikroTik hardware shortage (import) | Medium | Pre-buy buffer stock, support TP-Link alternative |
| Operators switch to free competitor | Medium | Commission model — no upfront cost, agents defend churn |
| Electricity outages affect operations | High | Edge agent + battery backup UPS included in enterprise kit |
| Mobile money fraud / chargebacks | Low | ZengaPay handles fraud, idempotency keys prevent double-charge |
