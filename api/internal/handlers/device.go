package handlers

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/myfibase/myfibase/internal/middleware"
	"github.com/myfibase/myfibase/internal/wireguard"
)

// Router (NAS) self-onboarding. A registered device gets a per-router RADIUS
// secret stored both in "devices" (product view) and in the FreeRADIUS "nas"
// table (read_clients=yes). scripts/radius-sync.sh on the host notices nas
// changes, opens UFW for the router IP, and restarts FreeRADIUS.

const radiusServerIP = "170.64.177.20" // routers speak RADIUS to the droplet directly, not via Cloudflare

func genDeviceSecret() (string, error) {
	const charset = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = charset[int(b[i])%len(charset)]
	}
	return string(b), nil
}

type deviceRow struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Type       string     `json:"type"`
	NasIP      string     `json:"nas_ip"`
	Secret     string     `json:"radius_secret"`
	LocationID string     `json:"location_id"`
	Location   string     `json:"location_name"`
	PortalSlug string     `json:"portal_slug"`
	Online     bool       `json:"online"`
	LastSeen   *time.Time `json:"last_seen"`
	LastPing   *time.Time `json:"last_ping"`
	CreatedAt  time.Time  `json:"created_at"`
}

func (h *Handler) deviceByID(ctx context.Context, deviceID, tenantID string) (*deviceRow, error) {
	var d deviceRow
	err := h.db.QueryRow(ctx, `
		SELECT d.id, COALESCE(d.name,''), d.type, COALESCE(host(d.nas_ip),''), d.radius_secret,
		       l.id, l.name, l.portal_slug, d.online, d.last_seen, d.last_ping, d.created_at
		FROM devices d JOIN locations l ON d.location_id = l.id
		WHERE d.id = $1 AND l.tenant_id = $2 LIMIT 1
	`, deviceID, tenantID).Scan(&d.ID, &d.Name, &d.Type, &d.NasIP, &d.Secret,
		&d.LocationID, &d.Location, &d.PortalSlug, &d.Online, &d.LastSeen, &d.LastPing, &d.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (h *Handler) ListDevices(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r)
	ctx := context.Background()

	rows, err := h.db.Query(ctx, `
		SELECT d.id, COALESCE(d.name,''), d.type, COALESCE(host(d.nas_ip),''), d.radius_secret,
		       l.id, l.name, l.portal_slug, d.online, d.last_seen, d.last_ping, d.created_at
		FROM devices d JOIN locations l ON d.location_id = l.id
		WHERE l.tenant_id = $1 ORDER BY d.created_at DESC
	`, claims.TenantID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "query failed")
		return
	}
	defer rows.Close()

	var devices []deviceRow
	for rows.Next() {
		var d deviceRow
		if err := rows.Scan(&d.ID, &d.Name, &d.Type, &d.NasIP, &d.Secret,
			&d.LocationID, &d.Location, &d.PortalSlug, &d.Online, &d.LastSeen, &d.LastPing, &d.CreatedAt); err == nil {
			devices = append(devices, d)
		}
	}
	if devices == nil {
		devices = []deviceRow{}
	}
	respond(w, http.StatusOK, devices)
}

func (h *Handler) CreateDevice(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r)
	ctx := context.Background()

	var req struct {
		Name       string `json:"name"`
		LocationID string `json:"location_id"`
		NasIP      string `json:"nas_ip"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" || req.LocationID == "" {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "name, location_id and nas_ip are required")
		return
	}
	ip := net.ParseIP(req.NasIP)
	if ip == nil || ip.To4() == nil {
		respondError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "nas_ip must be a valid IPv4 address (your router's public IP)")
		return
	}

	// Location must belong to this tenant
	var locOK bool
	h.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM locations WHERE id=$1 AND tenant_id=$2)`, req.LocationID, claims.TenantID).Scan(&locOK)
	if !locOK {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "location not found")
		return
	}

	// One RADIUS client per IP, platform-wide (FreeRADIUS keys clients by source IP)
	var ipTaken bool
	h.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM devices WHERE nas_ip = $1)`, ip.String()).Scan(&ipTaken)
	if ipTaken {
		respondError(w, http.StatusConflict, "CONFLICT", "a router with this IP address is already registered")
		return
	}

	secret, err := genDeviceSecret()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "SECRET_ERROR", "could not generate secret")
		return
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "could not start transaction")
		return
	}
	defer tx.Rollback(ctx)

	var deviceID string
	err = tx.QueryRow(ctx, `
		INSERT INTO devices (location_id, name, type, nas_identifier, nas_ip, radius_secret)
		VALUES ($1, $2, 'mikrotik', NULL, $3, $4) RETURNING id
	`, req.LocationID, req.Name, ip.String(), secret).Scan(&deviceID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "could not register device")
		return
	}

	shortname := "mfb-" + deviceID[:8]
	_, err = tx.Exec(ctx, `
		INSERT INTO nas (nasname, shortname, type, secret, description)
		VALUES ($1, $2, 'mikrotik', $3, $4)
	`, ip.String(), shortname, secret, req.Name)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "could not register RADIUS client")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "could not save device")
		return
	}

	d, err := h.deviceByID(ctx, deviceID, claims.TenantID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "device saved but could not be loaded")
		return
	}
	respond(w, http.StatusCreated, d)
}

func (h *Handler) UpdateDevice(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r)
	deviceID := chi.URLParam(r, "id")
	ctx := context.Background()

	var req struct {
		Name  string `json:"name"`
		NasIP string `json:"nas_ip"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "name and nas_ip are required")
		return
	}
	ip := net.ParseIP(req.NasIP)
	if ip == nil || ip.To4() == nil {
		respondError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "nas_ip must be a valid IPv4 address")
		return
	}

	d, err := h.deviceByID(ctx, deviceID, claims.TenantID)
	if err != nil {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "device not found")
		return
	}

	if ip.String() != d.NasIP {
		var ipTaken bool
		h.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM devices WHERE nas_ip = $1 AND id <> $2)`, ip.String(), deviceID).Scan(&ipTaken)
		if ipTaken {
			respondError(w, http.StatusConflict, "CONFLICT", "a router with this IP address is already registered")
			return
		}
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "could not start transaction")
		return
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `UPDATE devices SET name=$1, nas_ip=$2, updated_at=NOW() WHERE id=$3`, req.Name, ip.String(), deviceID)
	if err == nil {
		_, err = tx.Exec(ctx, `UPDATE nas SET nasname=$1, description=$2 WHERE shortname=$3`, ip.String(), req.Name, "mfb-"+deviceID[:8])
	}
	if err != nil || tx.Commit(ctx) != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "could not update device")
		return
	}
	respond(w, http.StatusOK, map[string]string{"message": "device updated"})
}

func (h *Handler) DeleteDevice(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r)
	deviceID := chi.URLParam(r, "id")
	ctx := context.Background()

	if _, err := h.deviceByID(ctx, deviceID, claims.TenantID); err != nil {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "device not found")
		return
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "could not start transaction")
		return
	}
	defer tx.Rollback(ctx)
	tx.Exec(ctx, `DELETE FROM nas WHERE shortname=$1`, "mfb-"+deviceID[:8])
	tx.Exec(ctx, `DELETE FROM devices WHERE id=$1`, deviceID)
	if err := tx.Commit(ctx); err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "could not remove device")
		return
	}
	respond(w, http.StatusOK, map[string]string{"message": "device removed"})
}

// ensureWG lazily provisions the device's management-tunnel identity: a
// keypair (API-generated so the setup script stays copy-paste) and a tunnel
// IP from 10.77.0.0/16. Idempotent — returns the existing identity if set.
// The host cron scripts/wg-sync.sh picks the peer up within a minute.
func (h *Handler) ensureWG(ctx context.Context, deviceID string) (wgIP, privKey string, err error) {
	err = h.db.QueryRow(ctx, `SELECT COALESCE(host(wg_ip),''), COALESCE(wg_private_key,'')
		FROM devices WHERE id=$1`, deviceID).Scan(&wgIP, &privKey)
	if err != nil || (wgIP != "" && privKey != "") {
		return wgIP, privKey, err
	}
	priv, pub, err := wireguard.GenerateKeypair()
	if err != nil {
		return "", "", err
	}
	err = h.db.QueryRow(ctx, `
		UPDATE devices SET wg_private_key=$1, wg_public_key=$2,
			wg_ip='10.77.0.0'::inet + nextval('wg_ip_seq'), updated_at=NOW()
		WHERE id=$3 AND wg_ip IS NULL
		RETURNING host(wg_ip)
	`, priv, pub, deviceID).Scan(&wgIP)
	if err != nil {
		// lost a provisioning race — another request set it; read theirs
		err = h.db.QueryRow(ctx, `SELECT COALESCE(host(wg_ip),''), COALESCE(wg_private_key,'')
			FROM devices WHERE id=$1 AND wg_ip IS NOT NULL`, deviceID).Scan(&wgIP, &privKey)
		return wgIP, privKey, err
	}
	return wgIP, priv, nil
}

// DeviceScript returns a copy-paste MikroTik terminal script for this router.
// The hotspot login page itself is generated by the dashboard (login.html
// download) because RouterOS file quoting from the terminal is unreliable.
func (h *Handler) DeviceScript(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r)
	deviceID := chi.URLParam(r, "id")
	ctx := context.Background()

	d, err := h.deviceByID(ctx, deviceID, claims.TenantID)
	if err != nil {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "device not found")
		return
	}

	script := fmt.Sprintf(`# myFiPay router setup — %s (%s)
# Prerequisite: run MikroTik's built-in Hotspot Setup first
# (IP -> Hotspot -> Hotspot Setup in Winbox/WebFig), then paste this
# whole script into a Terminal window.

# 1. Point the hotspot at the myFiPay RADIUS server
/radius add service=hotspot address=%s secret="%s" timeout=3s comment="myFiPay"
/radius incoming set accept=yes

# 2. Authenticate hotspot users via RADIUS
/ip hotspot profile set [find] use-radius=yes login-by=http-pap,http-chap

# 3. Walled garden: let customers reach the payment portal before they pay
/ip hotspot walled-garden add dst-host=myfipay.com comment="myFiPay portal"
/ip hotspot walled-garden add dst-host=*.myfipay.com comment="myFiPay portal"
/ip hotspot walled-garden ip add action=accept dst-host=myfipay.com comment="myFiPay portal"

# 4. Replace the hotspot login page:
#    In the myFiPay dashboard, click "Download login.html" for this router,
#    then upload the file into the router's "hotspot" folder
#    (Winbox -> Files -> drag into hotspot/), replacing login.html.
#    It redirects customers to your branded portal:
#    https://myfipay.com/portal/%s/

# 5. Back in the dashboard, click "Test connection", then connect a phone
#    to your WiFi and try to open any website — you should see your portal.
`, d.Name, d.Location, radiusServerIP, d.Secret, d.PortalSlug)

	if h.cfg.WGServerPublicKey != "" {
		if wgIP, wgPriv, err := h.ensureWG(ctx, d.ID); err == nil {
			script += fmt.Sprintf(`
# 6. Management tunnel (RouterOS 7 or newer; safe to skip on RouterOS 6 —
#    everything above still works). Lets myFiPay monitor your router and end
#    sessions cleanly even when it has no public IP.
/interface/wireguard add name=myfipay-mgmt private-key="%s" comment="myFiPay"
/ip/address add address=%s/16 interface=myfipay-mgmt
/interface/wireguard/peers add interface=myfipay-mgmt public-key="%s" endpoint-address=%s endpoint-port=%s allowed-address=10.77.0.1/32 persistent-keepalive=25s comment="myFiPay"
`, wgPriv, wgIP, h.cfg.WGServerPublicKey, h.cfg.WGEndpointHost, h.cfg.WGEndpointPort)
		} else {
			log.Printf("wg provision failed for device %s: %v", d.ID, err)
		}
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(script))
}

// DeviceStatus reports router health from two signals: RADIUS traffic (auth
// attempts in radpostauth, accounting in radacct) and the ICMP heartbeat cron
// (devices.last_ping, scripts/router-heartbeat.sh). Online means either signal
// is fresh — RADIUS alone can't tell a dead router from one with no customers.
func (h *Handler) DeviceStatus(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r)
	deviceID := chi.URLParam(r, "id")
	ctx := context.Background()

	d, err := h.deviceByID(ctx, deviceID, claims.TenantID)
	if err != nil {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "device not found")
		return
	}

	var lastAuth, lastAcct *time.Time
	h.db.QueryRow(ctx, `SELECT MAX(authdate) FROM radpostauth WHERE nasipaddress = $1`, d.NasIP).Scan(&lastAuth)
	h.db.QueryRow(ctx, `SELECT MAX(COALESCE(acctupdatetime, acctstarttime)) FROM radacct WHERE nasipaddress = $1`, d.NasIP).Scan(&lastAcct)

	var lastSeen *time.Time
	if lastAuth != nil {
		lastSeen = lastAuth
	}
	if lastAcct != nil && (lastSeen == nil || lastAcct.After(*lastSeen)) {
		lastSeen = lastAcct
	}
	radiusFresh := lastSeen != nil && time.Since(*lastSeen) < 10*time.Minute
	pingFresh := d.LastPing != nil && time.Since(*d.LastPing) < 3*time.Minute
	online := radiusFresh || pingFresh

	h.db.Exec(ctx, `UPDATE devices SET online=$1, last_seen=COALESCE($2, last_seen), updated_at=NOW() WHERE id=$3`,
		online, lastSeen, deviceID)

	respond(w, http.StatusOK, map[string]any{
		"online":     online,
		"last_seen":  lastSeen,
		"last_ping":  d.LastPing,
		"last_auth":  lastAuth,
		"last_acct":  lastAcct,
		"checked_at": time.Now().UTC(),
	})
}

// DeviceClients lists who is on the router right now: open radacct rows
// (no Stop yet) for its NAS IP. Bytes/duration are as of the router's last
// interim update, not real time.
func (h *Handler) DeviceClients(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r)
	deviceID := chi.URLParam(r, "id")
	ctx := context.Background()

	d, err := h.deviceByID(ctx, deviceID, claims.TenantID)
	if err != nil {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "device not found")
		return
	}
	if d.NasIP == "" {
		respond(w, http.StatusOK, []struct{}{})
		return
	}

	type clientRow struct {
		Username    string     `json:"username"`
		IP          string     `json:"ip"`
		Mac         string     `json:"mac"`
		StartedAt   *time.Time `json:"started_at"`
		UpdatedAt   *time.Time `json:"updated_at"`
		SessionSecs int64      `json:"session_secs"`
		BytesIn     int64      `json:"bytes_in"`
		BytesOut    int64      `json:"bytes_out"`
	}

	rows, err := h.db.Query(ctx, `
		SELECT COALESCE(username,''), COALESCE(host(framedipaddress),''), COALESCE(callingstationid,''),
		       acctstarttime, acctupdatetime, COALESCE(acctsessiontime,0),
		       COALESCE(acctinputoctets,0), COALESCE(acctoutputoctets,0)
		FROM radacct
		WHERE nasipaddress = $1 AND acctstoptime IS NULL
		ORDER BY acctstarttime DESC NULLS LAST
		LIMIT 200
	`, d.NasIP)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "query failed")
		return
	}
	defer rows.Close()

	clients := []clientRow{}
	for rows.Next() {
		var c clientRow
		if err := rows.Scan(&c.Username, &c.IP, &c.Mac, &c.StartedAt, &c.UpdatedAt,
			&c.SessionSecs, &c.BytesIn, &c.BytesOut); err == nil {
			clients = append(clients, c)
		}
	}
	respond(w, http.StatusOK, clients)
}
