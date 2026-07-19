package handlers

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/myfibase/myfibase/internal/middleware"
)

type payRequest struct {
	PlanID string `json:"plan_id"`
	Phone  string `json:"phone"`
	Method string `json:"method"`
	MAC    string `json:"mac"`
	IP     string `json:"ip"`
}

type voucherRequest struct {
	Code string `json:"code"`
}

// ZengaPay webhook format: {"event":"collection.success","data":{...}}
// Amount is sent as a string "500.00" — not an integer.
type zengapayWebhookPayload struct {
	Event string       `json:"event"`
	Data  *zengapayTxn `json:"data"`
	zengapayTxn        // flat fallback for future format variants
}

type zengapayTxn struct {
	TransactionReference         string `json:"transactionReference"`
	TransactionExternalReference string `json:"transactionExternalReference"`
	TransactionStatus            string `json:"transactionStatus"`
	Amount                       string `json:"amount"`
	Phone                        string `json:"msisdn"`
}

func (h *Handler) InitiatePayment(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	_ = slug

	// Rate limit: 10 payment attempts per IP per 5 minutes. ClientIP uses the
	// nginx-set X-Real-IP — the first X-Forwarded-For element is
	// client-supplied and was previously forgeable to evade this limit.
	ctx := context.Background()
	clientIP := middleware.ClientIP(r)
	rateLimitKey := "ratelimit:pay:" + clientIP
	count, _ := h.cache.Incr(ctx, rateLimitKey).Result()
	if count == 1 {
		h.cache.Expire(ctx, rateLimitKey, 5*time.Minute)
	}
	if count > 10 {
		w.Header().Set("Retry-After", "300")
		respondError(w, http.StatusTooManyRequests, "RATE_LIMIT", "too many payment attempts — please wait 5 minutes")
		return
	}

	var req payRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_BODY", "invalid request body")
		return
	}

	if req.PlanID == "" || req.Phone == "" {
		respondError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "plan_id and phone are required")
		return
	}

	if req.Method == "" {
		req.Method = "mtn_momo"
	}

	paymentID := uuid.New().String()
	idempotencyKey := fmt.Sprintf("%s-%s-%d", req.Phone, req.PlanID, time.Now().Unix()/300)

	// Store pending payment in Redis (TTL 10 minutes)
	key := "payment:pending:" + paymentID
	mac := strings.ToLower(strings.ReplaceAll(req.MAC, "-", ":"))
	h.cache.HSet(ctx, key,
		"plan_id", req.PlanID,
		"phone", req.Phone,
		"method", req.Method,
		"status", "pending",
		"idempotency_key", idempotencyKey,
		"location_slug", slug,
		"mac", mac,
		"ip", req.IP,
	)
	h.cache.Expire(ctx, key, 10*time.Minute)

	// Call ZengaPay collections API
	if h.cfg.ZengapayAPIToken != "" {
		amount := h.getPlanPrice(req.PlanID)
		if err := h.callZengapay(ctx, paymentID, req.Phone, amount, idempotencyKey); err != nil {
			respondError(w, http.StatusBadGateway, "PAYMENT_GATEWAY_ERROR", "could not reach payment gateway")
			return
		}
	} else {
		// Dev mode: simulate pending
		h.cache.HSet(ctx, key, "zengapay_ref", "dev-"+paymentID)
	}

	respond(w, http.StatusAccepted, map[string]string{
		"payment_id": paymentID,
		"status":     "pending",
		"message":    "Check your phone for the mobile money prompt",
	})
}

func (h *Handler) PaymentStatus(w http.ResponseWriter, r *http.Request) {
	paymentID := chi.URLParam(r, "paymentID")

	ctx := context.Background()
	key := "payment:pending:" + paymentID

	status, err := h.cache.HGet(ctx, key, "status").Result()
	if err != nil {
		// Check DB for completed payments
		respond(w, http.StatusOK, map[string]string{"status": "not_found"})
		return
	}

	respond(w, http.StatusOK, map[string]string{"status": status, "payment_id": paymentID})
}

func (h *Handler) RedeemVoucher(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")

	var req struct {
		Code  string `json:"code"`
		Phone string `json:"phone"`
		MAC   string `json:"mac"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Code == "" || req.Phone == "" {
		respondError(w, http.StatusBadRequest, "INVALID_BODY", "code and phone are required")
		return
	}

	ctx := context.Background()
	code := strings.ToUpper(strings.TrimSpace(req.Code))

	// Look up the voucher
	var voucherID, planID string
	var voucherStatus string
	err := h.db.QueryRow(ctx, `
		SELECT v.id, v.plan_id, v.status
		FROM vouchers v
		JOIN locations l ON v.location_id = l.id
		WHERE v.code = $1 AND l.portal_slug = $2 LIMIT 1
	`, code, slug).Scan(&voucherID, &planID, &voucherStatus)
	if err != nil {
		respondError(w, http.StatusNotFound, "INVALID_CODE", "voucher code not found")
		return
	}
	if voucherStatus != "unused" {
		respondError(w, http.StatusConflict, "ALREADY_USED", "voucher has already been used")
		return
	}

	pl := h.getPlanFromDB(planID)
	sessionID := uuid.New().String()
	username := req.Phone
	mac := strings.ToLower(strings.ReplaceAll(req.MAC, "-", ":"))
	duration := time.Duration(pl.DurationMins) * time.Minute
	durationSecs := int(duration.Seconds())
	now := time.Now()
	expiresAt := now.Add(duration)

	// Mark voucher as used
	h.db.Exec(ctx,
		`UPDATE vouchers SET status='used', used_by_phone=$1, activated_at=NOW(), expires_at=$2 WHERE id=$3`,
		username, expiresAt, voucherID,
	)

	// Redis session
	sessionKey := "session:user:" + username
	h.cache.HSet(ctx, sessionKey,
		"session_id", sessionID, "plan_id", planID, "phone", username,
		"mac", mac, "status", "active", "started_at", now.UTC().Format(time.RFC3339), "grant_type", "voucher",
	)
	h.cache.Expire(ctx, sessionKey, duration)
	if mac != "" {
		h.cache.Set(ctx, "session:mac:"+mac, sessionID, duration)
	}

	// DB session
	h.db.Exec(ctx, `
		INSERT INTO sessions (id, location_id, plan_id, username, customer_phone, mac_address, status, started_at, expires_at)
		VALUES ($1, (SELECT id FROM locations WHERE portal_slug=$2 LIMIT 1), $3, $4, $5, NULLIF($6,''), 'active', NOW(), $7)
		ON CONFLICT DO NOTHING
	`, sessionID, slug, planID, username, username, mac, expiresAt)

	// RADIUS
	h.db.Exec(ctx, `DELETE FROM radcheck WHERE username=$1`, username)
	h.db.Exec(ctx, `INSERT INTO radcheck (username,attribute,op,value) VALUES ($1,'Auth-Type',':=','Accept')`, username)
	h.db.Exec(ctx, `DELETE FROM radreply WHERE username=$1`, username)
	rateLimit := fmt.Sprintf("%dk/%dk", pl.SpeedDownKbps, pl.SpeedUpKbps)
	h.db.Exec(ctx, `INSERT INTO radreply (username,attribute,op,value) VALUES ($1,'Session-Timeout',':=',$2)`, username, fmt.Sprintf("%d", durationSecs))
	h.db.Exec(ctx, `INSERT INTO radreply (username,attribute,op,value) VALUES ($1,'Idle-Timeout',':=','300')`, username)
	h.db.Exec(ctx, `INSERT INTO radreply (username,attribute,op,value) VALUES ($1,'Mikrotik-Rate-Limit',':=',$2)`, username, rateLimit)
	h.db.Exec(ctx, `INSERT INTO radreply (username,attribute,op,value) VALUES ($1,'WISPr-Bandwidth-Max-Down',':=',$2)`, username, fmt.Sprintf("%d", pl.SpeedDownKbps*1000))
	h.db.Exec(ctx, `INSERT INTO radreply (username,attribute,op,value) VALUES ($1,'WISPr-Bandwidth-Max-Up',':=',$2)`, username, fmt.Sprintf("%d", pl.SpeedUpKbps*1000))

	respond(w, http.StatusCreated, map[string]any{
		"session_id":   sessionID,
		"plan_name":    pl.Name,
		"duration_mins": pl.DurationMins,
		"expires_at":   expiresAt.UTC(),
	})
}

func (h *Handler) ZengapayWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}


	// Verify HMAC if secret is configured.
	// ZengaPay may send signature in any of these headers.
	if h.cfg.ZengapayWebhookSecret != "" {
		sig := r.Header.Get("X-Signature")
		if sig == "" {
			sig = r.Header.Get("X-Webhook-Signature")
		}
		if sig == "" {
			sig = r.Header.Get("X-ZengaPay-Signature")
		}
		if !verifyHMAC(body, sig, h.cfg.ZengapayWebhookSecret) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
	}

	var raw zengapayWebhookPayload
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&raw); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Unwrap nested "data" key if present, otherwise use the top-level fields.
	txn := raw.Data
	if txn == nil {
		txn = &raw.zengapayTxn
	}

	ctx := context.Background()

	// Deduplicate on ZengaPay's own transaction reference.
	dedupKey := "webhook:zengapay:" + txn.TransactionReference
	if set, _ := h.cache.SetNX(ctx, dedupKey, "1", 24*time.Hour).Result(); !set {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Our payment ID is in transactionExternalReference (what we sent as external_reference).
	paymentID := txn.TransactionExternalReference
	if paymentID == "" {
		w.WriteHeader(http.StatusOK)
		return
	}

	pendingKey := "payment:pending:" + paymentID

	switch classifyZengapayEvent(txn.TransactionStatus, raw.Event) {
	case "success":
		h.cache.HSet(ctx, pendingKey, "status", "successful", "zengapay_ref", txn.TransactionReference)
		go h.createSessionAfterPayment(paymentID, txn.Phone)
	case "failed":
		h.cache.HSet(ctx, pendingKey, "status", "failed")
	}

	w.WriteHeader(http.StatusOK)
}

func (h *Handler) createSessionAfterPayment(paymentID, phone string) {
	ctx := context.Background()
	key := "payment:pending:" + paymentID

	vals, err := h.cache.HGetAll(ctx, key).Result()
	if err != nil || len(vals) == 0 {
		return
	}

	planID := vals["plan_id"]
	locationSlug := vals["location_slug"]
	mac := vals["mac"]
	pl := h.getPlanFromDB(planID)
	sessionID := uuid.New().String()
	username := phone
	duration := time.Duration(pl.DurationMins) * time.Minute
	durationSecs := int(duration.Seconds())

	// 1. Store in Redis for instant RADIUS lookup (TTL = session duration)
	sessionKey := "session:user:" + username
	h.cache.HSet(ctx, sessionKey,
		"session_id", sessionID,
		"plan_id", planID,
		"phone", phone,
		"payment_id", paymentID,
		"mac", mac,
		"status", "active",
		"started_at", time.Now().UTC().Format(time.RFC3339),
	)
	h.cache.Expire(ctx, sessionKey, duration)

	// Link MAC → session so MikroTik can poll /portal/:slug/session-status?mac=XX:XX
	if mac != "" {
		macKey := "session:mac:" + mac
		h.cache.Set(ctx, macKey, sessionID, duration)
	}

	// 2. Persist session to PostgreSQL.
	// If planID is a real UUID it matches directly; if legacy slug, fall back to name lookup.
	h.db.Exec(ctx, `
		INSERT INTO sessions (id, location_id, plan_id, username, customer_phone, mac_address, status, started_at, expires_at)
		VALUES (
			$1,
			(SELECT id FROM locations WHERE portal_slug = $2 LIMIT 1),
			COALESCE(
				(SELECT id FROM plans WHERE id::text = $3 LIMIT 1),
				(SELECT id FROM plans WHERE name = $4 LIMIT 1)
			),
			$5, $6, NULLIF($8, ''), 'active',
			NOW(),
			NOW() + ($7 || ' seconds')::interval
		)
		ON CONFLICT DO NOTHING
	`, sessionID, locationSlug, planID, pl.Name, username, phone, fmt.Sprintf("%d", durationSecs), mac)

	// 3. Write to radcheck — FreeRADIUS grants access to any phone with this row.
	h.db.Exec(ctx, `DELETE FROM radcheck WHERE username = $1`, username)
	h.db.Exec(ctx, `
		INSERT INTO radcheck (username, attribute, op, value)
		VALUES ($1, 'Auth-Type', ':=', 'Accept')
	`, username)

	// 4. Write reply attributes — all values must be strings for the radreply value column.
	h.db.Exec(ctx, `DELETE FROM radreply WHERE username = $1`, username)

	rateLimit := fmt.Sprintf("%dk/%dk", pl.SpeedDownKbps, pl.SpeedUpKbps)
	timeoutStr := fmt.Sprintf("%d", durationSecs)
	downStr := fmt.Sprintf("%d", pl.SpeedDownKbps*1000)
	upStr := fmt.Sprintf("%d", pl.SpeedUpKbps*1000)

	h.db.Exec(ctx, `INSERT INTO radreply (username, attribute, op, value) VALUES ($1, 'Session-Timeout', ':=', $2)`, username, timeoutStr)
	h.db.Exec(ctx, `INSERT INTO radreply (username, attribute, op, value) VALUES ($1, 'Idle-Timeout', ':=', '300')`, username)
	h.db.Exec(ctx, `INSERT INTO radreply (username, attribute, op, value) VALUES ($1, 'Mikrotik-Rate-Limit', ':=', $2)`, username, rateLimit)
	h.db.Exec(ctx, `INSERT INTO radreply (username, attribute, op, value) VALUES ($1, 'WISPr-Bandwidth-Max-Down', ':=', $2)`, username, downStr)
	h.db.Exec(ctx, `INSERT INTO radreply (username, attribute, op, value) VALUES ($1, 'WISPr-Bandwidth-Max-Up', ':=', $2)`, username, upStr)

	// 5. Persist the confirmed mobile-money payment for audit/reporting (previously Redis-only).
	// RETURNING id gives us the row actually written — on a duplicate webhook the ON CONFLICT
	// suppresses the insert and returns no rows, which correctly suppresses the commission too.
	var confirmedPaymentID string
	err = h.db.QueryRow(ctx, `
		INSERT INTO payments (id, location_id, plan_id, customer_phone, amount_ugx, method, status, zengapay_ref, initiated_at, confirmed_at, metadata)
		VALUES (
			$1,
			(SELECT id FROM locations WHERE portal_slug = $2 LIMIT 1),
			COALESCE(
				(SELECT id FROM plans WHERE id::text = $3 LIMIT 1),
				(SELECT id FROM plans WHERE name = $4 LIMIT 1)
			),
			$5, $6, 'mobile_money', 'confirmed', NULLIF($7, ''), NOW(), NOW(),
			jsonb_build_object('payment_id', $8::text, 'session_id', $9::text)
		)
		ON CONFLICT (zengapay_ref) DO NOTHING
		RETURNING id
	`, uuid.New().String(), locationSlug, planID, pl.Name, phone, pl.PriceUGX, vals["zengapay_ref"], paymentID, sessionID).Scan(&confirmedPaymentID)
	if err == nil {
		h.createAgentCommission(ctx, locationSlug, confirmedPaymentID, pl.PriceUGX)
	}

	// Session cleanup is handled by StartSessionReaper (polls expires_at) — no per-session goroutine.
}

// createAgentCommission credits the referring agent for a specific confirmed payment.
// paymentID must be the row just written, so concurrent payments at the same location
// each credit their own commission. Silently skips if the operator has no referring agent.
func (h *Handler) createAgentCommission(ctx context.Context, locationSlug, paymentID string, amountUGX int) {
	var agentID, operatorID string
	err := h.db.QueryRow(ctx, `
		SELECT ar.agent_id, ar.operator_id
		FROM locations l
		JOIN agent_referrals ar ON ar.operator_id = l.tenant_id
		WHERE l.portal_slug = $1
		LIMIT 1
	`, locationSlug).Scan(&agentID, &operatorID)
	if err != nil {
		return
	}

	commissionUGX := agentCommission(amountUGX, agentCommissionRate)
	if commissionUGX == 0 {
		return
	}

	h.db.Exec(ctx, `
		INSERT INTO commissions (agent_id, operator_id, payment_id, amount_ugx, rate_pct)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (payment_id) DO NOTHING
	`, agentID, operatorID, paymentID, commissionUGX, agentCommissionRate*100)
}

func (h *Handler) expireSession(username, sessionID string) {
	ctx := context.Background()
	h.db.Exec(ctx, `UPDATE sessions SET status='expired', terminated_at=NOW() WHERE id=$1`, sessionID)
	h.db.Exec(ctx, `DELETE FROM radcheck WHERE username=$1`, username)
	h.db.Exec(ctx, `DELETE FROM radreply WHERE username=$1`, username)

	// Clean up MAC key if present
	sessionKey := "session:user:" + username
	mac, _ := h.cache.HGet(ctx, sessionKey, "mac").Result()
	if mac != "" {
		h.cache.Del(ctx, "session:mac:"+mac)
	}
	h.cache.Del(ctx, sessionKey)
}

func (h *Handler) callZengapay(ctx context.Context, paymentID, phone string, amount int, _ string) error {
	// ZengaPay collection fields (from adapter: msisdn, amount, external_reference, narration).
	// Webhook URL is configured globally in the ZengaPay dashboard — not per-request.
	payload := map[string]any{
		"msisdn":             phone,
		"amount":             amount,
		"external_reference": paymentID,
		"narration":          "myFiBase WiFi access",
	}

	body, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		h.cfg.ZengapayAPIURL+"/v1/collections", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+h.cfg.ZengapayAPIToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("zengapay %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

type planRecord struct {
	ID            string
	Name          string
	PriceUGX      int
	DurationMins  int
	SpeedDownKbps int
	SpeedUpKbps   int
}

// getPlanFromDB fetches plan fields from the database by UUID.
// Falls back to legacy slug-based hardcoded values if the ID is a legacy slug
// or the DB lookup fails, so live sessions created before the migration still work.
func (h *Handler) getPlanFromDB(planID string) planRecord {
	ctx := context.Background()
	var p planRecord
	err := h.db.QueryRow(ctx, `
		SELECT id, name, price_ugx, duration_mins,
		       COALESCE(speed_down_kbps, 2048), COALESCE(speed_up_kbps, 512)
		FROM plans WHERE id = $1 LIMIT 1
	`, planID).Scan(&p.ID, &p.Name, &p.PriceUGX, &p.DurationMins, &p.SpeedDownKbps, &p.SpeedUpKbps)
	if err == nil {
		return p
	}
	// Legacy fallback for hardcoded plan slugs still in-flight in Redis
	legacy := map[string]planRecord{
		"plan-1h":   {ID: planID, Name: "1 Hour", PriceUGX: 500, DurationMins: 60, SpeedDownKbps: 2048, SpeedUpKbps: 512},
		"plan-day":  {ID: planID, Name: "All Day", PriceUGX: 2000, DurationMins: 1440, SpeedDownKbps: 5120, SpeedUpKbps: 1024},
		"plan-week": {ID: planID, Name: "Weekly", PriceUGX: 8000, DurationMins: 10080, SpeedDownKbps: 10240, SpeedUpKbps: 2048},
	}
	if r, ok := legacy[planID]; ok {
		return r
	}
	return planRecord{ID: planID, Name: "Plan", PriceUGX: 500, DurationMins: 60, SpeedDownKbps: 2048, SpeedUpKbps: 512}
}

func (h *Handler) getPlanPrice(planID string) int {
	return h.getPlanFromDB(planID).PriceUGX
}

func (h *Handler) getPlanDuration(planID string) time.Duration {
	return time.Duration(h.getPlanFromDB(planID).DurationMins) * time.Minute
}

func (h *Handler) getPlanSpeeds(planID string) (down, up int) {
	p := h.getPlanFromDB(planID)
	return p.SpeedDownKbps, p.SpeedUpKbps
}

func verifyHMAC(body []byte, signature, secret string) bool {
	// Try the secret as-is and also as hex-decoded binary (ZengaPay may store it either way).
	secrets := [][]byte{[]byte(secret)}
	if decoded, err := hex.DecodeString(secret); err == nil {
		secrets = append(secrets, decoded)
	}

	sigLower := strings.ToLower(strings.TrimSpace(signature))

	for _, key := range secrets {
		mac := hmac.New(sha256.New, key)
		mac.Write(body)
		expected := hex.EncodeToString(mac.Sum(nil))
		if hmac.Equal([]byte(expected), []byte(sigLower)) {
			return true
		}
	}
	return false
}
