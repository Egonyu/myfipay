package handlers

import (
	"encoding/json"
	"net/http"

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
