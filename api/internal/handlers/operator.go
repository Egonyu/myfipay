package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/myfibase/myfibase/internal/middleware"
	"golang.org/x/crypto/bcrypt"
)

// ─── Dashboard Stats ──────────────────────────────────────────────────────────

func (h *Handler) DashboardStats(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r)
	ctx := context.Background()

	var activeSessions, todaySessions int
	var todayRevenue float64

	h.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM sessions
		 WHERE status = 'active'
		   AND location_id IN (SELECT id FROM locations WHERE tenant_id = $1)`,
		claims.TenantID,
	).Scan(&activeSessions)

	h.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM sessions
		 WHERE started_at >= CURRENT_DATE
		   AND location_id IN (SELECT id FROM locations WHERE tenant_id = $1)`,
		claims.TenantID,
	).Scan(&todaySessions)

	// Revenue from plan prices for today's sessions
	h.db.QueryRow(ctx,
		`SELECT COALESCE(SUM(p.price_ugx), 0)
		 FROM sessions s
		 JOIN plans p ON s.plan_id = p.id
		 WHERE s.started_at >= CURRENT_DATE
		   AND s.location_id IN (SELECT id FROM locations WHERE tenant_id = $1)`,
		claims.TenantID,
	).Scan(&todayRevenue)

	var totalLocations int
	h.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM locations WHERE tenant_id = $1 AND status = 'active'`,
		claims.TenantID,
	).Scan(&totalLocations)

	var todayBandwidthBytes int64
	h.db.QueryRow(ctx,
		`SELECT COALESCE(SUM(ra.acctinputoctets + ra.acctoutputoctets), 0)
		 FROM radacct ra
		 WHERE ra.username IN (
		     SELECT s.username FROM sessions s
		     WHERE s.started_at >= CURRENT_DATE
		       AND s.location_id IN (SELECT id FROM locations WHERE tenant_id = $1)
		 )`,
		claims.TenantID,
	).Scan(&todayBandwidthBytes)

	respond(w, http.StatusOK, map[string]any{
		"active_sessions":       activeSessions,
		"today_sessions":        todaySessions,
		"today_revenue":         todayRevenue,
		"total_locations":       totalLocations,
		"today_bandwidth_bytes": todayBandwidthBytes,
	})
}

func (h *Handler) RevenueChart(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r)
	ctx := context.Background()

	rows, err := h.db.Query(ctx,
		`SELECT DATE(s.started_at) AS day, COALESCE(SUM(p.price_ugx), 0) AS revenue
		 FROM sessions s
		 JOIN plans p ON s.plan_id = p.id
		 WHERE s.started_at >= NOW() - INTERVAL '30 days'
		   AND s.location_id IN (SELECT id FROM locations WHERE tenant_id = $1)
		 GROUP BY day ORDER BY day`,
		claims.TenantID,
	)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "query failed")
		return
	}
	defer rows.Close()

	type point struct {
		Day     string  `json:"day"`
		Revenue float64 `json:"revenue"`
	}
	var data []point
	for rows.Next() {
		var p point
		rows.Scan(&p.Day, &p.Revenue)
		data = append(data, p)
	}
	if data == nil {
		data = []point{}
	}
	respond(w, http.StatusOK, data)
}

// ─── Sessions ─────────────────────────────────────────────────────────────────

func (h *Handler) ListSessions(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r)
	ctx := context.Background()

	status := r.URL.Query().Get("status") // active, expired, all
	if status == "" {
		status = "all"
	}

	query := `
		SELECT s.id, s.username, s.customer_phone, s.status,
		       s.started_at, s.expires_at, s.terminated_at,
		       l.name AS location_name, p.name AS plan_name, p.price_ugx,
		       COALESCE((
		           SELECT SUM(acctinputoctets + acctoutputoctets)
		           FROM radacct WHERE username = s.username
		           AND acctstarttime >= s.started_at
		       ), 0) AS bytes_used
		FROM sessions s
		JOIN locations l ON s.location_id = l.id
		JOIN plans p ON s.plan_id = p.id
		WHERE l.tenant_id = $1`
	args := []any{claims.TenantID}

	if status != "all" {
		query += ` AND s.status = $2`
		args = append(args, status)
	}
	query += ` ORDER BY s.started_at DESC LIMIT 200`

	rows, err := h.db.Query(ctx, query, args...)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "query failed")
		return
	}
	defer rows.Close()

	type session struct {
		ID           string     `json:"id"`
		Username     string     `json:"username"`
		Phone        string     `json:"customer_phone"`
		Status       string     `json:"status"`
		StartedAt    time.Time  `json:"started_at"`
		ExpiresAt    *time.Time `json:"expires_at"`
		TerminatedAt *time.Time `json:"terminated_at,omitempty"`
		LocationName string     `json:"location_name"`
		PlanName     string     `json:"plan_name"`
		PriceUGX     float64    `json:"price_ugx"`
		BytesUsed    int64      `json:"bytes_used"`
	}
	var sessions []session
	for rows.Next() {
		var s session
		rows.Scan(&s.ID, &s.Username, &s.Phone, &s.Status,
			&s.StartedAt, &s.ExpiresAt, &s.TerminatedAt,
			&s.LocationName, &s.PlanName, &s.PriceUGX, &s.BytesUsed)
		sessions = append(sessions, s)
	}
	if sessions == nil {
		sessions = []session{}
	}
	respond(w, http.StatusOK, sessions)
}

// ListPayments returns confirmed payment records (cash + mobile money) for the
// operator's tenant — the persisted audit trail, not the Redis-only pending state.
func (h *Handler) ListPayments(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r)
	ctx := context.Background()

	method := r.URL.Query().Get("method") // 'cash' | 'mobile_money' | '' (all)

	query := `
		SELECT pm.id, pm.customer_phone, pm.amount_ugx, pm.method, pm.status,
		       COALESCE(pm.confirmed_at, pm.initiated_at) AS paid_at,
		       l.name AS location_name, COALESCE(p.name, '—') AS plan_name
		FROM payments pm
		JOIN locations l ON pm.location_id = l.id
		LEFT JOIN plans p ON pm.plan_id = p.id
		WHERE l.tenant_id = $1 AND pm.status = 'confirmed'`
	args := []any{claims.TenantID}

	if method == "cash" || method == "mobile_money" {
		query += ` AND pm.method = $2`
		args = append(args, method)
	}
	query += ` ORDER BY COALESCE(pm.confirmed_at, pm.initiated_at) DESC LIMIT 300`

	rows, err := h.db.Query(ctx, query, args...)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "query failed")
		return
	}
	defer rows.Close()

	type payment struct {
		ID           string    `json:"id"`
		Phone        string    `json:"customer_phone"`
		AmountUGX    int       `json:"amount_ugx"`
		Method       string    `json:"method"`
		Status       string    `json:"status"`
		PaidAt       time.Time `json:"paid_at"`
		LocationName string    `json:"location_name"`
		PlanName     string    `json:"plan_name"`
	}
	var payments []payment
	for rows.Next() {
		var p payment
		rows.Scan(&p.ID, &p.Phone, &p.AmountUGX, &p.Method, &p.Status,
			&p.PaidAt, &p.LocationName, &p.PlanName)
		payments = append(payments, p)
	}
	if payments == nil {
		payments = []payment{}
	}
	respond(w, http.StatusOK, payments)
}

func (h *Handler) GrantSession(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r)
	ctx := context.Background()

	var req struct {
		Phone    string `json:"phone"`
		PlanID   string `json:"plan_id"`
		MAC      string `json:"mac"`
		Note     string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Phone == "" || req.PlanID == "" {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "phone and plan_id are required")
		return
	}

	// Verify plan belongs to this operator's tenant
	var planName string
	var durationMins, speedDown, speedUp int
	var priceUGX float64
	err := h.db.QueryRow(ctx, `
		SELECT p.name, p.duration_mins, COALESCE(p.speed_down_kbps,2048), COALESCE(p.speed_up_kbps,512), p.price_ugx
		FROM plans p JOIN locations l ON p.location_id = l.id
		WHERE p.id = $1 AND l.tenant_id = $2 AND p.active = TRUE LIMIT 1
	`, req.PlanID, claims.TenantID).Scan(&planName, &durationMins, &speedDown, &speedUp, &priceUGX)
	if err != nil {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "plan not found")
		return
	}

	// Use phone as RADIUS username (normalise: strip leading + or 0, ensure 256 prefix)
	phone := req.Phone
	username := phone
	mac := strings.ToLower(strings.ReplaceAll(req.MAC, "-", ":"))
	duration := time.Duration(durationMins) * time.Minute
	durationSecs := int(duration.Seconds())
	sessionID := uuid.New().String()

	// Redis session
	sessionKey := "session:user:" + username
	h.cache.HSet(ctx, sessionKey,
		"session_id", sessionID,
		"plan_id", req.PlanID,
		"phone", phone,
		"mac", mac,
		"status", "active",
		"started_at", time.Now().UTC().Format(time.RFC3339),
		"grant_type", "manual",
		"note", req.Note,
	)
	h.cache.Expire(ctx, sessionKey, duration)

	if mac != "" {
		h.cache.Set(ctx, "session:mac:"+mac, sessionID, duration)
	}

	// Persist to DB — get first location for this tenant
	h.db.Exec(ctx, `
		INSERT INTO sessions (id, location_id, plan_id, username, customer_phone, mac_address, status, started_at, expires_at)
		VALUES (
			$1,
			(SELECT id FROM locations WHERE tenant_id = $2 LIMIT 1),
			$3, $4, $5, NULLIF($6,''), 'active', NOW(),
			NOW() + ($7 || ' seconds')::interval
		)
		ON CONFLICT DO NOTHING
	`, sessionID, claims.TenantID, req.PlanID, username, phone, mac, fmt.Sprintf("%d", durationSecs))

	// Record the cash payment for audit/reporting — method=cash, immediately confirmed.
	// Operators take cash off-platform; this row gives revenue reports a complete picture.
	h.db.Exec(ctx, `
		INSERT INTO payments (id, location_id, plan_id, customer_phone, amount_ugx, method, status, initiated_at, confirmed_at, metadata)
		VALUES (
			$1,
			(SELECT id FROM locations WHERE tenant_id = $2 LIMIT 1),
			$3, $4, $5, 'cash', 'confirmed', NOW(), NOW(),
			jsonb_build_object('granted_by', $6::text, 'session_id', $7::text, 'note', $8::text)
		)
	`, uuid.New().String(), claims.TenantID, req.PlanID, phone, int(priceUGX), claims.UserID, sessionID, req.Note)

	// RADIUS entries
	h.db.Exec(ctx, `DELETE FROM radcheck WHERE username = $1`, username)
	h.db.Exec(ctx, `INSERT INTO radcheck (username, attribute, op, value) VALUES ($1,'Auth-Type',':=','Accept')`, username)
	h.db.Exec(ctx, `DELETE FROM radreply WHERE username = $1`, username)
	rateLimit := fmt.Sprintf("%dk/%dk", speedDown, speedUp)
	h.db.Exec(ctx, `INSERT INTO radreply (username,attribute,op,value) VALUES ($1,'Session-Timeout',':=',$2)`, username, fmt.Sprintf("%d", durationSecs))
	h.db.Exec(ctx, `INSERT INTO radreply (username,attribute,op,value) VALUES ($1,'Idle-Timeout',':=','300')`, username)
	h.db.Exec(ctx, `INSERT INTO radreply (username,attribute,op,value) VALUES ($1,'Mikrotik-Rate-Limit',':=',$2)`, username, rateLimit)
	h.db.Exec(ctx, `INSERT INTO radreply (username,attribute,op,value) VALUES ($1,'WISPr-Bandwidth-Max-Down',':=',$2)`, username, fmt.Sprintf("%d", speedDown*1000))
	h.db.Exec(ctx, `INSERT INTO radreply (username,attribute,op,value) VALUES ($1,'WISPr-Bandwidth-Max-Up',':=',$2)`, username, fmt.Sprintf("%d", speedUp*1000))

	respond(w, http.StatusCreated, map[string]any{
		"session_id":    sessionID,
		"username":      username,
		"plan_name":     planName,
		"duration_mins": durationMins,
		"expires_at":    time.Now().Add(duration).UTC(),
	})
}

func (h *Handler) ExtendSession(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r)
	sessionID := chi.URLParam(r, "id")
	ctx := context.Background()

	var req struct {
		PlanID string `json:"plan_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.PlanID == "" {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "plan_id is required")
		return
	}

	// Verify session belongs to this tenant and is active
	var username string
	var currentExpiry time.Time
	err := h.db.QueryRow(ctx,
		`SELECT s.username, s.expires_at FROM sessions s
		 JOIN locations l ON s.location_id = l.id
		 WHERE s.id = $1 AND l.tenant_id = $2 AND s.status = 'active'`,
		sessionID, claims.TenantID,
	).Scan(&username, &currentExpiry)
	if err != nil {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "active session not found")
		return
	}

	// Verify plan belongs to this tenant (tenant-scoped query)
	var pl planRecord
	pl.ID = req.PlanID
	err = h.db.QueryRow(ctx, `
		SELECT p.name, p.duration_mins, COALESCE(p.speed_down_kbps,2048), COALESCE(p.speed_up_kbps,512)
		FROM plans p JOIN locations l ON p.location_id = l.id
		WHERE p.id = $1 AND l.tenant_id = $2 AND p.active = TRUE LIMIT 1
	`, req.PlanID, claims.TenantID).Scan(&pl.Name, &pl.DurationMins, &pl.SpeedDownKbps, &pl.SpeedUpKbps)
	if err != nil {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "plan not found")
		return
	}

	addDuration := time.Duration(pl.DurationMins) * time.Minute

	// Extend from current expiry (or now if already past)
	base := currentExpiry
	if base.Before(time.Now()) {
		base = time.Now()
	}
	newExpiry := base.Add(addDuration)
	newDurationSecs := int(time.Until(newExpiry).Seconds())
	if newDurationSecs < 60 {
		newDurationSecs = 60
	}

	// Update DB
	h.db.Exec(ctx,
		`UPDATE sessions SET expires_at = $1 WHERE id = $2`,
		newExpiry, sessionID,
	)

	// Update radreply Session-Timeout
	h.db.Exec(ctx,
		`UPDATE radreply SET value = $1 WHERE username = $2 AND attribute = 'Session-Timeout'`,
		fmt.Sprintf("%d", newDurationSecs), username,
	)

	// Extend Redis TTL
	sessionKey := "session:user:" + username
	h.cache.Expire(ctx, sessionKey, time.Until(newExpiry))
	mac, _ := h.cache.HGet(ctx, sessionKey, "mac").Result()
	if mac != "" {
		h.cache.Expire(ctx, "session:mac:"+mac, time.Until(newExpiry))
	}

	respond(w, http.StatusOK, map[string]any{
		"session_id":    sessionID,
		"username":      username,
		"added_mins":    pl.DurationMins,
		"new_expires_at": newExpiry.UTC(),
	})
}

func (h *Handler) TerminateSession(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r)
	sessionID := chi.URLParam(r, "id")
	ctx := context.Background()

	var username string
	err := h.db.QueryRow(ctx,
		`SELECT s.username FROM sessions s
		 JOIN locations l ON s.location_id = l.id
		 WHERE s.id = $1 AND l.tenant_id = $2 AND s.status = 'active'`,
		sessionID, claims.TenantID,
	).Scan(&username)
	if err != nil {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "active session not found")
		return
	}

	h.expireSession(username, sessionID)
	respond(w, http.StatusOK, map[string]string{"message": "session terminated"})
}

// ─── Locations ────────────────────────────────────────────────────────────────

func (h *Handler) ListOperatorLocations(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r)
	ctx := context.Background()

	rows, err := h.db.Query(ctx,
		`SELECT id, name, COALESCE(address,''), portal_slug, status, created_at
		 FROM locations WHERE tenant_id = $1 ORDER BY created_at DESC`,
		claims.TenantID,
	)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "query failed")
		return
	}
	defer rows.Close()

	type loc struct {
		ID         string    `json:"id"`
		Name       string    `json:"name"`
		Address    string    `json:"address"`
		PortalSlug string    `json:"portal_slug"`
		Status     string    `json:"status"`
		CreatedAt  time.Time `json:"created_at"`
	}
	var locations []loc
	for rows.Next() {
		var l loc
		rows.Scan(&l.ID, &l.Name, &l.Address, &l.PortalSlug, &l.Status, &l.CreatedAt)
		locations = append(locations, l)
	}
	if locations == nil {
		locations = []loc{}
	}
	respond(w, http.StatusOK, locations)
}

func (h *Handler) CreateLocation(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r)

	var req struct {
		Name    string `json:"name"`
		Address string `json:"address"`
		Slug    string `json:"portal_slug"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" || req.Slug == "" {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "name and portal_slug are required")
		return
	}

	ctx := context.Background()
	id := uuid.New().String()
	_, err := h.db.Exec(ctx,
		`INSERT INTO locations (id, tenant_id, name, address, portal_slug, status)
		 VALUES ($1, $2, $3, $4, $5, 'active')`,
		id, claims.TenantID, req.Name, req.Address, req.Slug,
	)
	if err != nil {
		respondError(w, http.StatusConflict, "CONFLICT", "portal_slug may already be taken")
		return
	}
	respond(w, http.StatusCreated, map[string]string{
		"id":         id,
		"portal_url": "http://170.64.177.20/portal/" + req.Slug + "/",
	})
}

func (h *Handler) UpdateLocationBranding(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r)
	locationID := chi.URLParam(r, "id")
	ctx := context.Background()

	var req struct {
		PortalName   string `json:"portal_name"`
		Tagline      string `json:"tagline"`
		PrimaryColor string `json:"primary_color"`
		LogoURL      string `json:"logo_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_BODY", "invalid request body")
		return
	}

	// Validate color is a hex code
	if req.PrimaryColor != "" && (len(req.PrimaryColor) != 7 || req.PrimaryColor[0] != '#') {
		respondError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "primary_color must be a hex color like #2563eb")
		return
	}

	res, err := h.db.Exec(ctx, `
		UPDATE locations
		SET branding = jsonb_build_object(
			'portal_name',   $1::text,
			'tagline',       $2::text,
			'primary_color', $3::text,
			'logo_url',      $4::text
		)
		WHERE id = $5 AND tenant_id = $6
	`, req.PortalName, req.Tagline, req.PrimaryColor, req.LogoURL, locationID, claims.TenantID)
	if err != nil || res.RowsAffected() == 0 {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "location not found")
		return
	}

	respond(w, http.StatusOK, map[string]string{"message": "branding updated"})
}

func (h *Handler) GetLocationBranding(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r)
	locationID := chi.URLParam(r, "id")
	ctx := context.Background()

	var portalName, tagline, primaryColor, logoURL string
	err := h.db.QueryRow(ctx, `
		SELECT COALESCE(branding->>'portal_name',''),
		       COALESCE(branding->>'tagline',''),
		       COALESCE(branding->>'primary_color','#2563eb'),
		       COALESCE(branding->>'logo_url','')
		FROM locations WHERE id = $1 AND tenant_id = $2 LIMIT 1
	`, locationID, claims.TenantID).Scan(&portalName, &tagline, &primaryColor, &logoURL)
	if err != nil {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "location not found")
		return
	}

	respond(w, http.StatusOK, map[string]any{
		"portal_name":   portalName,
		"tagline":       tagline,
		"primary_color": primaryColor,
		"logo_url":      logoURL,
	})
}

// ─── Plans ────────────────────────────────────────────────────────────────────

func (h *Handler) ListOperatorPlans(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r)
	ctx := context.Background()

	rows, err := h.db.Query(ctx,
		`SELECT p.id, p.location_id, l.name AS location_name,
		        p.name, p.price_ugx, p.duration_mins,
		        COALESCE(p.speed_down_kbps, 2048), COALESCE(p.speed_up_kbps, 512),
		        CASE WHEN p.active THEN 'active' ELSE 'inactive' END
		 FROM plans p
		 JOIN locations l ON p.location_id = l.id
		 WHERE l.tenant_id = $1
		 ORDER BY l.name, p.price_ugx`,
		claims.TenantID,
	)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "query failed")
		return
	}
	defer rows.Close()

	type plan struct {
		ID             string  `json:"id"`
		LocationID     string  `json:"location_id"`
		LocationName   string  `json:"location_name"`
		Name           string  `json:"name"`
		PriceUGX       float64 `json:"price_ugx"`
		DurationMins   int     `json:"duration_minutes"`
		SpeedDownKbps  int     `json:"speed_down_kbps"`
		SpeedUpKbps    int     `json:"speed_up_kbps"`
		Status         string  `json:"status"`
	}
	var plans []plan
	for rows.Next() {
		var p plan
		rows.Scan(&p.ID, &p.LocationID, &p.LocationName, &p.Name, &p.PriceUGX,
			&p.DurationMins, &p.SpeedDownKbps, &p.SpeedUpKbps, &p.Status)
		plans = append(plans, p)
	}
	if plans == nil {
		plans = []plan{}
	}
	respond(w, http.StatusOK, plans)
}

func (h *Handler) CreatePlan(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r)

	var req struct {
		LocationID    string  `json:"location_id"`
		Name          string  `json:"name"`
		PriceUGX      float64 `json:"price_ugx"`
		DurationMins  int     `json:"duration_minutes"`
		SpeedDownKbps int     `json:"speed_down_kbps"`
		SpeedUpKbps   int     `json:"speed_up_kbps"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_BODY", "invalid request body")
		return
	}
	if req.Name == "" || req.PriceUGX <= 0 || req.DurationMins <= 0 {
		respondError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "name, price_ugx, duration_minutes are required")
		return
	}

	ctx := context.Background()
	// Verify location belongs to this tenant
	var count int
	h.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM locations WHERE id = $1 AND tenant_id = $2`,
		req.LocationID, claims.TenantID,
	).Scan(&count)
	if count == 0 {
		respondError(w, http.StatusForbidden, "FORBIDDEN", "location not found")
		return
	}

	id := uuid.New().String()
	h.db.Exec(ctx,
		`INSERT INTO plans (id, location_id, name, price_ugx, duration_mins, speed_down_kbps, speed_up_kbps, active)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, TRUE)`,
		id, req.LocationID, req.Name, req.PriceUGX, req.DurationMins, req.SpeedDownKbps, req.SpeedUpKbps,
	)
	respond(w, http.StatusCreated, map[string]string{"id": id})
}

func (h *Handler) UpdatePlan(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r)
	planID := chi.URLParam(r, "id")

	var req struct {
		Name          string  `json:"name"`
		PriceUGX      float64 `json:"price_ugx"`
		DurationMins  int     `json:"duration_minutes"`
		SpeedDownKbps int     `json:"speed_down_kbps"`
		SpeedUpKbps   int     `json:"speed_up_kbps"`
		Status        string  `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_BODY", "invalid request body")
		return
	}

	ctx := context.Background()
	isActive := req.Status != "inactive"
	res, err := h.db.Exec(ctx,
		`UPDATE plans SET name=$1, price_ugx=$2, duration_mins=$3,
		        speed_down_kbps=$4, speed_up_kbps=$5, active=$6, updated_at=NOW()
		 WHERE id=$7
		   AND location_id IN (SELECT id FROM locations WHERE tenant_id=$8)`,
		req.Name, req.PriceUGX, req.DurationMins,
		req.SpeedDownKbps, req.SpeedUpKbps, isActive,
		planID, claims.TenantID,
	)
	if err != nil || res.RowsAffected() == 0 {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "plan not found")
		return
	}
	respond(w, http.StatusOK, map[string]string{"message": "plan updated"})
}

func (h *Handler) DeletePlan(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r)
	planID := chi.URLParam(r, "id")
	ctx := context.Background()

	res, err := h.db.Exec(ctx,
		`UPDATE plans SET active=FALSE, updated_at=NOW()
		 WHERE id=$1
		   AND location_id IN (SELECT id FROM locations WHERE tenant_id=$2)`,
		planID, claims.TenantID,
	)
	if err != nil || res.RowsAffected() == 0 {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "plan not found")
		return
	}
	respond(w, http.StatusOK, map[string]string{"message": "plan deactivated"})
}

// ─── Vouchers ─────────────────────────────────────────────────────────────────

func (h *Handler) CreateVoucherBatch(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r)
	ctx := context.Background()

	var req struct {
		LocationID string `json:"location_id"`
		PlanID     string `json:"plan_id"`
		Quantity   int    `json:"quantity"`
		Note       string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_BODY", "invalid request body")
		return
	}
	if req.LocationID == "" || req.PlanID == "" || req.Quantity < 1 || req.Quantity > 500 {
		respondError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "location_id, plan_id, quantity (1-500) are required")
		return
	}

	// Verify location + plan belong to this tenant
	var locationOK bool
	h.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM locations WHERE id=$1 AND tenant_id=$2)`, req.LocationID, claims.TenantID).Scan(&locationOK)
	if !locationOK {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "location not found")
		return
	}
	var planOK bool
	h.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM plans WHERE id=$1 AND location_id=$2 AND active=TRUE)`, req.PlanID, req.LocationID).Scan(&planOK)
	if !planOK {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "plan not found")
		return
	}

	batchID := uuid.New().String()
	_, err := h.db.Exec(ctx,
		`INSERT INTO voucher_batches (id, location_id, plan_id, quantity, note) VALUES ($1,$2,$3,$4,$5)`,
		batchID, req.LocationID, req.PlanID, req.Quantity, req.Note,
	)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "could not create batch")
		return
	}

	// Generate voucher codes: prefix MFB- + 8 random alphanumeric chars
	const charset = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no 0/O/1/I to avoid confusion
	codes := make([]string, req.Quantity)
	for i := range codes {
		b := make([]byte, 8)
		for j := range b {
			b[j] = charset[uuid.New()[j%16]%byte(len(charset))]
		}
		codes[i] = "MFB-" + string(b)
	}

	// Insert vouchers individually (quantity capped at 500)
	for _, code := range codes {
		h.db.Exec(ctx,
			`INSERT INTO vouchers (batch_id, location_id, plan_id, code, status) VALUES ($1,$2,$3,$4,'unused')`,
			batchID, req.LocationID, req.PlanID, code,
		)
	}

	respond(w, http.StatusCreated, map[string]any{
		"batch_id": batchID,
		"quantity": req.Quantity,
		"codes":    codes,
	})
}

func (h *Handler) ListVoucherBatches(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r)
	ctx := context.Background()

	rows, err := h.db.Query(ctx, `
		SELECT vb.id, l.name, p.name, p.price_ugx, vb.quantity,
		       COUNT(CASE WHEN v.status='used' THEN 1 END),
		       vb.note, vb.created_at
		FROM voucher_batches vb
		JOIN locations l ON vb.location_id = l.id
		JOIN plans p ON vb.plan_id = p.id
		LEFT JOIN vouchers v ON v.batch_id = vb.id
		WHERE l.tenant_id = $1
		GROUP BY vb.id, l.name, p.name, p.price_ugx, vb.quantity, vb.note, vb.created_at
		ORDER BY vb.created_at DESC
	`, claims.TenantID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "query failed")
		return
	}
	defer rows.Close()

	type batch struct {
		ID           string    `json:"id"`
		LocationName string    `json:"location_name"`
		PlanName     string    `json:"plan_name"`
		PriceUGX     int       `json:"price_ugx"`
		Quantity     int       `json:"quantity"`
		UsedCount    int       `json:"used_count"`
		Note         string    `json:"note"`
		CreatedAt    time.Time `json:"created_at"`
	}
	var batches []batch
	for rows.Next() {
		var b batch
		rows.Scan(&b.ID, &b.LocationName, &b.PlanName, &b.PriceUGX, &b.Quantity, &b.UsedCount, &b.Note, &b.CreatedAt)
		batches = append(batches, b)
	}
	if batches == nil {
		batches = []batch{}
	}
	respond(w, http.StatusOK, batches)
}

func (h *Handler) GetVoucherBatch(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r)
	batchID := chi.URLParam(r, "id")
	ctx := context.Background()

	rows, err := h.db.Query(ctx, `
		SELECT v.id, v.code, v.status, COALESCE(v.used_by_phone,''), v.expires_at, v.activated_at, v.created_at
		FROM vouchers v
		JOIN voucher_batches vb ON v.batch_id = vb.id
		JOIN locations l ON vb.location_id = l.id
		WHERE v.batch_id = $1 AND l.tenant_id = $2
		ORDER BY v.created_at ASC
	`, batchID, claims.TenantID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "query failed")
		return
	}
	defer rows.Close()

	type voucher struct {
		ID          string     `json:"id"`
		Code        string     `json:"code"`
		Status      string     `json:"status"`
		UsedByPhone string     `json:"used_by_phone"`
		ExpiresAt   *time.Time `json:"expires_at"`
		ActivatedAt *time.Time `json:"activated_at"`
		CreatedAt   time.Time  `json:"created_at"`
	}
	var vouchers []voucher
	for rows.Next() {
		var v voucher
		rows.Scan(&v.ID, &v.Code, &v.Status, &v.UsedByPhone, &v.ExpiresAt, &v.ActivatedAt, &v.CreatedAt)
		vouchers = append(vouchers, v)
	}
	if vouchers == nil {
		vouchers = []voucher{}
	}
	respond(w, http.StatusOK, vouchers)
}

// ─── Operator Profile ─────────────────────────────────────────────────────────

func (h *Handler) GetProfile(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r)
	ctx := context.Background()

	var u struct {
		ID    string
		Email string
		Name  string
		Phone *string
		Role  string
	}
	h.db.QueryRow(ctx,
		`SELECT id, email, name, phone, role FROM users WHERE id = $1`,
		claims.UserID,
	).Scan(&u.ID, &u.Email, &u.Name, &u.Phone, &u.Role)

	respond(w, http.StatusOK, map[string]any{
		"id":    u.ID,
		"email": u.Email,
		"name":  u.Name,
		"phone": u.Phone,
		"role":  u.Role,
	})
}

func (h *Handler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r)

	var req struct {
		Name  string `json:"name"`
		Phone string `json:"phone"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "name is required")
		return
	}

	ctx := context.Background()
	h.db.Exec(ctx,
		`UPDATE users SET name=$1, phone=$2, updated_at=NOW() WHERE id=$3`,
		req.Name, req.Phone, claims.UserID,
	)
	respond(w, http.StatusOK, map[string]string{"message": "profile updated"})
}

// ─── Admin: Create Operator ───────────────────────────────────────────────────

func (h *Handler) CreateOperator(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TenantID string `json:"tenant_id"`
		Email    string `json:"email"`
		Name     string `json:"name"`
		Password string `json:"password"`
		Phone    string `json:"phone"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_BODY", "invalid request body")
		return
	}
	if req.Email == "" || req.Password == "" || req.Name == "" {
		respondError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "email, name, password required")
		return
	}
	if len(req.Password) < 8 {
		respondError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "password must be at least 8 characters")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "HASH_ERROR", "could not hash password")
		return
	}

	ctx := context.Background()
	// Auto-create tenant if not provided
	tenantID := req.TenantID
	if tenantID == "" {
		tenantID = uuid.New().String()
		slug := strings.ToLower(strings.ReplaceAll(req.Name, " ", "-")) + "-" + tenantID[:8]
		h.db.Exec(ctx,
			`INSERT INTO tenants (id, name, slug, status) VALUES ($1, $2, $3, 'active') ON CONFLICT DO NOTHING`,
			tenantID, req.Name+" (Operator)", slug,
		)
	}

	id := uuid.New().String()
	_, err = h.db.Exec(ctx,
		`INSERT INTO users (id, tenant_id, email, name, phone, role, password, status)
		 VALUES ($1, $2, $3, $4, $5, 'operator', $6, 'active')`,
		id, tenantID, req.Email, req.Name, req.Phone, string(hash),
	)
	if err != nil {
		respondError(w, http.StatusConflict, "CONFLICT", "email already registered")
		return
	}
	respond(w, http.StatusCreated, map[string]string{"id": id, "tenant_id": tenantID})
}
