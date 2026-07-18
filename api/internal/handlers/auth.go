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
)

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type userRow struct {
	ID           string
	TenantID     string
	Email        string
	Name         string
	Role         string
	Status       string
	PasswordHash string
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_BODY", "invalid request body")
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if req.Email == "" || req.Password == "" {
		respondError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "email and password are required")
		return
	}

	ctx := context.Background()
	var u userRow
	err := h.db.QueryRow(ctx,
		`SELECT id, tenant_id, email, name, role, status, password
		 FROM users WHERE email = $1 LIMIT 1`,
		req.Email,
	).Scan(&u.ID, &u.TenantID, &u.Email, &u.Name, &u.Role, &u.Status, &u.PasswordHash)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "invalid email or password")
		return
	}

	switch u.Status {
	case "active":
		// continue
	case "pending_kyc":
		respondError(w, http.StatusForbidden, "PENDING_KYC", "Your account is pending review. You will be notified once approved.")
		return
	case "rejected":
		respondError(w, http.StatusForbidden, "ACCOUNT_REJECTED", "Your application was not approved. Contact support for details.")
		return
	default:
		respondError(w, http.StatusForbidden, "ACCOUNT_INACTIVE", "account is not active")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Password)); err != nil {
		respondError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "invalid email or password")
		return
	}

	claims := &middleware.UserClaims{
		UserID:   u.ID,
		TenantID: u.TenantID,
		Email:    u.Email,
		Role:     u.Role,
		Name:     u.Name,
	}
	token, err := middleware.GenerateToken(claims, h.cfg.JWTSecret, h.cfg.JWTExpiryHours)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "TOKEN_ERROR", "could not generate token")
		return
	}

	// Update last_login
	h.db.Exec(ctx, `UPDATE users SET last_login = NOW() WHERE id = $1`, u.ID)

	// Set httpOnly cookie for dashboard
	http.SetCookie(w, &http.Cookie{
		Name:     "mfb_token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   h.cfg.JWTExpiryHours * 3600,
	})

	respond(w, http.StatusOK, map[string]any{
		"token":      token,
		"expires_in": h.cfg.JWTExpiryHours * 3600,
		"user": map[string]string{
			"id":        u.ID,
			"tenant_id": u.TenantID,
			"email":     u.Email,
			"name":      u.Name,
			"role":      u.Role,
		},
	})
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r)

	ctx := context.Background()
	var u struct {
		ID        string
		TenantID  string
		Email     string
		Name      string
		Role      string
		Status    string
		LastLogin *time.Time
		CreatedAt time.Time
	}
	err := h.db.QueryRow(ctx,
		`SELECT id, tenant_id, email, name, role, status, last_login, created_at
		 FROM users WHERE id = $1`,
		claims.UserID,
	).Scan(&u.ID, &u.TenantID, &u.Email, &u.Name, &u.Role, &u.Status, &u.LastLogin, &u.CreatedAt)
	if err != nil {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "user not found")
		return
	}

	respond(w, http.StatusOK, map[string]any{
		"id":         u.ID,
		"tenant_id":  u.TenantID,
		"email":      u.Email,
		"name":       u.Name,
		"role":       u.Role,
		"status":     u.Status,
		"last_login": u.LastLogin,
		"created_at": u.CreatedAt,
	})
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	// Clear the cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "mfb_token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
	respond(w, http.StatusOK, map[string]string{"message": "logged out"})
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name         string `json:"name"`
		BusinessName string `json:"business_name"`
		Email        string `json:"email"`
		Phone        string `json:"phone"`
		District     string `json:"district"`
		Password     string `json:"password"`
		AgentCode    string `json:"agent_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_BODY", "invalid request body")
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.Name = strings.TrimSpace(req.Name)
	req.BusinessName = strings.TrimSpace(req.BusinessName)

	if req.Name == "" || req.BusinessName == "" || req.Email == "" || req.Password == "" {
		respondError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "name, business_name, email, and password are required")
		return
	}
	if len(req.Password) < 8 {
		respondError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "password must be at least 8 characters")
		return
	}

	// Build a URL-safe slug from business name
	slug := slugify(req.BusinessName)

	ctx := context.Background()

	// Check email uniqueness
	var exists int
	h.db.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE email = $1`, req.Email).Scan(&exists)
	if exists > 0 {
		respondError(w, http.StatusConflict, "EMAIL_TAKEN", "an account with this email already exists")
		return
	}

	// Ensure slug uniqueness
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

	// Insert tenant + user in a transaction
	tx, err := h.db.Begin(ctx)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "could not start transaction")
		return
	}
	defer tx.Rollback(ctx)

	var tenantID string
	err = tx.QueryRow(ctx, `
		INSERT INTO tenants (name, slug, type, status, settings)
		VALUES ($1, $2, 'operator', 'pending_kyc', $3)
		RETURNING id
	`, req.BusinessName, slug, fmt.Sprintf(`{"district":"%s"}`, req.District)).Scan(&tenantID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "could not create tenant")
		return
	}

	var userID string
	err = tx.QueryRow(ctx, `
		INSERT INTO users (tenant_id, email, phone, name, role, password, status)
		VALUES ($1, $2, $3, $4, 'operator', $5, 'pending_kyc')
		RETURNING id
	`, tenantID, req.Email, req.Phone, req.Name, string(hash)).Scan(&userID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "could not create user")
		return
	}

	if err := tx.Commit(ctx); err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "could not commit registration")
		return
	}

	// Wire agent referral if a valid agent_code was provided
	if req.AgentCode != "" {
		var agentTenantID string
		err := h.db.QueryRow(ctx,
			`SELECT id FROM tenants WHERE slug = $1 AND type = 'agent' AND status = 'active' LIMIT 1`,
			req.AgentCode,
		).Scan(&agentTenantID)
		if err == nil {
			h.db.Exec(ctx,
				`INSERT INTO agent_referrals (agent_id, operator_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
				agentTenantID, tenantID,
			)
		}
	}

	respond(w, http.StatusCreated, map[string]string{
		"id":     userID,
		"email":  req.Email,
		"status": "pending_kyc",
	})
}

// slugify converts a business name to a URL-safe slug.
func slugify(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		} else if r == ' ' || r == '-' || r == '_' {
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

func (h *Handler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r)

	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_BODY", "invalid request body")
		return
	}
	if len(req.NewPassword) < 8 {
		respondError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "new password must be at least 8 characters")
		return
	}

	ctx := context.Background()
	var hash string
	if err := h.db.QueryRow(ctx, `SELECT password FROM users WHERE id = $1`, claims.UserID).Scan(&hash); err != nil {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "user not found")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.CurrentPassword)); err != nil {
		respondError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "current password is incorrect")
		return
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "HASH_ERROR", "could not hash password")
		return
	}
	h.db.Exec(ctx, `UPDATE users SET password = $1, updated_at = NOW() WHERE id = $2`, string(newHash), claims.UserID)
	respond(w, http.StatusOK, map[string]string{"message": "password updated"})
}
