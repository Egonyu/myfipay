package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/myfibase/myfibase/internal/middleware"
	"golang.org/x/crypto/bcrypt"

	"github.com/google/uuid"
)

// agentCommissionRate is the referring agent's cut of every confirmed payment from
// an operator they recruited — 3% lifetime, per BUSINESS_MODEL.md §4. Paid out of the
// platform's own commission, not charged on top of the operator.
const agentCommissionRate = 0.03

// ─── Agent Registration ───────────────────────────────────────────────────────

func (h *Handler) RegisterAgent(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		Phone    string `json:"phone"`
		Email    string `json:"email"`
		District string `json:"district"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_BODY", "invalid request body")
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.Name = strings.TrimSpace(req.Name)

	if req.Name == "" || req.Email == "" || req.Password == "" || req.Phone == "" {
		respondError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "name, email, phone, and password are required")
		return
	}
	if len(req.Password) < 8 {
		respondError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "password must be at least 8 characters")
		return
	}

	slug := "agent-" + slugify(req.Name)
	ctx := context.Background()

	var exists int
	h.db.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE email = $1`, req.Email).Scan(&exists)
	if exists > 0 {
		respondError(w, http.StatusConflict, "EMAIL_TAKEN", "an account with this email already exists")
		return
	}

	var slugCount int
	h.db.QueryRow(ctx, `SELECT COUNT(*) FROM tenants WHERE slug LIKE $1`, slug+"%").Scan(&slugCount)
	if slugCount > 0 {
		slug = fmt.Sprintf("%s-%d", slug, slugCount+1)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "HASH_ERROR", "could not process password")
		return
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "could not start transaction")
		return
	}
	defer tx.Rollback(ctx)

	var tenantID string
	err = tx.QueryRow(ctx, `
		INSERT INTO tenants (name, slug, type, status, settings)
		VALUES ($1, $2, 'agent', 'active', $3)
		RETURNING id
	`, req.Name+" (Agent)", slug, fmt.Sprintf(`{"district":"%s"}`, req.District)).Scan(&tenantID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "could not create agent tenant")
		return
	}

	var userID string
	err = tx.QueryRow(ctx, `
		INSERT INTO users (tenant_id, email, phone, name, role, password, status)
		VALUES ($1, $2, $3, $4, 'agent', $5, 'active')
		RETURNING id
	`, tenantID, req.Email, req.Phone, req.Name, string(hash)).Scan(&userID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "could not create agent user")
		return
	}

	if err := tx.Commit(ctx); err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "could not complete registration")
		return
	}

	respond(w, http.StatusCreated, map[string]string{
		"id":          userID,
		"tenant_id":   tenantID,
		"invite_code": slug,
		"status":      "active",
	})
}

// ─── Agent Dashboard ──────────────────────────────────────────────────────────

func (h *Handler) AgentDashboard(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r)
	ctx := context.Background()

	var operatorCount int
	h.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM agent_referrals WHERE agent_id = $1`,
		claims.TenantID,
	).Scan(&operatorCount)

	var totalEarned float64
	h.db.QueryRow(ctx,
		`SELECT COALESCE(SUM(amount_ugx), 0) FROM commissions WHERE agent_id = $1`,
		claims.TenantID,
	).Scan(&totalEarned)

	var pendingCommission float64
	h.db.QueryRow(ctx,
		`SELECT COALESCE(SUM(amount_ugx), 0) FROM commissions WHERE agent_id = $1 AND status = 'pending'`,
		claims.TenantID,
	).Scan(&pendingCommission)

	// Paid out = sum of paid + approved payout requests (reserved against balance)
	var reservedForPayouts float64
	h.db.QueryRow(ctx,
		`SELECT COALESCE(SUM(amount_ugx), 0) FROM payout_requests
		 WHERE agent_id = $1 AND status IN ('pending', 'approved', 'paid')`,
		claims.TenantID,
	).Scan(&reservedForPayouts)

	availableBalance := totalEarned - reservedForPayouts

	var pendingPayouts int
	h.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM payout_requests WHERE agent_id = $1 AND status = 'pending'`,
		claims.TenantID,
	).Scan(&pendingPayouts)

	respond(w, http.StatusOK, map[string]any{
		"operator_count":      operatorCount,
		"total_earned_ugx":    totalEarned,
		"pending_commission":  pendingCommission,
		"available_balance":   availableBalance,
		"pending_payouts":     pendingPayouts,
	})
}

// ─── Invite Link ──────────────────────────────────────────────────────────────

func (h *Handler) AgentInviteLink(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r)
	ctx := context.Background()

	var slug string
	h.db.QueryRow(ctx, `SELECT slug FROM tenants WHERE id = $1`, claims.TenantID).Scan(&slug)

	respond(w, http.StatusOK, map[string]string{
		"invite_code": slug,
		"invite_url":  "https://myfipay.com/signup?agent=" + slug,
	})
}

// ─── Operators ────────────────────────────────────────────────────────────────

func (h *Handler) AgentOperators(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r)
	ctx := context.Background()

	rows, err := h.db.Query(ctx, `
		SELECT t.id, t.name, t.status, ar.created_at,
		       COALESCE(SUM(c.amount_ugx), 0) AS total_commission
		FROM agent_referrals ar
		JOIN tenants t ON ar.operator_id = t.id
		LEFT JOIN commissions c ON c.operator_id = t.id AND c.agent_id = $1
		WHERE ar.agent_id = $1
		GROUP BY t.id, t.name, t.status, ar.created_at
		ORDER BY ar.created_at DESC
	`, claims.TenantID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "query failed")
		return
	}
	defer rows.Close()

	type operatorRow struct {
		ID              string    `json:"id"`
		Name            string    `json:"name"`
		Status          string    `json:"status"`
		JoinedAt        time.Time `json:"joined_at"`
		TotalCommission float64   `json:"total_commission_ugx"`
	}
	var operators []operatorRow
	for rows.Next() {
		var o operatorRow
		rows.Scan(&o.ID, &o.Name, &o.Status, &o.JoinedAt, &o.TotalCommission)
		operators = append(operators, o)
	}
	if operators == nil {
		operators = []operatorRow{}
	}
	respond(w, http.StatusOK, operators)
}

// ─── Commission History ───────────────────────────────────────────────────────

func (h *Handler) AgentCommissions(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r)
	ctx := context.Background()

	rows, err := h.db.Query(ctx, `
		SELECT c.id, t.name AS operator_name, c.amount_ugx, c.rate_pct, c.status, c.created_at
		FROM commissions c
		JOIN tenants t ON c.operator_id = t.id
		WHERE c.agent_id = $1
		ORDER BY c.created_at DESC
		LIMIT 200
	`, claims.TenantID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "query failed")
		return
	}
	defer rows.Close()

	type commissionRow struct {
		ID           string    `json:"id"`
		OperatorName string    `json:"operator_name"`
		AmountUGX    int       `json:"amount_ugx"`
		RatePct      float64   `json:"rate_pct"`
		Status       string    `json:"status"`
		CreatedAt    time.Time `json:"created_at"`
	}
	var commissions []commissionRow
	for rows.Next() {
		var c commissionRow
		rows.Scan(&c.ID, &c.OperatorName, &c.AmountUGX, &c.RatePct, &c.Status, &c.CreatedAt)
		commissions = append(commissions, c)
	}
	if commissions == nil {
		commissions = []commissionRow{}
	}
	respond(w, http.StatusOK, commissions)
}

// ─── Payouts ──────────────────────────────────────────────────────────────────

func (h *Handler) RequestPayout(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r)

	var req struct {
		AmountUGX int    `json:"amount_ugx"`
		Method    string `json:"method"`
		Phone     string `json:"phone"`
		Notes     string `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_BODY", "invalid request body")
		return
	}
	if req.AmountUGX < minPayoutUGX {
		respondError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR",
			fmt.Sprintf("minimum payout is %d UGX", minPayoutUGX))
		return
	}
	if req.Phone == "" {
		respondError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "phone is required")
		return
	}
	if req.Method == "" {
		req.Method = "mtn_momo"
	}

	ctx := context.Background()

	var totalEarned float64
	h.db.QueryRow(ctx,
		`SELECT COALESCE(SUM(amount_ugx), 0) FROM commissions WHERE agent_id = $1`,
		claims.TenantID,
	).Scan(&totalEarned)

	var reserved float64
	h.db.QueryRow(ctx,
		`SELECT COALESCE(SUM(amount_ugx), 0) FROM payout_requests
		 WHERE agent_id = $1 AND status IN ('pending', 'approved', 'paid')`,
		claims.TenantID,
	).Scan(&reserved)

	available := int(totalEarned) - int(reserved)
	if req.AmountUGX > available {
		respondError(w, http.StatusUnprocessableEntity, "INSUFFICIENT_BALANCE",
			fmt.Sprintf("available balance is %d UGX", available))
		return
	}

	id := uuid.New().String()
	h.db.Exec(ctx, `
		INSERT INTO payout_requests (id, agent_id, amount_ugx, method, phone, notes)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, id, claims.TenantID, req.AmountUGX, req.Method, req.Phone, req.Notes)

	respond(w, http.StatusCreated, map[string]string{
		"id":      id,
		"status":  "pending",
		"message": "Payout request submitted. You will be paid within 24 hours.",
	})
}

func (h *Handler) ListPayoutRequests(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r)
	ctx := context.Background()

	rows, err := h.db.Query(ctx, `
		SELECT id, amount_ugx, method, phone, status,
		       COALESCE(notes, ''), requested_at, processed_at
		FROM payout_requests
		WHERE agent_id = $1
		ORDER BY requested_at DESC
	`, claims.TenantID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "query failed")
		return
	}
	defer rows.Close()

	type payoutRow struct {
		ID          string     `json:"id"`
		AmountUGX   int        `json:"amount_ugx"`
		Method      string     `json:"method"`
		Phone       string     `json:"phone"`
		Status      string     `json:"status"`
		Notes       string     `json:"notes"`
		RequestedAt time.Time  `json:"requested_at"`
		ProcessedAt *time.Time `json:"processed_at,omitempty"`
	}
	var payouts []payoutRow
	for rows.Next() {
		var p payoutRow
		rows.Scan(&p.ID, &p.AmountUGX, &p.Method, &p.Phone, &p.Status,
			&p.Notes, &p.RequestedAt, &p.ProcessedAt)
		payouts = append(payouts, p)
	}
	if payouts == nil {
		payouts = []payoutRow{}
	}
	respond(w, http.StatusOK, payouts)
}
