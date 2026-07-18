package handlers

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
)

type portalData struct {
	LocationName string
	PortalName   string // operator-branded name, e.g. "Soroti Market WiFi"
	Tagline      string // e.g. "Fast. Affordable. Always on."
	District     string
	LogoURL      string
	PrimaryColor string
	Plans        []plan
	Slug         string
	MAC          string
	IP           string
	LoginURL     string // hotspot $(link-login-only) — where to POST credentials after payment
	DisplayName  string // computed: PortalName if set, else LocationName
	Initial      string // first letter of DisplayName for logo placeholder
}

type plan struct {
	ID          string
	Name        string
	Description string
	PriceUGX    int
	DurationMins int
	DataMB      int
}

var portalTmpl = template.Must(template.New("portal").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{.DisplayName}} — WiFi</title>
<style>
:root{--accent:{{.PrimaryColor}}}
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;background:#f0f2f5;min-height:100vh;max-width:480px;margin:0 auto}
.header{background:var(--accent);padding:28px 20px 36px;text-align:center;position:relative}
.logo-wrap{width:72px;height:72px;border-radius:16px;overflow:hidden;margin:0 auto 12px;background:rgba(255,255,255,.15);display:flex;align-items:center;justify-content:center}
.logo-wrap img{width:100%;height:100%;object-fit:cover}
.logo-initial{font-size:32px;font-weight:800;color:#fff;line-height:1}
h1{color:#fff;font-size:20px;font-weight:700;letter-spacing:-.3px}
.tagline{color:rgba(255,255,255,.8);font-size:13px;margin-top:5px}
.card{background:#fff;border-radius:20px 20px 0 0;margin-top:-16px;padding:22px 18px 32px;min-height:60vh}
.section-title{font-size:10px;font-weight:700;color:#aaa;text-transform:uppercase;letter-spacing:.8px;margin-bottom:10px}
.plan{border:2px solid #eee;border-radius:12px;padding:13px 15px;margin-bottom:9px;cursor:pointer;display:flex;justify-content:space-between;align-items:center;transition:all .15s;background:#fafafa}
.plan:active{opacity:.8}
.plan.selected{border-color:var(--accent);background:#fff}
.plan-name{font-size:14px;font-weight:700;color:#111}
.plan-desc{font-size:12px;color:#999;margin-top:2px}
.plan-price{font-size:17px;font-weight:800;color:var(--accent);white-space:nowrap}
.plan-price span{font-size:10px;font-weight:500;color:#aaa;display:block;text-align:right}
.input-group{margin-bottom:14px}
label{font-size:12px;font-weight:600;color:#666;display:block;margin-bottom:7px}
.phone-row{display:flex;background:#f5f5f5;border:1.5px solid #e8e8e8;border-radius:10px;overflow:hidden}
.prefix{padding:12px 13px;font-size:14px;color:#555;border-right:1.5px solid #e8e8e8;background:#eee;font-weight:600}
input[type=tel],input[type=text]{flex:1;border:none;background:transparent;padding:12px 13px;font-size:14px;outline:none;color:#111}
.method-row{display:flex;gap:8px}
.method{flex:1;border:1.5px solid #e0e0e0;border-radius:10px;padding:9px 6px;text-align:center;font-size:12px;color:#777;cursor:pointer;transition:all .15s;background:#fafafa}
.method.selected{border-color:var(--accent);color:var(--accent);font-weight:700;background:#fff}
.btn{width:100%;background:var(--accent);color:#fff;border:none;border-radius:12px;padding:15px;font-size:15px;font-weight:700;cursor:pointer;margin-top:10px;letter-spacing:.1px}
.btn:disabled{opacity:.6;cursor:default}
.btn:active:not(:disabled){opacity:.85}
.footer{text-align:center;padding:20px 0 12px;font-size:11px;color:#bbb}
.footer a{color:#bbb;text-decoration:none}
#status{display:none;padding:14px 16px;border-radius:10px;margin-top:12px;font-size:13px;text-align:center;font-weight:500}
.status-pending{background:#fff8e1;color:#856404}
.status-ok{background:#e8f5e9;color:#1b5e20}
.status-fail{background:#ffebee;color:#b71c1c}
</style>
</head>
<body>
<div class="header">
  <div class="logo-wrap">
    {{if .LogoURL}}<img src="{{.LogoURL}}" alt="{{.DisplayName}}">
    {{else}}<span class="logo-initial">{{.Initial}}</span>{{end}}
  </div>
  <h1>{{.DisplayName}}</h1>
  <div class="tagline">{{if .Tagline}}{{.Tagline}}{{else}}Select a plan to get online{{end}}</div>
</div>
<div class="card">
  <div class="section-title">Choose plan</div>
  {{range .Plans}}
  <div class="plan" onclick="selectPlan(this,'{{.ID}}','{{.PriceUGX}}')" data-id="{{.ID}}">
    <div>
      <div class="plan-name">{{.Name}}</div>
      <div class="plan-desc">{{.Description}}</div>
    </div>
    <div class="plan-price">{{.PriceUGX}} UGX</div>
  </div>
  {{end}}

  <div class="input-group" style="margin-top:18px">
    <label>Mobile money number</label>
    <div class="phone-row">
      <span class="prefix">+256</span>
      <input type="tel" id="phone" placeholder="7XX XXX XXX" maxlength="9" inputmode="numeric">
    </div>
  </div>

  <div class="input-group">
    <label>Payment method</label>
    <div class="method-row">
      <div class="method selected" onclick="selectMethod(this,'mtn_momo')">MTN MoMo</div>
      <div class="method" onclick="selectMethod(this,'airtel_money')">Airtel Money</div>
      <div class="method" onclick="selectMethod(this,'voucher')">Voucher</div>
    </div>
  </div>

  <div id="voucher-input" style="display:none;margin-bottom:14px">
    <label>Voucher code</label>
    <input type="text" id="voucher" placeholder="AB3-KX9-72P" style="width:100%;border:1px solid #e0e0e0;border-radius:8px;padding:11px 12px;font-size:14px">
  </div>

  <button class="btn" onclick="pay()">Connect — <span id="btn-amount">Select a plan</span></button>
  <div id="status"></div>
</div>
<div class="footer">Powered by <a href="https://myfibase.ug">myFiBase</a></div>

<script>
var selectedPlan = null;
var selectedMethod = 'mtn_momo';
var slug = '{{.Slug}}';
var deviceMAC = '{{.MAC}}';
var deviceIP  = '{{.IP}}';
var loginURL  = '{{.LoginURL}}';
var lastPhone = '';

// After a grant, log the device into the hotspot: the router sends our RADIUS
// server an Access-Request for this username (radcheck Auth-Type Accept), then
// opens the gate. Without a router login URL (e.g. direct browser visit), just
// land somewhere neutral.
function connectClient(username) {
  if (loginURL) {
    window.location.href = loginURL + (loginURL.indexOf('?') === -1 ? '?' : '&') +
      'username=' + encodeURIComponent(username) + '&password=connect';
  } else {
    window.location.href = 'https://www.google.com';
  }
}

function selectPlan(el, id, price) {
  document.querySelectorAll('.plan').forEach(function(p){ p.classList.remove('selected'); });
  el.classList.add('selected');
  selectedPlan = id;
  document.getElementById('btn-amount').textContent = price + ' UGX';
}

function selectMethod(el, method) {
  document.querySelectorAll('.method').forEach(function(m){ m.classList.remove('selected'); });
  el.classList.add('selected');
  selectedMethod = method;
  document.getElementById('voucher-input').style.display = method === 'voucher' ? 'block' : 'none';
}

function pay() {
  if (!selectedPlan) { showStatus('Select a plan first', 'fail'); return; }

  var phone = document.getElementById('phone').value.replace(/\D/g,'');
  if (phone.length < 9) { showStatus('Enter a valid phone number', 'fail'); return; }
  lastPhone = '256' + phone;

  if (selectedMethod === 'voucher') {
    var code = document.getElementById('voucher').value.trim();
    if (!code) { showStatus('Enter your voucher code', 'fail'); return; }
    redeemVoucher(code);
    return;
  }

  showStatus('Sending payment request...', 'pending');

  fetch('/portal/' + slug + '/pay', {
    method: 'POST',
    headers: {'Content-Type':'application/json'},
    body: JSON.stringify({plan_id: selectedPlan, phone: '256' + phone, method: selectedMethod, mac: deviceMAC, ip: deviceIP})
  })
  .then(function(r){ return r.json(); })
  .then(function(res){
    if (res.success) {
      showStatus('Check your phone for the MoMo prompt. Approve to connect.', 'pending');
      pollStatus(res.data.payment_id);
    } else {
      showStatus(res.error.message || 'Payment failed', 'fail');
    }
  })
  .catch(function(){ showStatus('Network error. Try again.', 'fail'); });
}

function redeemVoucher(code) {
  showStatus('Checking voucher...', 'pending');
  fetch('/portal/' + slug + '/voucher', {
    method: 'POST',
    headers: {'Content-Type':'application/json'},
    body: JSON.stringify({code: code, phone: lastPhone, mac: deviceMAC})
  })
  .then(function(r){ return r.json(); })
  .then(function(res){
    if (res.success) {
      showStatus('Voucher accepted! Connecting...', 'ok');
      setTimeout(function(){ connectClient(lastPhone); }, 2000);
    } else {
      showStatus(res.error.message || 'Invalid voucher', 'fail');
    }
  });
}

function pollStatus(paymentID) {
  var attempts = 0;
  var interval = setInterval(function(){
    attempts++;
    if (attempts > 30) {
      clearInterval(interval);
      showStatus('Payment timed out. Try again.', 'fail');
      return;
    }
    fetch('/portal/' + slug + '/pay/' + paymentID + '/status')
    .then(function(r){ return r.json(); })
    .then(function(res){
      if (!res.success) return;
      var s = res.data.status;
      if (s === 'successful') {
        clearInterval(interval);
        showStatus('Payment confirmed! You are now connected.', 'ok');
        setTimeout(function(){ connectClient(lastPhone); }, 2000);
      } else if (s === 'failed') {
        clearInterval(interval);
        showStatus('Payment declined. Please try again.', 'fail');
      }
    });
  }, 3000);
}

function showStatus(msg, type) {
  var el = document.getElementById('status');
  el.textContent = msg;
  el.className = 'status-' + type;
  el.style.display = 'block';
}
</script>
</body>
</html>`))

func (h *Handler) PortalPage(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	ctx := context.Background()

	// Load location + branding by portal_slug
	var locName, district, primaryColor, portalName, tagline, logoURL string
	err := h.db.QueryRow(ctx, `
		SELECT name,
		       COALESCE(district, address, ''),
		       COALESCE(branding->>'primary_color', '#2563eb'),
		       COALESCE(branding->>'portal_name', ''),
		       COALESCE(branding->>'tagline', ''),
		       COALESCE(branding->>'logo_url', '')
		FROM locations WHERE portal_slug = $1 AND status = 'active' LIMIT 1
	`, slug).Scan(&locName, &district, &primaryColor, &portalName, &tagline, &logoURL)
	if err != nil {
		locName = "myFiBase WiFi"
		district = ""
		primaryColor = "#2563eb"
		portalName = ""
		tagline = ""
		logoURL = ""
	}

	// Load active plans for this location
	rows, err := h.db.Query(ctx, `
		SELECT p.id, p.name, p.price_ugx, p.duration_mins, COALESCE(p.speed_down_kbps, 2048)
		FROM plans p
		JOIN locations l ON l.id = p.location_id
		WHERE l.portal_slug = $1 AND p.active = TRUE
		ORDER BY p.price_ugx ASC
	`, slug)

	var plans []plan
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var pl plan
			var speedDown int
			if err := rows.Scan(&pl.ID, &pl.Name, &pl.PriceUGX, &pl.DurationMins, &speedDown); err == nil {
				pl.Description = planDescription(pl.DurationMins, speedDown)
				plans = append(plans, pl)
			}
		}
	}

	// Fallback so the page is never blank
	if len(plans) == 0 {
		plans = []plan{
			{ID: "plan-1h", Name: "1 Hour", Description: "High speed access", PriceUGX: 500, DurationMins: 60},
			{ID: "plan-day", Name: "All Day", Description: "Full day access", PriceUGX: 2000, DurationMins: 1440},
			{ID: "plan-week", Name: "Weekly", Description: "7-day access", PriceUGX: 8000, DurationMins: 10080},
		}
	}

	mac := strings.ToLower(r.URL.Query().Get("mac"))
	ip := r.URL.Query().Get("ip")

	// Router login URL (MikroTik $(link-login-only)); only accept plain http(s)
	// URLs so nothing else can be injected into the page.
	loginURL := r.URL.Query().Get("login")
	if u, err := url.Parse(loginURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		loginURL = ""
	}

	displayName := locName
	if portalName != "" {
		displayName = portalName
	}
	initial := "W"
	if len(displayName) > 0 {
		initial = strings.ToUpper(string([]rune(displayName)[0]))
	}

	data := portalData{
		LocationName: locName,
		PortalName:   portalName,
		Tagline:      tagline,
		District:     district,
		LogoURL:      logoURL,
		PrimaryColor: primaryColor,
		Slug:         slug,
		Plans:        plans,
		MAC:          mac,
		IP:           ip,
		LoginURL:     loginURL,
		DisplayName:  displayName,
		Initial:      initial,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := portalTmpl.Execute(w, data); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

// planDescription generates a human-readable description from duration and speed.
func planDescription(durationMins, speedDownKbps int) string {
	var dur string
	switch {
	case durationMins >= 10080:
		dur = fmt.Sprintf("%d day", durationMins/1440)
	case durationMins >= 1440:
		dur = "full day"
	case durationMins >= 60:
		dur = fmt.Sprintf("%dhr", durationMins/60)
	default:
		dur = fmt.Sprintf("%dmin", durationMins)
	}
	speed := "high speed"
	if speedDownKbps >= 5120 {
		speed = fmt.Sprintf("%d Mbps", speedDownKbps/1024)
	} else if speedDownKbps >= 1024 {
		speed = fmt.Sprintf("%d Mbps", speedDownKbps/1024)
	}
	return dur + " · " + speed
}

func (h *Handler) SessionStatus(w http.ResponseWriter, r *http.Request) {
	mac := r.URL.Query().Get("mac")
	if mac == "" {
		respondError(w, http.StatusBadRequest, "MISSING_MAC", "mac query param required")
		return
	}

	ctx := context.Background()
	key := "session:mac:" + strings.ToLower(mac)
	val, err := h.cache.Get(ctx, key).Result()
	if err != nil {
		respond(w, http.StatusOK, map[string]any{"active": false})
		return
	}
	respond(w, http.StatusOK, map[string]any{"active": true, "session_id": val})
}

func (h *Handler) ListLocations(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	rows, err := h.db.Query(ctx, `SELECT id, name, address, portal_slug, status FROM locations WHERE status = 'active' ORDER BY name`)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "could not fetch locations")
		return
	}
	defer rows.Close()
	type loc struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		Address    string `json:"address"`
		PortalSlug string `json:"portal_slug"`
		Status     string `json:"status"`
	}
	var locs []loc
	for rows.Next() {
		var l loc
		if err := rows.Scan(&l.ID, &l.Name, &l.Address, &l.PortalSlug, &l.Status); err == nil {
			locs = append(locs, l)
		}
	}
	if locs == nil {
		locs = []loc{}
	}
	respond(w, http.StatusOK, locs)
}

func (h *Handler) ListPlans(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	ctx := context.Background()
	rows, err := h.db.Query(ctx, `
		SELECT p.id, p.name, p.price_ugx, p.duration_mins,
		       COALESCE(p.speed_down_kbps, 2048), COALESCE(p.speed_up_kbps, 512)
		FROM plans p JOIN locations l ON l.id = p.location_id
		WHERE l.portal_slug = $1 AND p.active = TRUE
		ORDER BY p.price_ugx ASC
	`, slug)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "could not fetch plans")
		return
	}
	defer rows.Close()
	type planRow struct {
		ID            string `json:"id"`
		Name          string `json:"name"`
		PriceUGX      int    `json:"price_ugx"`
		DurationMins  int    `json:"duration_minutes"`
		SpeedDownKbps int    `json:"speed_down_kbps"`
		SpeedUpKbps   int    `json:"speed_up_kbps"`
	}
	var plans []planRow
	for rows.Next() {
		var p planRow
		if err := rows.Scan(&p.ID, &p.Name, &p.PriceUGX, &p.DurationMins, &p.SpeedDownKbps, &p.SpeedUpKbps); err == nil {
			plans = append(plans, p)
		}
	}
	if plans == nil {
		plans = []planRow{}
	}
	respond(w, http.StatusOK, plans)
}
