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
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type payRequest struct {
	PlanID string `json:"plan_id"`
	Phone  string `json:"phone"`
	Method string `json:"method"`
}

type voucherRequest struct {
	Code string `json:"code"`
}

type zengapayWebhookPayload struct {
	Reference string `json:"reference"`
	Status    string `json:"status"`
	Amount    int    `json:"amount"`
	Phone     string `json:"msisdn"`
	Metadata  map[string]string `json:"metadata"`
}

func (h *Handler) InitiatePayment(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	_ = slug

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
	ctx := context.Background()
	key := "payment:pending:" + paymentID
	h.cache.HSet(ctx, key,
		"plan_id", req.PlanID,
		"phone", req.Phone,
		"method", req.Method,
		"status", "pending",
		"idempotency_key", idempotencyKey,
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
	var req voucherRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Code == "" {
		respondError(w, http.StatusBadRequest, "INVALID_BODY", "code is required")
		return
	}

	// TODO: query vouchers table, validate, create session
	respondError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "voucher redemption coming soon")
}

func (h *Handler) ZengapayWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Verify HMAC signature if secret is configured
	if h.cfg.ZengapayWebhookSecret != "" {
		sig := r.Header.Get("X-ZengaPay-Signature")
		if !verifyHMAC(body, sig, h.cfg.ZengapayWebhookSecret) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
	}

	var payload zengapayWebhookPayload
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&payload); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	ctx := context.Background()

	// Deduplicate: ignore if already processed
	dedupKey := "webhook:zengapay:" + payload.Reference
	if set, _ := h.cache.SetNX(ctx, dedupKey, "1", 24*time.Hour).Result(); !set {
		w.WriteHeader(http.StatusOK)
		return
	}

	paymentID := payload.Metadata["payment_id"]
	if paymentID == "" {
		w.WriteHeader(http.StatusOK)
		return
	}

	pendingKey := "payment:pending:" + paymentID

	switch payload.Status {
	case "SUCCESSFUL":
		h.cache.HSet(ctx, pendingKey, "status", "successful", "zengapay_ref", payload.Reference)
		// Create session in DB
		go h.createSessionAfterPayment(paymentID, payload.Phone)
	case "FAILED":
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
	sessionID := uuid.New().String()
	username := phone

	// Store session in Redis for fast RADIUS lookup
	sessionKey := "session:user:" + username
	h.cache.HSet(ctx, sessionKey,
		"session_id", sessionID,
		"plan_id", planID,
		"phone", phone,
		"payment_id", paymentID,
		"status", "active",
		"started_at", time.Now().UTC().Format(time.RFC3339),
	)

	duration := h.getPlanDuration(planID)
	h.cache.Expire(ctx, sessionKey, duration)

	// TODO: persist to PostgreSQL sessions table
}

func (h *Handler) callZengapay(ctx context.Context, paymentID, phone string, amount int, idempotencyKey string) error {
	payload := map[string]any{
		"amount":         amount,
		"currency":       "UGX",
		"msisdn":         phone,
		"description":    "myFiBase WiFi access",
		"reference":      paymentID,
		"callback_url":   "https://api.myfibase.ug/webhooks/zengapay",
		"metadata": map[string]string{
			"payment_id": paymentID,
		},
	}

	body, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		h.cfg.ZengapayAPIURL+"/v1/collections", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+h.cfg.ZengapayAPIToken)
	req.Header.Set("Idempotency-Key", idempotencyKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("zengapay returned %d", resp.StatusCode)
	}
	return nil
}

func (h *Handler) getPlanPrice(planID string) int {
	prices := map[string]int{
		"plan-1h":   500,
		"plan-day":  2000,
		"plan-week": 8000,
	}
	if p, ok := prices[planID]; ok {
		return p
	}
	return 500
}

func (h *Handler) getPlanDuration(planID string) time.Duration {
	durations := map[string]time.Duration{
		"plan-1h":   1 * time.Hour,
		"plan-day":  24 * time.Hour,
		"plan-week": 7 * 24 * time.Hour,
	}
	if d, ok := durations[planID]; ok {
		return d
	}
	return 1 * time.Hour
}

func verifyHMAC(body []byte, signature, secret string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}
