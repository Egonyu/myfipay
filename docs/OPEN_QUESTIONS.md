# Open Questions
## Critical decisions needed before build begins

---

## 1. Identity & Branding

**Q1.1 — What is the system name?**
This name becomes the domain, product brand, and how operators refer it to customers.
Requirements: memorable, works in Lugisu/Ateso/Luo markets, not "Emuria".
Candidates to consider: WiFiBase, HotBase, PayWifi, ZenoWifi, ConnectUG, LinkPay, ZoneUG

**Q1.2 — Will this be white-labeled or single-brand?**
Option A: Single brand — every captive portal shows your logo (fast to launch, less operator buy-in)
Option B: White-label — each operator can set their own logo/colors (requires more UI work, but operators feel ownership)
*Recommendation: start single-brand, add white-label in Phase 2*

---

## 2. Infrastructure & Hosting

**Q2.1 — Hosting: DigitalOcean Nairobi or existing Fol server or new VPS?**
The Fol server (100.95.193.92) is already running Docker. Could host the initial version there to save cost.
Downsides: shared with TesoTunes, RADIUS UDP ports may conflict.
DigitalOcean Nairobi = cleanest, ~$48/month for 2 droplets.

**Q2.2 — Domain: new domain or subdomain of existing?**
Options:
- New domain (e.g., wifibase.ug) — most professional, ~50,000 UGX/year
- Subdomain (e.g., hotspot.tesotunes.com) — free but looks shared/amateur to operators
*Recommendation: register a .ug domain for this*

**Q2.3 — Will you manage your own server or use a managed service?**
Self-managed VPS = cheaper, you need someone who can handle Linux operations.
Managed (Railway, Render, Fly.io) = more expensive but less ops burden.
Given you already manage fol server, self-managed VPS is likely fine.

---

## 3. Build vs. Buy

**Q3.1 — Is there any part of this you want to buy/license vs. build?**
Possible shortcuts:
- **Captive portal**: CoovaChilli (open source) can replace the Go captive portal component
- **Billing UI**: Volt/Stripe billing dashboard could replace custom operator dashboard (but UGX/MoMo = custom anyway)
- **RADIUS**: FreeRADIUS is the standard choice — no alternative recommended
*Recommendation: build everything except FreeRADIUS (non-negotiable standard)*

---

## 4. Payments & Finance

**Q4.1 — ZengaPay contract: what is your per-transaction rate?**
This determines gross margin calculations. Architecture assumes ~1.5%. If your rate is different the commission model shifts.
Need: your actual ZengaPay rate (%) from your contract.

**Q4.2 — How will operator settlements happen?**
Option A: Operator withdraws manually (requests via dashboard → you send via ZengaPay disbursement)
Option B: Automatic weekly payout every Friday via scheduled disbursement
Option C: Operator wallet balance they can withdraw anytime
*Recommendation: start with Option A (manual) in Phase 1, automate in Phase 2*

**Q4.3 — Minimum withdrawal / settlement amount?**
Below a threshold, ZengaPay fees eat the payout. Suggested: 10,000 UGX minimum.

**Q4.4 — Will you support cash payments (operator collects cash, enters manually)?**
Some operators may not want to force MoMo. A "cash" payment type in the system lets operators manually grant sessions.
This creates a trust problem — operator could grant sessions without paying the platform.
Decision needed: trust operators with cash mode, or MoMo-only?

---

## 5. Offline & Edge

**Q5.1 — How long offline should the system support before edge agent fails?**
Current plan: indefinite (local SQLite caches sessions, queues payments).
Is this correct or should there be a maximum offline duration (e.g., 24 hours)?
After 24h offline, new sessions could be rejected until cloud reconnects.

**Q5.2 — Will every deployment have a Raspberry Pi / edge agent, or is it optional?**
Option A: Edge agent is optional add-on for operators who want offline resilience
Option B: Recommend edge agent for every deployment (standard)
Option C: MikroTik-only (direct RADIUS to cloud) for most operators, edge for rural-only
*Recommendation: Option C — MikroTik direct for urban, edge for rural operators*

**Q5.3 — What's the maximum acceptable payment latency from PIN entry to WiFi access?**
Architecture targets < 5 seconds. Is this acceptable or do you need faster?
Actual bottleneck: ZengaPay webhook delay (varies 2–10s for MoMo confirmation).

---

## 6. Operator Experience

**Q6.1 — How technical are your target operators?**
Sarah at a kiosk: not technical at all — needs a phone app, not a web dashboard.
Hotel manager: somewhat technical, can use a web dashboard.
Implication: is a React Native mobile app for operators required in Phase 1, or Phase 2?

**Q6.2 — How should operators first set up their router?**
Option A: You (or your agent) physically visit and configure the MikroTik on-site
Option B: Operator copies a script from the dashboard and pastes it into MikroTik terminal
Option C: Plug-and-play pre-configured router shipped to operator
*Recommendation: Option A for early operators, move to B + C as you scale*

**Q6.3 — Multiple locations per operator — Phase 1 or Phase 2?**
Sarah has one kiosk. Hotel chain may have 3 locations.
Supporting multiple locations is built into the schema but the dashboard needs to handle it.
Decision: include multi-location UI in Phase 1 or simplify to single-location first?

---

## 7. End User Experience

**Q7.1 — Should users be able to top up without reconnecting?**
Current design: session expires → user reconnects → new portal session.
Alternative: send SMS with top-up link 10 minutes before expiry → user pays without reconnecting.
SMS adds cost (~50 UGX/SMS) but dramatically improves UX.

**Q7.2 — MAC-based vs. phone-based identity?**
Current design: username is MAC address (device identity) for RADIUS.
Alternative: phone number as primary identity — same phone number gets session regardless of which device.
MAC-based = simpler to implement. Phone-based = better for "family plan" use cases.
Decision needed for Phase 1.

**Q7.3 — Should the captive portal support Luganda / local language?**
Soroti = Ateso/Kumam. Mbale = Lugisu/Gisu. Lira = Lango/Acholi.
English-only portal works for now, but local language significantly improves conversion for rural operators.
Decision: English only for Phase 1?

---

## 8. Agent Network

**Q8.1 — Who are your first 5 target agents?**
You mentioned having local advantage in Eastern Uganda. Specific names/contacts?
Early agents define your initial geography of operators.

**Q8.2 — How will agents be paid their commission?**
Option A: Weekly MoMo disbursement to agent's phone
Option B: Agent has a balance they withdraw via dashboard
Option C: Manual bank transfer (not recommended for small amounts)

**Q8.3 — Will agents have their own dashboard login?**
Current architecture gives agents a portal to see their operators' stats.
Is this a Phase 1 requirement or can agents start with just WhatsApp updates from you?

---

## 9. Legal & Compliance

**Q9.1 — Do you need a UCC (Uganda Communications Commission) license?**
Providing internet services for commercial resale may require registration.
Operators are likely the ones who need ISP-level licenses, but aggregating operators may trigger licensing.
Action needed: consult with UCC or a local telecoms lawyer before commercial launch.

**Q9.2 — Data retention: how long do you keep session/phone number data?**
GDPR-equivalent in Uganda (PDPA 2019) requires defined retention periods.
Recommended: 90 days for session logs, phone numbers hashed after that.

**Q9.3 — ZengaPay Know Your Customer (KYC): what's required for operators?**
ZengaPay likely requires operator KYC for disbursements. What documents?
Will you handle operator KYC or delegate to ZengaPay entirely?

---

## 10. Timeline & Resources

**Q10.1 — What is the target date for first live pilot operator?**
Everything cascades from this. 3 months? 6 months?

**Q10.2 — Who is building this?**
Solo (you) → MVP takes 4–6 months full-time
With a Go developer → 2–3 months
Outsourced team → 6–10 weeks if managed tightly
*Architecture is designed for a team of 2–3 developers*

**Q10.3 — What is the Phase 1 budget?**
Minimum viable: 2 DigitalOcean droplets + domain + 10 MikroTik routers for pilots
Approximate cost: 500,000 – 700,000 UGX for hardware + 3 months hosting

---

## Priority Matrix

| Question | Must answer before build | Can decide later |
|---|---|---|
| System name | YES | |
| Hosting decision | YES | |
| ZengaPay rate | YES | |
| Cash payment support | YES | |
| Offline duration | YES | |
| Operator settlement method | YES | |
| Mobile app Phase 1? | YES | |
| Multi-location Phase 1? | | YES (Phase 2) |
| Local language portal | | YES (Phase 2) |
| Agent dashboard Phase 1? | | YES (Phase 2) |
| USSD top-up | | YES (Phase 2) |
| UCC license | | Consult lawyer |
