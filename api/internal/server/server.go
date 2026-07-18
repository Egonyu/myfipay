package server

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/myfibase/myfibase/internal/config"
	"github.com/myfibase/myfibase/internal/handlers"
	"github.com/myfibase/myfibase/internal/middleware"
	"github.com/redis/go-redis/v9"
)

func New(ctx context.Context, cfg *config.Config, db *pgxpool.Pool, cache *redis.Client) http.Handler {
	r := chi.NewRouter()

	r.Use(chiMiddleware.RealIP)
	r.Use(chiMiddleware.Logger)
	r.Use(chiMiddleware.Recoverer)
	r.Use(chiMiddleware.CleanPath)
	r.Use(corsMiddleware)

	h := handlers.New(cfg, db, cache)
	go h.StartSessionReaper(ctx)

	// Health
	r.Get("/health", h.Health)

	// Captive portal — public
	r.Route("/portal/{slug}", func(r chi.Router) {
		r.Get("/", h.PortalPage)
		r.Post("/pay", h.InitiatePayment)
		r.Get("/pay/{paymentID}/status", h.PaymentStatus)
		r.Post("/voucher", h.RedeemVoucher)
		r.Get("/session", h.SessionStatus)
	})

	// ZengaPay webhook — public, HMAC verified
	r.Post("/webhooks/zengapay", h.ZengapayWebhook)

	// Auth — public
	r.Post("/api/auth/login", h.Login)
	r.Post("/api/auth/register", h.Register)
	r.Post("/api/auth/register/agent", h.RegisterAgent)

	// Operator API — JWT required
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAuth(cfg.JWTSecret))

		r.Post("/api/auth/logout", h.Logout)
		r.Get("/api/auth/me", h.Me)
		r.Put("/api/auth/password", h.ChangePassword)

		// Dashboard stats
		r.Get("/api/dashboard/stats", h.DashboardStats)
		r.Get("/api/dashboard/revenue-chart", h.RevenueChart)

		// Payments
		r.Get("/api/payments", h.ListPayments)

		// Payouts (operator settlement)
		r.Get("/api/payouts", h.ListPayouts)
		r.Get("/api/payouts/balance", h.GetPayoutBalance)
		r.Post("/api/payouts", h.RequestOperatorPayout)

		// Sessions
		r.Get("/api/sessions", h.ListSessions)
		r.Post("/api/sessions/grant", h.GrantSession)
		r.Post("/api/sessions/{id}/extend", h.ExtendSession)
		r.Delete("/api/sessions/{id}", h.TerminateSession)

		// Locations
		r.Get("/api/locations", h.ListOperatorLocations)
		r.Post("/api/locations", h.CreateLocation)
		r.Get("/api/locations/{id}/branding", h.GetLocationBranding)
		r.Put("/api/locations/{id}/branding", h.UpdateLocationBranding)

		// Devices (routers)
		r.Get("/api/devices", h.ListDevices)
		r.Post("/api/devices", h.CreateDevice)
		r.Put("/api/devices/{id}", h.UpdateDevice)
		r.Delete("/api/devices/{id}", h.DeleteDevice)
		r.Get("/api/devices/{id}/script", h.DeviceScript)
		r.Get("/api/devices/{id}/status", h.DeviceStatus)

		// Plans
		r.Get("/api/plans", h.ListOperatorPlans)
		r.Post("/api/plans", h.CreatePlan)
		r.Put("/api/plans/{id}", h.UpdatePlan)
		r.Delete("/api/plans/{id}", h.DeletePlan)

		// Vouchers
		r.Post("/api/vouchers/batches", h.CreateVoucherBatch)
		r.Get("/api/vouchers/batches", h.ListVoucherBatches)
		r.Get("/api/vouchers/batches/{id}", h.GetVoucherBatch)

		// Profile
		r.Get("/api/profile", h.GetProfile)
		r.Put("/api/profile", h.UpdateProfile)

		// Agent API
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireRole("agent"))
			r.Get("/api/agent/dashboard", h.AgentDashboard)
			r.Get("/api/agent/invite", h.AgentInviteLink)
			r.Get("/api/agent/operators", h.AgentOperators)
			r.Get("/api/agent/commissions", h.AgentCommissions)
			r.Post("/api/agent/payouts", h.RequestPayout)
			r.Get("/api/agent/payouts", h.ListPayoutRequests)
		})

		// Admin only
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireRole("admin", "super_admin"))
			r.Post("/api/admin/operators", h.CreateOperator)
			r.Get("/api/admin/kyc", h.ListKYCQueue)
			r.Post("/api/admin/kyc/{id}/approve", h.ApproveKYC)
			r.Post("/api/admin/kyc/{id}/reject", h.RejectKYC)
			r.Get("/api/admin/tenants", h.ListTenants)
			r.Get("/api/admin/revenue", h.PlatformRevenue)

			// Operator payout queue
			r.Get("/api/admin/payouts", h.ListPayoutQueue)
			r.Post("/api/admin/payouts/{id}/approve", h.ApprovePayout)
			r.Post("/api/admin/payouts/{id}/reject", h.RejectPayout)
			r.Post("/api/admin/payouts/{id}/mark-paid", h.MarkOperatorPayoutPaid)

			// Agent management
			r.Get("/api/admin/agents", h.ListAgents)
			r.Get("/api/admin/agent-payouts", h.ListAllPayoutRequests)
			r.Post("/api/admin/agent-payouts/{id}/approve", h.ApprovePayoutRequest)
			r.Post("/api/admin/agent-payouts/{id}/paid", h.MarkPayoutPaid)
			r.Post("/api/admin/agent-payouts/{id}/reject", h.RejectPayoutRequest)
		})
	})

	return r
}

// corsMiddleware allows the dashboard origin during development.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type,Authorization")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
