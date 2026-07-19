# MikroTik CHR Live Test (P0.5 #3)

Goal: prove a real RouterOS device authenticates hotspot users against our
FreeRADIUS via the self-serve router-onboarding flow — the last technical
blocker (with ZengaPay prod) before the founder dry-run and M8.

The test router is MikroTik **CHR** (Cloud Hosted Router) installed over a
throwaway DigitalOcean droplet. First attempt (170.64.169.239) died with SSH
timeouts — likely the image conversion or networking never came up. This is
the from-scratch procedure.

---

## Phase 0 — Rebuild the CHR droplet (Daniel)

1. **Create droplet**: region **syd1** (same as the myFiBase server, so both
   share the default VPC `10.49.0.0/16` — required for Phase 2), cheapest
   size, any Ubuntu image (it gets overwritten). No need to add SSH keys for
   the final router, but add yours anyway for the conversion step.

2. **Convert to CHR** — SSH in as root and run:

   ```bash
   wget "https://download.mikrotik.com/routeros/7.16.2/chr-7.16.2.img.zip" -O /tmp/chr.img.zip
   cd /tmp && unzip chr.img.zip
   lsblk   # confirm the disk is /dev/vda
   dd if=/tmp/chr-*.img of=/dev/vda bs=4M oflag=sync status=progress
   # hard reboot without letting the (now-overwritten) OS shut down cleanly:
   echo 1 > /proc/sys/kernel/sysrq
   echo b > /proc/sysrq-trigger
   ```

   Yes, `dd` over the running root disk is intentional — the droplet becomes
   the router. Wait ~1–2 minutes after the reboot.

3. **First login — use the DO web console** (droplet page → Access → Launch
   Droplet Console), *not* SSH, because networking may not come up on its
   own (the likely cause of last time's timeout). Login `admin`, empty
   password; RouterOS 7 forces you to set one — **make it strong, this is a
   public router**.

4. **If the console shows no IP** (`/ip address print` empty): copy the
   public IP, netmask and gateway from the droplet's Networking tab, then:

   ```
   /ip address add address=<PUBLIC_IP>/20 interface=ether1
   /ip route add gateway=<GATEWAY>
   /ip dns set servers=1.1.1.1
   /ping 170.64.177.20
   ```

5. **Harden immediately** (CHR on a public IP gets scanned within minutes):

   ```
   /ip service disable telnet,ftp,www,api,api-ssl
   /ip service print   # leave ssh + winbox
   ```

6. Note the VPC/private interface: `/interface print` should show a second
   NIC (ether2). Give it the droplet's private IP from the Networking tab:

   ```
   /ip address add address=<VPC_IP>/16 interface=ether2
   ```

License note: unlicensed CHR is limited to 1 Mbps per interface — fine for
this test; the free trial can be activated later if needed.

**Hand back to Claude: the new public IP + VPC IP.** Steps from here run
from the myFiBase server + dashboard.

---

## Phase 1 — Self-serve registration (this is the product test)

Done through the dashboard exactly as a real operator would (no SSH):

1. Dashboard → Routers → Add router with the CHR's **public IP**.
2. Within ≤1 min the `radius-sync.sh` cron must: add the UFW allow for
   1812-1813/udp from that IP, and FreeRADIUS must log
   `Adding client <IP>` (per-device secret from the `nas` table).
3. Copy the generated setup script from the dashboard modal; paste into the
   CHR terminal (via SSH or DO console). Note: the script's step-1 `/radius
   add` works standalone; the hotspot steps need Phase 2's hotspot first.
4. Dashboard "Test connection" goes green only after the router has actually
   spoken RADIUS to us (radpostauth) — expect that in Phase 2.

## Phase 2 — Hotspot + captive-portal flow over the VPC

CHR has no WiFi and no LAN clients, but the myFiBase server shares the VPC —
so the server plays the role of a hotspot client.

On the CHR: run Hotspot Setup on **ether2** (the VPC interface), then paste
the dashboard script (RADIUS, `use-radius=yes`, walled garden), and upload
the dashboard-generated `login.html` into the `hotspot/` folder.

On the myFiBase server (temporary, surgical — never touch the default
route):

```bash
# route ONE harmless external IP via the CHR so its hotspot intercepts us
ip route add <TEST_IP>/32 via <CHR_VPC_IP>
curl -v http://<TEST_IP>/        # expect the hotspot redirect / our portal
```

Success criteria (all verified from the server / dashboard):

- [ ] Hotspot redirect reaches the branded portal URL (walled garden lets it through)
- [ ] A login attempt produces a `radpostauth` row with `nasipaddress` = CHR public IP
- [ ] Dashboard "Test connection" shows the router online
- [ ] Granting a session (dashboard or voucher) then logging in through the
      hotspot yields Access-Accept + a `radacct` row + rate limit visible in
      `/ip hotspot active print detail` on the CHR
- [ ] Observe: dashboard "terminate" removes radcheck but does NOT kick the
      active hotspot session (no CoA/Disconnect yet — record actual behavior;
      candidate P2 item)

Cleanup: `ip route del <TEST_IP>/32`; keep or destroy the CHR droplet
(≈$4–6/mo if kept as a permanent test rig).

---

*Written 2026-07-19 while the CHR droplet was being rebuilt.*
