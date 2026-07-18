# Hardware Compatibility Matrix

---

## Tier 1 — Fully Supported (Certified)

These are tested, documented, and include one-click setup guides.

| Hardware | Model | Protocol | Notes |
|---|---|---|---|
| **MikroTik** | hAP ac², hAP ac³, hAP lite | RADIUS | Most common in Uganda. Full support for bandwidth limits, session timeout via RADIUS attributes |
| **MikroTik** | RB750Gr3, RB951Ui-2HnD | RADIUS | Entry-level, very affordable. Widely deployed by small operators |
| **MikroTik** | hEX PoE, CRS switches | RADIUS | For larger venues with external APs |
| **MikroTik** | CHR (Cloud Hosted Router) | RADIUS | For edge agent VM deployment on existing x86 hardware |
| **MikroTik** | RouterBoard any (RouterOS 6.x, 7.x) | RADIUS | Universal — if it runs RouterOS, it works |
| **Ubiquiti** | UniFi AP AC Lite, AC Pro, AP6 | RADIUS | Popular in schools and hotels. Managed via UniFi Controller |
| **Ubiquiti** | EdgeRouter X, ER-4 | RADIUS | For wired NAT + wireless AP combo |
| **TP-Link** | EAP225, EAP245, EAP660 HD | RADIUS | Omada SDN controller RADIUS integration |
| **TP-Link** | TL-WR840N (OpenWRT) | RADIUS | Flashed with OpenWRT + CoovaChilli or similar |

---

## Tier 2 — Supported (Community Tested)

| Hardware | Model | Protocol | Notes |
|---|---|---|---|
| **Huawei** | AR151, AR169 | RADIUS | Less common but RADIUS compliant |
| **ZTE** | MF286, ZXA10 | RADIUS | Common in telco-supplied deployments |
| **Cisco** | SG series (small business) | RADIUS | For enterprise hotel/school deployments |
| **GL.iNet** | GL-MT300N, GL-AR750 | RADIUS | OpenWRT-based, popular for travel routers |
| **OpenWRT** | Any supported device | RADIUS via CoovaChilli | Full captive portal + RADIUS control |
| **pfSense / OPNsense** | Any | RADIUS | For technical operators using PC-based firewalls |
| **Raspberry Pi** | 3B+, 4B | Edge Agent | Runs the edge agent + acts as RADIUS proxy |
| **Starlink** | Gen 2, Gen 3 (bypass mode) | RADIUS | Starlink in bypass + MikroTik as RADIUS NAS |

---

## Tier 3 — Planned (Roadmap)

| Hardware | Notes |
|---|---|
| **Reyee (Ruijie)** | Growing in East Africa, RADIUS support exists |
| **Cambium** | Enterprise WISPs, ePMP series |
| **BDCOM** | Used by some ISPs in Uganda |
| **H3C** | Enterprise, RADIUS compliant |

---

## MikroTik Setup (Most Common — Detailed)

### Step 1: RADIUS Client Configuration
```
/radius
add address=<cloud-ip> secret=<device-secret> service=hotspot timeout=3s
```

### Step 2: Hotspot Server Profile
```
/ip hotspot profile
set [find default=yes] use-radius=yes radius-accounting=yes \
    radius-interim-update=1m
```

### Step 3: Walled Garden (allow captive portal domains)
```
/ip hotspot walled-garden ip
add dst-address=<cloud-ip> action=accept
add dst-address=<zengapay-ip> action=accept
```

### Step 4: DHCP + DNS for captive portal redirect
```
/ip dns
set servers=<cloud-ip>
```

Auto-setup via our provisioning API:
`POST /api/devices/provision` → returns RouterOS script → paste in MikroTik terminal.

---

## Ubiquiti UniFi Setup

1. UniFi Network Controller → Settings → Profiles → RADIUS
2. Add RADIUS profile:
   - Auth server: `<cloud-ip>:1812`
   - Accounting server: `<cloud-ip>:1813`
   - Shared secret: `<device-secret>`
3. Apply to SSID → Enable RADIUS MAC Authentication
4. Set captive portal to "External Portal"
   - Portal URL: `https://portal.<domain>.com/connect?location=<id>`

---

## Edge Agent Installation (Raspberry Pi)

```bash
# Download edge agent binary for ARM
curl -fsSL https://api.<domain>.com/edge/install.sh | sudo bash

# Configure with your location credentials
sudo /opt/edge-agent/configure --location-id=<id> --secret=<key>

# Start as system service
sudo systemctl enable edge-agent
sudo systemctl start edge-agent
```

The edge agent automatically:
- Registers the device with the cloud
- Configures FreeRADIUS locally
- Sets up captive portal on local IP
- Begins syncing sessions

---

## RADIUS Attributes Used

These attributes are sent in Access-Accept to control the user session:

| Attribute | Value | Effect |
|---|---|---|
| `Session-Timeout` | seconds | Auto-disconnect after N seconds |
| `Idle-Timeout` | seconds | Disconnect if idle for N seconds |
| `WISPr-Bandwidth-Max-Down` | bps | Download speed limit |
| `WISPr-Bandwidth-Max-Up` | bps | Upload speed limit |
| `Mikrotik-Rate-Limit` | "2M/1M" | MikroTik-specific rate limiting |
| `Framed-IP-Address` | IP | Assign specific IP (optional) |
| `Class` | plan_id | Tag session with plan for accounting |

---

## Minimum Hardware Requirements

### For Operators

| Scenario | Recommended Hardware | Approx. UGX Cost |
|---|---|---|
| Small kiosk (< 20 users) | MikroTik hAP lite | 80,000 – 120,000 |
| Medium shop/cafe (20–50) | MikroTik hAP ac² | 180,000 – 250,000 |
| Large venue (50–150) | MikroTik RB750 + APs | 400,000 – 600,000 |
| Hotel / School (150–500) | Ubiquiti UniFi setup | 1,500,000 – 3,000,000 |

### For Edge Agent (Offline Mode)

| Device | RAM | Storage | Cost |
|---|---|---|---|
| Raspberry Pi 3B+ | 1GB | 16GB microSD | ~120,000 UGX |
| Raspberry Pi 4B | 4GB | 32GB microSD | ~200,000 UGX |
| MikroTik CHR on old PC | 512MB+ | 4GB | Reuse existing |
| NanoPi Neo2 | 512MB | 8GB | ~80,000 UGX |
