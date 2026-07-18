package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/myfibase/myfibase/internal/middleware"
)

// defaultCommissionRate is the platform's cut of mobile-money revenue (starter tier).
// Per-tenant override lives in tenants.settings->>'commission_rate'.
const defaultCommissionRate = 0.08

// minPayoutUGX is the floor for a withdrawal request.
const minPayoutUGX = 5000

// tenantBalance computes a tenant's withdrawable mobile-money balance.
// Only mobile_money payments are platform-held; cash is collected directly by the operator.
// net = grossMobileMoney * (1 - commission) - (pending + approved + paid payouts).
func (h *Handler) tenantBalance(ctx context.Context, tenantID string) (gross, commission, withdrawn, available int, rate float64) {
	// Commission rate (per-tenant override, else default).
	rate = defaultCommissionRate
	var rateStr string
	h.db.QueryRow(ctx,
		`SELECT COALESCE(settings->>'commission_rate', '') FROM tenants WHERE id = $1`,
		tenantID).Scan(&rateStr)
	if rateStr != "" {
		if parsed, err := strconv.ParseFloat(rateStr, 64); err == nil && parsed >= 0 && parsed < 1 {
			rate = parsed
		}
	}

	// Gross mobile-money revenue confirmed for this tenant's locations.
	h.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(pm.amount_ugx), 0)
		FROM payments pm
		JOIN locations l ON pm.location_id = l.id
		WHERE l.tenant_id = $1 AND pm.status = 'confirmed' AND pm.method = 'mobile_money'
	`, tenantID).Scan(&gross)

	// Amounts already claimed via payouts (anything not rejected ties up balance).
	h.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount_ugx), 0)
		FROM payouts
		WHERE tenant_id = $1 AND status IN ('pending', 'approved', 'paid')
	`, tenantID).Scan(&withdrawn)

	commission = int(float64(gross) * rate)
	available = gross - commission - withdrawn
	if available < 0 {
		available = 0
	}
	return
}

// GetPayoutBalance returns the withdrawable balance breakdown for the operator.
func (h *Handler) GetPayoutBalance(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r)
	ctx := context.Background()

	gross, commission, withdrawn, available, rate := h.tenantBalance(ctx, claims.TenantID)
	respond(w, http.StatusOK, map[string]any{
		"gross_mobile_money_ugx": gross,
		"commission_ugx":         commission,
		"commission_rate":        rate,
		"already_requested_ugx":  withdrawn,
		"available_ugx":          available,
		"min_payout_ugx":         minPayoutUGX,
	})
}

// ListPayouts returns the operator's own payout history.
func (h *Handler) ListPayouts(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r)
	ctx := context.Background()

	rows, err := h.db.Query(ctx, `
		SELECT id, amount_ugx, momo_phone, COALESCE(momo_name,''), status,
		       COALESCE(reference,''), COALESCE(note,''), COALESCE(rejection_reason,''),
		       requested_at, paid_at
		FROM payouts
		WHERE tenant_id = $1
		ORDER BY requested_at DESC
		LIMIT 200
	`, claims.TenantID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "query failed")
		return
	}
	defer rows.Close()

	type payout struct {
		ID              string     `json:"id"`
		AmountUGX       int        `json:"amount_ugx"`
		MomoPhone       string     `json:"momo_phone"`
		MomoName        string     `json:"momo_name"`
		Status          string     `json:"status"`
		Reference       string     `json:"reference"`
		Note            string     `json:"note"`
		RejectionReason string     `json:"rejection_reason"`
		RequestedAt     time.Time  `json:"requested_at"`
		PaidAt          *time.Time `json:"paid_at"`
	}
	var payouts []payout
	for rows.Next() {
		var p payout
		rows.Scan(&p.ID, &p.AmountUGX, &p.MomoPhone, &p.MomoName, &p.Status,
			&p.Reference, &p.Note, &p.RejectionReason, &p.RequestedAt, &p.PaidAt)
		payouts = append(payouts, p)
	}
	if payouts == nil {
		payouts = []payout{}
	}
	respond(w, http.StatusOK, payouts)
}

// RequestOperatorPayout lets an operator request a withdrawal of their available balance.
func (h *Handler) RequestOperatorPayout(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r)
	ctx := context.Background()

	var req struct {
		AmountUGX int    `json:"amount_ugx"`
		MomoPhone string `json:"momo_phone"`
		MomoName  string `json:"momo_name"`
		Note      string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
		return
	}
	req.MomoPhone = strings.TrimSpace(req.MomoPhone)
	if req.MomoPhone == "" {
		respondError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "momo_phone is required")
		return
	}
	if req.AmountUGX < minPayoutUGX {
		respondError(w, http.StatusUnprocessableEntity, "AMOUNT_TOO_LOW",
			"minimum withdrawal is UGX 5,000")
		return
	}

	// Re-check available balance at request time to prevent over-withdrawal.
	_, _, _, available, _ := h.tenantBalance(ctx, claims.TenantID)
	if req.AmountUGX > available {
		respondError(w, http.StatusUnprocessableEntity, "INSUFFICIENT_BALANCE",
			"requested amount exceeds available balance")
		return
	}

	id := uuid.New().String()
	_, err := h.db.Exec(ctx, `
		INSERT INTO payouts (id, tenant_id, requested_by, amount_ugx, momo_phone, momo_name, note, status)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6,''), NULLIF($7,''), 'pending')
	`, id, claims.TenantID, claims.UserID, req.AmountUGX, req.MomoPhone, req.MomoName, req.Note)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "could not create payout request")
		return
	}

	respond(w, http.StatusCreated, map[string]any{
		"id":         id,
		"amount_ugx": req.AmountUGX,
		"status":     "pending",
	})
}

// ─── Admin payout queue ─────────────────────────────────────────────────────────

// ListPayoutQueue returns payouts across all tenants for the admin, optionally filtered by status.
func (h *Handler) ListPayoutQueue(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	status := r.URL.Query().Get("status") // pending | approved | paid | rejected | '' (all)

	query := `
		SELECT po.id, po.tenant_id, t.name AS tenant_name, po.amount_ugx,
		       po.momo_phone, COALESCE(po.momo_name,''), po.status,
		       COALESCE(po.note,''), COALESCE(po.rejection_reason,''),
		       po.requested_at, po.paid_at
		FROM payouts po
		JOIN tenants t ON po.tenant_id = t.id`
	args := []any{}
	if status != "" {
		query += ` WHERE po.status = $1`
		args = append(args, status)
	}
	query += ` ORDER BY po.requested_at DESC LIMIT 300`

	rows, err := h.db.Query(ctx, query, args...)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "query failed")
		return
	}
	defer rows.Close()

	type payout struct {
		ID              string     `json:"id"`
		TenantID        string     `json:"tenant_id"`
		TenantName      string     `json:"tenant_name"`
		AmountUGX       int        `json:"amount_ugx"`
		MomoPhone       string     `json:"momo_phone"`
		MomoName        string     `json:"momo_name"`
		Status          string     `json:"status"`
		Note            string     `json:"note"`
		RejectionReason string     `json:"rejection_reason"`
		RequestedAt     time.Time  `json:"requested_at"`
		PaidAt          *time.Time `json:"paid_at"`
	}
	var payouts []payout
	for rows.Next() {
		var p payout
		rows.Scan(&p.ID, &p.TenantID, &p.TenantName, &p.AmountUGX,
			&p.MomoPhone, &p.MomoName, &p.Status, &p.Note, &p.RejectionReason,
			&p.RequestedAt, &p.PaidAt)
		payouts = append(payouts, p)
	}
	if payouts == nil {
		payouts = []payout{}
	}
	respond(w, http.StatusOK, payouts)
}

// ApprovePayout marks a pending payout as approved (cleared for disbursement).
func (h *Handler) ApprovePayout(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r)
	id := chi.URLParam(r, "id")
	ctx := context.Background()

	tag, err := h.db.Exec(ctx, `
		UPDATE payouts SET status = 'approved', reviewed_by = $2, reviewed_at = NOW()
		WHERE id = $1 AND status = 'pending'
	`, id, claims.UserID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "update failed")
		return
	}
	if tag.RowsAffected() == 0 {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "pending payout not found")
		return
	}
	respond(w, http.StatusOK, map[string]string{"status": "approved"})
}

// RejectPayout rejects a pending or approved payout, freeing the reserved balance.
func (h *Handler) RejectPayout(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r)
	id := chi.URLParam(r, "id")
	ctx := context.Background()

	var req struct {
		Reason string `json:"reason"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	tag, err := h.db.Exec(ctx, `
		UPDATE payouts SET status = 'rejected', rejection_reason = NULLIF($2,''),
		       reviewed_by = $3, reviewed_at = NOW()
		WHERE id = $1 AND status IN ('pending', 'approved')
	`, id, req.Reason, claims.UserID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "update failed")
		return
	}
	if tag.RowsAffected() == 0 {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "payout not found or already settled")
		return
	}
	respond(w, http.StatusOK, map[string]string{"status": "rejected"})
}

// MarkOperatorPayoutPaid marks an approved operator payout as paid (after ZengaPay disbursement).
// reference is the disbursement transaction reference for the audit trail.
func (h *Handler) MarkOperatorPayoutPaid(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx := context.Background()

	var req struct {
		Reference string `json:"reference"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	tag, err := h.db.Exec(ctx, `
		UPDATE payouts SET status = 'paid', reference = NULLIF($2,''), paid_at = NOW()
		WHERE id = $1 AND status = 'approved'
	`, id, req.Reference)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "update failed")
		return
	}
	if tag.RowsAffected() == 0 {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "approved payout not found")
		return
	}
	respond(w, http.StatusOK, map[string]string{"status": "paid"})
}
