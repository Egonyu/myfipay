package handlers

import (
	"context"
	"html/template"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

type portalData struct {
	LocationName string
	District     string
	LogoURL      string
	PrimaryColor string
	Plans        []plan
	Slug         string
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
<title>{{.LocationName}} — WiFi Access</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;background:#f0f2f5;min-height:100vh}
.header{background:{{.PrimaryColor}};padding:24px 20px;text-align:center}
.logo{width:64px;height:64px;border-radius:50%;object-fit:cover;margin:0 auto 10px;display:block}
.logo-placeholder{width:64px;height:64px;border-radius:50%;background:rgba(255,255,255,.2);margin:0 auto 10px;display:flex;align-items:center;justify-content:center;font-size:28px;color:#fff}
h1{color:#fff;font-size:18px;font-weight:600}
.subtitle{color:rgba(255,255,255,.8);font-size:13px;margin-top:4px}
.card{background:#fff;border-radius:16px 16px 0 0;margin-top:-8px;padding:20px}
.section-title{font-size:11px;font-weight:600;color:#888;text-transform:uppercase;letter-spacing:.5px;margin-bottom:10px}
.plan{border:1.5px solid #e8e8e8;border-radius:10px;padding:12px 14px;margin-bottom:8px;cursor:pointer;display:flex;justify-content:space-between;align-items:center;transition:border-color .15s}
.plan.selected{border-color:{{.PrimaryColor}}}
.plan-name{font-size:14px;font-weight:600;color:#111}
.plan-desc{font-size:12px;color:#888;margin-top:2px}
.plan-price{font-size:16px;font-weight:700;color:{{.PrimaryColor}}}
.input-group{margin-bottom:14px}
label{font-size:12px;color:#666;display:block;margin-bottom:6px}
.phone-row{display:flex;background:#f7f7f7;border:1px solid #e0e0e0;border-radius:8px;overflow:hidden}
.prefix{padding:11px 12px;font-size:14px;color:#555;border-right:1px solid #e0e0e0;background:#efefef}
input[type=tel]{flex:1;border:none;background:#f7f7f7;padding:11px 12px;font-size:14px;outline:none}
.method-row{display:flex;gap:8px;margin-top:8px}
.method{flex:1;border:1.5px solid #e0e0e0;border-radius:8px;padding:8px;text-align:center;font-size:12px;color:#555;cursor:pointer}
.method.selected{border-color:{{.PrimaryColor}};color:{{.PrimaryColor}};font-weight:600}
.btn{width:100%;background:{{.PrimaryColor}};color:#fff;border:none;border-radius:10px;padding:14px;font-size:15px;font-weight:600;cursor:pointer;margin-top:8px}
.btn:active{opacity:.85}
.footer{text-align:center;padding:16px;font-size:11px;color:#aaa}
#status{display:none;padding:14px;border-radius:8px;margin-top:12px;font-size:14px;text-align:center}
.status-pending{background:#fff8e1;color:#856404}
.status-ok{background:#e8f5e9;color:#1b5e20}
.status-fail{background:#ffebee;color:#b71c1c}
</style>
</head>
<body>
<div class="header">
  {{if .LogoURL}}
  <img class="logo" src="{{.LogoURL}}" alt="{{.LocationName}}">
  {{else}}
  <div class="logo-placeholder">W</div>
  {{end}}
  <h1>{{.LocationName}}</h1>
  <div class="subtitle">{{.District}} — Select a plan to connect</div>
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
<div class="footer">Powered by myFiBase</div>

<script>
var selectedPlan = null;
var selectedMethod = 'mtn_momo';
var slug = '{{.Slug}}';

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

  if (selectedMethod === 'voucher') {
    var code = document.getElementById('voucher').value.trim();
    if (!code) { showStatus('Enter your voucher code', 'fail'); return; }
    redeemVoucher(code);
    return;
  }

  var phone = document.getElementById('phone').value.replace(/\D/g,'');
  if (phone.length < 9) { showStatus('Enter a valid phone number', 'fail'); return; }

  showStatus('Sending payment request...', 'pending');

  fetch('/portal/' + slug + '/pay', {
    method: 'POST',
    headers: {'Content-Type':'application/json'},
    body: JSON.stringify({plan_id: selectedPlan, phone: '256' + phone, method: selectedMethod})
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
    body: JSON.stringify({code: code})
  })
  .then(function(r){ return r.json(); })
  .then(function(res){
    if (res.success) {
      showStatus('Voucher accepted! Connecting...', 'ok');
      setTimeout(function(){ window.location.href = 'https://www.google.com'; }, 2000);
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
        setTimeout(function(){ window.location.href = 'https://www.google.com'; }, 2000);
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

	// TODO: load from DB by slug — using test data for now
	data := portalData{
		LocationName: "myFiBase WiFi",
		District:     "Soroti",
		LogoURL:      "",
		PrimaryColor: "#0f7a5a",
		Slug:         slug,
		Plans: []plan{
			{ID: "plan-1h", Name: "1 Hour", Description: "500 MB data", PriceUGX: 500, DurationMins: 60, DataMB: 500},
			{ID: "plan-day", Name: "All Day", Description: "2 GB data", PriceUGX: 2000, DurationMins: 1440, DataMB: 2048},
			{ID: "plan-week", Name: "Weekly", Description: "10 GB data", PriceUGX: 8000, DurationMins: 10080, DataMB: 10240},
		},
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := portalTmpl.Execute(w, data); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
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
	respond(w, http.StatusOK, []any{})
}

func (h *Handler) ListPlans(w http.ResponseWriter, r *http.Request) {
	respond(w, http.StatusOK, []any{})
}
