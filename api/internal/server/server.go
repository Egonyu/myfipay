package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/myfibase/myfibase/internal/config"
	"github.com/myfibase/myfibase/internal/handlers"
	"github.com/redis/go-redis/v9"
)

func New(cfg *config.Config, db *pgxpool.Pool, cache *redis.Client) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.CleanPath)

	h := handlers.New(cfg, db, cache)

	// Health
	r.Get("/health", h.Health)

	// Captive portal — public, no auth
	r.Route("/portal/{slug}", func(r chi.Router) {
		r.Get("/", h.PortalPage)
		r.Post("/pay", h.InitiatePayment)
		r.Get("/pay/{paymentID}/status", h.PaymentStatus)
		r.Post("/voucher", h.RedeemVoucher)
		r.Get("/session", h.SessionStatus)
	})

	// ZengaPay webhook — public, HMAC verified
	r.Post("/webhooks/zengapay", h.ZengapayWebhook)

	// Operator API — auth required (Phase 2)
	r.Route("/api", func(r chi.Router) {
		r.Get("/locations", h.ListLocations)
		r.Get("/locations/{id}/plans", h.ListPlans)
	})

	return r
}
