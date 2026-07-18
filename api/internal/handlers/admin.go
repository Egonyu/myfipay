package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

type kycOperator struct {
	TenantID     string  `json:"tenant_id"`
	TenantName   string  `json:"business_name"`
	Slug         string  `json:"slug"`
	TenantStatus string  `json:"status"`
	District     string  `json:"district"`
	UserID       string  `json:"user_id"`
	UserName     string  `json:"name"`
	Email        string  `json:"email"`
	Phone        string  `json:"phone"`
	AppliedAt    time.Time `json:"applied_at"`
	RejectionReason string `json:"rejection_reason,omitempty"`
}

func (h *Handler) ListKYCQueue(w http.ResponseWriter, r *http.Request) {
	filter := r.URL.Query().Get("status")
	if filter == "" {
		filter = "pending_kyc"
	}

	ctx := context.Background()
	rows, err := h.db.Query(ctx, `
		SELECT
			t.id, t.name, t.slug, t.status,
			COALESCE(t.settings->>'district', ''),
			u.id, u.name, u.email, COALESCE(u.phone, ''),
			t.created_at,
			COALESCE(t.settings->>'kyc_rejection_reason', '')
		FROM tenants t
		JOIN users u ON u.tenant_id = t.id AND u.role = 'operator'
		WHERE t.status = $1 AND t.type = 'operator'
		ORDER BY t.created_at DESC
	`, filter)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "could not fetch KYC queue")
		return
	}
	defer rows.Close()

	var operators []kycOperator
	for rows.Next() {
		var op kycOperator
		if err := rows.Scan(
			&op.TenantID, &op.TenantName, &op.Slug, &op.TenantStatus,
			&op.District,
			&op.UserID, &op.UserName, &op.Email, &op.Phone,
			&op.AppliedAt,
			&op.RejectionReason,
		); err == nil {
			operators = append(operators, op)
		}
	}
	if operators == nil {
		operators = []kycOperator{}
	}
	respond(w, http.StatusOK, operators)
}

func (h *Handler) ApproveKYC(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "id")
	ctx := context.Background()

	res, err := h.db.Exec(ctx, `
		UPDATE tenants SET status = 'active', updated_at = NOW()
		WHERE id = $1 AND type = 'operator'
	`, tenantID)
	if err != nil || res.RowsAffected() == 0 {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "tenant not found")
		return
	}

	h.db.Exec(ctx, `UPDATE users SET status = 'active', updated_at = NOW() WHERE tenant_id = $1`, tenantID)

	respond(w, http.StatusOK, map[string]string{"message": "operator approved"})
}

func (h *Handler) RejectKYC(w http.ResponseWriter, r *http.Request) {
	tenantID := chi.URLParam(r, "id")

	var req struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_BODY", "invalid request body")
		return
	}
	if req.Reason == "" {
		req.Reason = "Application did not meet requirements."
	}

	ctx := context.Background()
	res, err := h.db.Exec(ctx, `
		UPDATE tenants
		SET status = 'rejected',
		    settings = jsonb_set(COALESCE(settings, '{}'), '{kyc_rejection_reason}', $1::jsonb),
		    updated_at = NOW()
		WHERE id = $2 AND type = 'operator'
	`, `"`+req.Reason+`"`, tenantID)
	if err != nil || res.RowsAffected() == 0 {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "tenant not found")
		return
	}

	respond(w, http.StatusOK, map[string]string{"message": "operator rejected"})
}

// ─── Tenant List ──────────────────────────────────────────────────────────────

func (h *Handler) ListTenants(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

	rows, err := h.db.Query(ctx, `
		SELECT
			t.id, t.name, t.slug, t.status, t.created_at,
			COUNT(DISTINCT u.id) FILTER (WHERE u.role = 'operator') AS user_count,
			COUNT(DISTINCT l.id) AS location_count,
			COUNT(DISTINCT s.id) AS session_count,
			COALESCE(SUM(p.price_ugx), 0) AS total_revenue,
			COALESCE(MAX(u.email) FILTER (WHERE u.role = 'operator'), '') AS owner_email
		FROM tenants t
		LEFT JOIN users u ON u.tenant_id = t.id
		LEFT JOIN locations l ON l.tenant_id = t.id
		LEFT JOIN sessions s ON s.location_id = l.id
		LEFT JOIN plans p ON s.plan_id = p.id
		WHERE t.type = 'operator'
		GROUP BY t.id, t.name, t.slug, t.status, t.created_at
		ORDER BY t.created_at DESC
	`)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "could not fetch tenants")
		return
	}
	defer rows.Close()

	type tenant struct {
		ID            string    `json:"id"`
		Name          string    `json:"name"`
		Slug          string    `json:"slug"`
		Status        string    `json:"status"`
		CreatedAt     time.Time `json:"created_at"`
		UserCount     int       `json:"user_count"`
		LocationCount int       `json:"location_count"`
		SessionCount  int       `json:"session_count"`
		TotalRevenue  float64   `json:"total_revenue"`
		OwnerEmail    string    `json:"owner_email"`
	}
	var tenants []tenant
	for rows.Next() {
		var t tenant
		if err := rows.Scan(&t.ID, &t.Name, &t.Slug, &t.Status, &t.CreatedAt,
			&t.UserCount, &t.LocationCount, &t.SessionCount, &t.TotalRevenue, &t.OwnerEmail); err == nil {
			tenants = append(tenants, t)
		}
	}
	if tenants == nil {
		tenants = []tenant{}
	}
	respond(w, http.StatusOK, tenants)
}

// ─── Platform Revenue ─────────────────────────────────────────────────────────

func (h *Handler) PlatformRevenue(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

	// Totals
	var totalRevenue, todayRevenue float64
	var totalSessions, todaySessions int
	h.db.QueryRow(ctx,
		`SELECT COALESCE(SUM(p.price_ugx),0), COUNT(s.id)
		 FROM sessions s JOIN plans p ON s.plan_id = p.id`).
		Scan(&totalRevenue, &totalSessions)
	h.db.QueryRow(ctx,
		`SELECT COALESCE(SUM(p.price_ugx),0), COUNT(s.id)
		 FROM sessions s JOIN plans p ON s.plan_id = p.id
		 WHERE s.started_at >= CURRENT_DATE`).
		Scan(&todayRevenue, &todaySessions)

	// Per-tenant breakdown
	tRows, err := h.db.Query(ctx, `
		SELECT t.id, t.name, t.status,
		       COALESCE(SUM(p.price_ugx), 0) AS revenue,
		       COUNT(s.id) AS sessions
		FROM tenants t
		LEFT JOIN locations l ON l.tenant_id = t.id
		LEFT JOIN sessions s ON s.location_id = l.id
		LEFT JOIN plans p ON s.plan_id = p.id
		WHERE t.type = 'operator'
		GROUP BY t.id, t.name, t.status
		ORDER BY revenue DESC
	`)
	type tenantRevRow struct {
		ID       string  `json:"id"`
		Name     string  `json:"name"`
		Status   string  `json:"status"`
		Revenue  float64 `json:"revenue"`
		Sessions int     `json:"sessions"`
	}
	var tenantRevenue []tenantRevRow
	if err == nil {
		defer tRows.Close()
		for tRows.Next() {
			var tr tenantRevRow
			if e := tRows.Scan(&tr.ID, &tr.Name, &tr.Status, &tr.Revenue, &tr.Sessions); e == nil {
				tenantRevenue = append(tenantRevenue, tr)
			}
		}
	}
	if tenantRevenue == nil {
		tenantRevenue = []tenantRevRow{}
	}

	// 30-day chart
	cRows, err := h.db.Query(ctx, `
		SELECT DATE(s.started_at)::text AS day, COALESCE(SUM(p.price_ugx), 0) AS revenue
		FROM sessions s JOIN plans p ON s.plan_id = p.id
		WHERE s.started_at >= NOW() - INTERVAL '30 days'
		GROUP BY day ORDER BY day
	`)
	type chartPoint struct {
		Day     string  `json:"day"`
		Revenue float64 `json:"revenue"`
	}
	var chart []chartPoint
	if err == nil {
		defer cRows.Close()
		for cRows.Next() {
			var cp chartPoint
			if e := cRows.Scan(&cp.Day, &cp.Revenue); e == nil {
				chart = append(chart, cp)
			}
		}
	}
	if chart == nil {
		chart = []chartPoint{}
	}

	respond(w, http.StatusOK, map[string]any{
		"total_revenue":   totalRevenue,
		"today_revenue":   todayRevenue,
		"total_sessions":  totalSessions,
		"today_sessions":  todaySessions,
		"by_tenant":       tenantRevenue,
		"chart":           chart,
	})
}

// ─── Admin: Agent Payout Management ──────────────────────────────────────────

func (h *Handler) ListAllPayoutRequests(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	status := r.URL.Query().Get("status")

	query := `
		SELECT pr.id, t.name AS agent_name, u.email AS agent_email,
		       pr.amount_ugx, pr.method, pr.phone, pr.status,
		       COALESCE(pr.notes, ''), COALESCE(pr.admin_notes, ''),
		       pr.requested_at, pr.processed_at
		FROM payout_requests pr
		JOIN tenants t ON pr.agent_id = t.id
		JOIN users u ON u.tenant_id = t.id AND u.role = 'agent'`
	args := []any{}

	if status != "" {
		query += ` WHERE pr.status = $1`
		args = append(args, status)
	}
	query += ` ORDER BY pr.requested_at DESC LIMIT 200`

	rows, err := h.db.Query(ctx, query, args...)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "query failed")
		return
	}
	defer rows.Close()

	type payoutRow struct {
		ID          string     `json:"id"`
		AgentName   string     `json:"agent_name"`
		AgentEmail  string     `json:"agent_email"`
		AmountUGX   int        `json:"amount_ugx"`
		Method      string     `json:"method"`
		Phone       string     `json:"phone"`
		Status      string     `json:"status"`
		Notes       string     `json:"notes"`
		AdminNotes  string     `json:"admin_notes"`
		RequestedAt time.Time  `json:"requested_at"`
		ProcessedAt *time.Time `json:"processed_at,omitempty"`
	}
	var payouts []payoutRow
	for rows.Next() {
		var p payoutRow
		rows.Scan(&p.ID, &p.AgentName, &p.AgentEmail, &p.AmountUGX, &p.Method,
			&p.Phone, &p.Status, &p.Notes, &p.AdminNotes, &p.RequestedAt, &p.ProcessedAt)
		payouts = append(payouts, p)
	}
	if payouts == nil {
		payouts = []payoutRow{}
	}
	respond(w, http.StatusOK, payouts)
}

func (h *Handler) ApprovePayoutRequest(w http.ResponseWriter, r *http.Request) {
	payoutID := chi.URLParam(r, "id")

	var req struct {
		AdminNotes string `json:"admin_notes"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	ctx := context.Background()
	res, err := h.db.Exec(ctx, `
		UPDATE payout_requests
		SET status = 'approved', admin_notes = $1, processed_at = NOW()
		WHERE id = $2 AND status = 'pending'
	`, req.AdminNotes, payoutID)
	if err != nil || res.RowsAffected() == 0 {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "pending payout request not found")
		return
	}

	// Mark associated commissions as settled when payout is approved
	h.db.Exec(ctx, `
		UPDATE commissions SET status = 'settled'
		WHERE agent_id = (SELECT agent_id FROM payout_requests WHERE id = $1)
		  AND status = 'pending'
	`, payoutID)

	respond(w, http.StatusOK, map[string]string{"message": "payout approved"})
}

func (h *Handler) MarkPayoutPaid(w http.ResponseWriter, r *http.Request) {
	payoutID := chi.URLParam(r, "id")

	var req struct {
		AdminNotes string `json:"admin_notes"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	ctx := context.Background()
	res, err := h.db.Exec(ctx, `
		UPDATE payout_requests
		SET status = 'paid', admin_notes = $1, processed_at = NOW()
		WHERE id = $2 AND status = 'approved'
	`, req.AdminNotes, payoutID)
	if err != nil || res.RowsAffected() == 0 {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "approved payout request not found")
		return
	}
	respond(w, http.StatusOK, map[string]string{"message": "payout marked as paid"})
}

func (h *Handler) RejectPayoutRequest(w http.ResponseWriter, r *http.Request) {
	payoutID := chi.URLParam(r, "id")

	var req struct {
		AdminNotes string `json:"admin_notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.AdminNotes == "" {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "admin_notes (reason) is required")
		return
	}

	ctx := context.Background()
	res, err := h.db.Exec(ctx, `
		UPDATE payout_requests
		SET status = 'rejected', admin_notes = $1, processed_at = NOW()
		WHERE id = $2 AND status = 'pending'
	`, req.AdminNotes, payoutID)
	if err != nil || res.RowsAffected() == 0 {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "pending payout request not found")
		return
	}
	respond(w, http.StatusOK, map[string]string{"message": "payout rejected"})
}

// ─── Admin: Agent List ────────────────────────────────────────────────────────

func (h *Handler) ListAgents(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

	rows, err := h.db.Query(ctx, `
		SELECT t.id, t.name, t.slug, t.status, t.created_at,
		       u.email,
		       COUNT(DISTINCT ar.operator_id) AS operator_count,
		       COALESCE(SUM(c.amount_ugx), 0) AS total_commission
		FROM tenants t
		JOIN users u ON u.tenant_id = t.id AND u.role = 'agent'
		LEFT JOIN agent_referrals ar ON ar.agent_id = t.id
		LEFT JOIN commissions c ON c.agent_id = t.id
		WHERE t.type = 'agent'
		GROUP BY t.id, t.name, t.slug, t.status, t.created_at, u.email
		ORDER BY total_commission DESC
	`)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "query failed")
		return
	}
	defer rows.Close()

	type agentRow struct {
		ID              string    `json:"id"`
		Name            string    `json:"name"`
		Slug            string    `json:"slug"`
		Status          string    `json:"status"`
		CreatedAt       time.Time `json:"created_at"`
		Email           string    `json:"email"`
		OperatorCount   int       `json:"operator_count"`
		TotalCommission float64   `json:"total_commission_ugx"`
	}
	var agents []agentRow
	for rows.Next() {
		var a agentRow
		rows.Scan(&a.ID, &a.Name, &a.Slug, &a.Status, &a.CreatedAt,
			&a.Email, &a.OperatorCount, &a.TotalCommission)
		agents = append(agents, a)
	}
	if agents == nil {
		agents = []agentRow{}
	}
	respond(w, http.StatusOK, agents)
}


