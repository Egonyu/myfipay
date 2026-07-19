# Engineering Standards — myFiBase

Adopted 2026-07-19 at Daniel's direction. These are the default working rules for
every session on this project, modeled on how Google/Meta-grade teams prevent
mistakes with **systems, not care**. They apply from the get-go, not "when we
scale". A payments platform earns no grace period.

> The one-sentence version: big companies don't write better code — they build
> systems that catch mistakes (tests, CI, staging, review, alerts). We build
> those systems here, sized for a one-droplet startup.

---

## The rules

### 1. Money paths ship with tests, always
Any code that computes, moves, or gates money (commissions, payouts, fees,
webhook processing, balance math) does not merge without unit tests. New money
feature = feature + tests in the same commit. A manual smoke test proves it
worked once; a unit test proves it still works after every future change.

### 2. Tests run in CI on every push
`go vet` + `go test` in GitHub Actions. A red build means stop and fix, not
"note it in the tracker". Tests that exist but don't run automatically don't
exist.

### 3. Prod is deployed to, never live-edited
The web root and API on the droplet change via `scripts/deploy.sh` (since
2026-07-19), not by editing files in place. The script refuses a dirty tree,
exports committed `site/` to `/var/www/myfipay-site/releases/<sha>` behind a
`current` symlink nginx serves (the repo working tree is no longer the web
root), rebuilds the API image when `api/` changed, and health-checks site +
API before reporting success. Roll back by repointing the symlink. Verification happens against local/ephemeral
containers first; prod verification is a final check, not the first one.
Temp test data in the prod DB is a last resort, cleaned in the same session,
and noted in the tracker.

### 4. Security items are blockers, not backlog
Anything on the "required before real money" list (TRACKER §2) blocks the
milestone it protects. New features do not land ahead of open security items
that touch the same surface. Attackers don't respect milestone ordering.

### 5. The service tells us when it's broken
Uptime monitoring + error visibility before pilot, no exceptions. "A customer
told us" is an incident review finding, not a monitoring strategy.

### 6. Schema changes are migrations, applied by script
No hand-run ALTERs on the live DB. Every schema change is a numbered migration
file, and applying it is a scripted, repeatable step recorded in the tracker.

### 7. Architecture changes get a written decision first
Framework swaps, storage choices, protocol changes → a dated entry in
`docs/DECISIONS.md` with the trade-off, *before* the code. The Next.js →
vanilla-JS dashboard swap was the counterexample: a defensible choice made by
accident and rationalized after. Never again.

### 8. Long-term stability beats the next shiny milestone
External integrations (MikroTik, ZengaPay prod) are pull, not push: we prepare
for them, but we do not rush toward them past open stability/security work.
When prioritizing, the order is: correctness systems → security → operability →
new features.

### 9. Every render of untrusted data is escaped by default
Frontend: any string interpolated into HTML goes through `esc()` — reviewer
checks this on every dashboard diff. Backend: parameterized queries only (pgx
already enforces this — keep it that way).

### 10. The tracker stays honest
(Existing rule, kept.) ✅ only when verified on this server; work recorded in
the same session; security posture never marked done by assumption.

---

## Daily habits checklist (start + end of every session)

**Start:**
- [ ] Read TRACKER.md + this file; check CI status of last push
- [ ] `git status` — working tree should be clean from last session

**Before any commit:**
- [ ] Tests written/updated for any money-path change
- [ ] `go vet` + `go test` pass (dockerized toolchain)
- [ ] No unescaped interpolation added to the dashboard
- [ ] No secrets in the diff

**End:**
- [ ] TRACKER.md updated (same session as the work)
- [ ] Prod DB clean of test data; commit + push

---

## Current gaps vs. these standards (2026-07-19)

Tracked so we close them deliberately; order = priority.

| # | Gap | Standard violated | Status |
|---|---|---|---|
| 1 | Zero automated tests (money paths first) | 1, 2 | ✅ closed 2026-07-19 — 9 money-path unit tests green |
| 2 | No CI | 2 | ✅ closed 2026-07-19 — Actions run on `e6072b6` succeeded |
| 3 | Prod is live-edited; no deploy step; verify-in-prod | 3 | ✅ closed 2026-07-19 — `scripts/deploy.sh`, nginx serves release export; live-edit verified inert |
| 4 | No uptime/error monitoring | 5 | ❌ open |
| 5 | CORS wildcard+credentials, no login rate limit, no JWT revocation, same-disk backups | 4 | ❌ open (TRACKER §2) |
| 6 | Dashboard XSS audit (esc() discipline never reviewed as a whole) | 9 | ❌ open |
| 7 | FreeRADIUS full restart on router sync (drops in-flight auths); Adminer on prod box | 8 | ❌ open |
| 8 | Migrations applied by hand | 6 | ❌ open |
