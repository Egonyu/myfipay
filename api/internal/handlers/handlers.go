package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/myfibase/myfibase/internal/config"
	"github.com/redis/go-redis/v9"
)

type Handler struct {
	cfg   *config.Config
	db    *pgxpool.Pool
	cache *redis.Client
}

func New(cfg *config.Config, db *pgxpool.Pool, cache *redis.Client) *Handler {
	return &Handler{cfg: cfg, db: db, cache: cache}
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	respond(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"service": "myfibase-api",
	})
}

// StartSessionReaper polls the DB every 60 seconds and expires overdue sessions.
// This replaces per-session goroutines that were lost on process restart.
// Call in a goroutine; exits when ctx is cancelled.
func (h *Handler) StartSessionReaper(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.reapExpiredSessions(ctx)
		}
	}
}

func (h *Handler) reapExpiredSessions(ctx context.Context) {
	rows, err := h.db.Query(ctx,
		`SELECT id, username FROM sessions WHERE status = 'active' AND expires_at < NOW()`)
	if err != nil {
		return
	}
	defer rows.Close()
	type row struct{ id, username string }
	var expired []row
	for rows.Next() {
		var r row
		rows.Scan(&r.id, &r.username)
		expired = append(expired, r)
	}
	for _, r := range expired {
		h.expireSession(r.username, r.id)
		log.Printf("session reaper: expired session %s (user %s)", r.id, r.username)
	}
}

// respond writes a JSON response.
func respond(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{
		"success": status < 400,
		"data":    data,
	})
}

func respondError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{
		"success": false,
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	})
}
