//go:build integration

// Integration test for the money path (P0-E): portal pay → ZengaPay webhook →
// session grant + RADIUS rows + confirmed payment + agent commission, plus
// webhook dedup and signature rejection — against ephemeral Postgres + Redis.
//
// Skipped unless TEST_DATABASE_URL and TEST_REDIS_URL are set. Run locally via
// scripts/integration-test.sh; in CI via the integration job (service
// containers). Never point these at the production database.
package integration

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/myfibase/myfibase/internal/config"
	"github.com/myfibase/myfibase/internal/server"
	"github.com/redis/go-redis/v9"
)

const (
	portalSlug    = "itest"
	planPriceUGX  = 1000
	webhookSecret = "itest-webhook-secret"
	customerPhone = "256700000001"
	customerMAC   = "aa:bb:cc:dd:ee:ff"
)

func TestPayWebhookSessionFlow(t *testing.T) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	redisURL := os.Getenv("TEST_REDIS_URL")
	if dbURL == "" || redisURL == "" {
		t.Skip("TEST_DATABASE_URL / TEST_REDIS_URL not set")
	}
	ctx := context.Background()

	db := mustPool(t, ctx, dbURL)
	applyMigrations(t, ctx, db)

	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		t.Fatalf("redis url: %v", err)
	}
	cache := redis.NewClient(opt)
	if err := cache.Ping(ctx).Err(); err != nil {
		t.Fatalf("redis ping: %v", err)
	}

	// Stub ZengaPay collections API
	var zengaCalls int
	var zengaExternalRef string
	zenga := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		zengaCalls++
		zengaExternalRef, _ = req["external_reference"].(string)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"code":202,"status":"pending"}`))
	}))
	defer zenga.Close()

	cfg := &config.Config{
		AppEnv:                "test",
		AppSecret:             "itest-secret",
		JWTSecret:             "itest-secret",
		JWTExpiryHours:        1,
		ZengapayAPIURL:        zenga.URL,
		ZengapayAPIToken:      "itest-token",
		ZengapayWebhookSecret: webhookSecret,
	}

	app := httptest.NewServer(server.New(ctx, cfg, db, cache))
	defer app.Close()

	planID := seed(t, ctx, db)

	// --- 1. Portal pay ---
	payBody, _ := json.Marshal(map[string]string{
		"plan_id": planID,
		"phone":   customerPhone,
		"mac":     customerMAC,
	})
	resp, err := http.Post(app.URL+"/portal/"+portalSlug+"/pay", "application/json", bytes.NewReader(payBody))
	if err != nil {
		t.Fatalf("pay: %v", err)
	}
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("pay: got %d, want 202", resp.StatusCode)
	}
	var payResp struct {
		Data struct {
			PaymentID string `json:"payment_id"`
		} `json:"data"`
		PaymentID string `json:"payment_id"`
	}
	json.NewDecoder(resp.Body).Decode(&payResp)
	resp.Body.Close()
	paymentID := payResp.Data.PaymentID
	if paymentID == "" {
		paymentID = payResp.PaymentID
	}
	if paymentID == "" {
		t.Fatal("pay: no payment_id in response")
	}
	if zengaCalls != 1 {
		t.Fatalf("zengapay stub calls: got %d, want 1", zengaCalls)
	}
	if zengaExternalRef != paymentID {
		t.Fatalf("zengapay external_reference: got %q, want %q", zengaExternalRef, paymentID)
	}

	// --- 2. Signed success webhook ---
	webhook := webhookBody(paymentID, "itest-zref-1")
	code := postWebhook(t, app.URL, webhook, signHMAC(webhook, webhookSecret))
	if code != http.StatusOK {
		t.Fatalf("webhook: got %d, want 200", code)
	}

	// Session creation is async (goroutine) — poll.
	waitFor(t, 5*time.Second, "session row", func() bool {
		return count(t, ctx, db, `SELECT COUNT(*) FROM sessions WHERE username=$1 AND status='active'`, customerPhone) == 1
	})

	// --- 3. Assert the money path wrote everything ---
	if n := count(t, ctx, db, `SELECT COUNT(*) FROM radcheck WHERE username=$1 AND attribute='Auth-Type' AND value='Accept'`, customerPhone); n != 1 {
		t.Errorf("radcheck Auth-Type rows: got %d, want 1", n)
	}
	if n := count(t, ctx, db, `SELECT COUNT(*) FROM radreply WHERE username=$1 AND attribute='Session-Timeout'`, customerPhone); n != 1 {
		t.Errorf("radreply Session-Timeout rows: got %d, want 1", n)
	}
	if n := count(t, ctx, db, `SELECT COUNT(*) FROM payments WHERE customer_phone=$1 AND status='confirmed' AND amount_ugx=$2 AND zengapay_ref='itest-zref-1'`, customerPhone, planPriceUGX); n != 1 {
		t.Errorf("confirmed payment rows: got %d, want 1", n)
	}
	wantCommission := int(float64(planPriceUGX) * 0.03)
	if n := count(t, ctx, db, `SELECT COUNT(*) FROM commissions WHERE amount_ugx=$1`, wantCommission); n != 1 {
		t.Errorf("commission rows for %d UGX: got %d, want 1", wantCommission, n)
	}

	// --- 4. Duplicate webhook is deduplicated ---
	code = postWebhook(t, app.URL, webhook, signHMAC(webhook, webhookSecret))
	if code != http.StatusOK {
		t.Fatalf("duplicate webhook: got %d, want 200", code)
	}
	time.Sleep(1500 * time.Millisecond) // would-be async session write
	if n := count(t, ctx, db, `SELECT COUNT(*) FROM sessions WHERE username=$1`, customerPhone); n != 1 {
		t.Errorf("sessions after duplicate webhook: got %d, want 1", n)
	}
	if n := count(t, ctx, db, `SELECT COUNT(*) FROM payments WHERE customer_phone=$1`, customerPhone); n != 1 {
		t.Errorf("payments after duplicate webhook: got %d, want 1", n)
	}
	if n := count(t, ctx, db, `SELECT COUNT(*) FROM commissions`); n != 1 {
		t.Errorf("commissions after duplicate webhook: got %d, want 1", n)
	}

	// --- 4b. Second success webhook with a DIFFERENT transactionReference for
	// the same external reference (gateway retry / forged duplicate) must not
	// double-credit. Observed live 2026-07-19: sandbox webhook + test webhook
	// each created a confirmed payment and commission before this guard.
	fresh := webhookBody(paymentID, "itest-zref-retry")
	code = postWebhook(t, app.URL, fresh, signHMAC(fresh, webhookSecret))
	if code != http.StatusOK {
		t.Fatalf("fresh-ref duplicate webhook: got %d, want 200", code)
	}
	time.Sleep(1500 * time.Millisecond) // would-be async session write
	if n := count(t, ctx, db, `SELECT COUNT(*) FROM payments WHERE customer_phone=$1 AND status='confirmed'`, customerPhone); n != 1 {
		t.Errorf("payments after fresh-ref duplicate: got %d, want 1", n)
	}
	if n := count(t, ctx, db, `SELECT COUNT(*) FROM commissions`); n != 1 {
		t.Errorf("commissions after fresh-ref duplicate: got %d, want 1", n)
	}

	// --- 5. Bad signature is rejected ---
	bad := webhookBody("someone-else", "itest-zref-2")
	code = postWebhook(t, app.URL, bad, signHMAC(bad, "wrong-secret"))
	if code != http.StatusUnauthorized {
		t.Fatalf("bad-signature webhook: got %d, want 401", code)
	}

	// --- 6. Payment status reflects success ---
	resp, err = http.Get(app.URL + "/portal/" + portalSlug + "/pay/" + paymentID + "/status")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	var statusResp struct {
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
		Status string `json:"status"`
	}
	json.NewDecoder(resp.Body).Decode(&statusResp)
	resp.Body.Close()
	status := statusResp.Data.Status
	if status == "" {
		status = statusResp.Status
	}
	if status != "successful" {
		t.Errorf("payment status: got %q, want \"successful\"", status)
	}
}

func mustPool(t *testing.T, ctx context.Context, url string) *pgxpool.Pool {
	t.Helper()
	pcfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatalf("db url: %v", err)
	}
	// Simple protocol so multi-statement migration files can be Exec'd whole.
	pcfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	pool, err := pgxpool.NewWithConfig(ctx, pcfg)
	if err != nil {
		t.Fatalf("db connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("db ping: %v", err)
	}
	return pool
}

func applyMigrations(t *testing.T, ctx context.Context, db *pgxpool.Pool) {
	t.Helper()
	files, err := filepath.Glob("../db/migrations/*.sql")
	if err != nil || len(files) == 0 {
		t.Fatalf("no migration files found: %v", err)
	}
	sort.Strings(files)
	for _, f := range files {
		sql, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		if _, err := db.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("apply %s: %v", filepath.Base(f), err)
		}
	}
}

// seed creates agent → operator referral, a location, and a plan; returns the plan ID.
func seed(t *testing.T, ctx context.Context, db *pgxpool.Pool) string {
	t.Helper()
	var agentID, operatorID, locationID, planID string
	q := func(dst *string, sql string, args ...any) {
		t.Helper()
		if err := db.QueryRow(ctx, sql, args...).Scan(dst); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	q(&agentID, `INSERT INTO tenants (name, slug, type) VALUES ('IT Agent', 'itest-agent', 'agent') RETURNING id`)
	q(&operatorID, `INSERT INTO tenants (name, slug, type) VALUES ('IT Operator', 'itest-op', 'operator') RETURNING id`)
	q(&locationID, `INSERT INTO locations (tenant_id, name, portal_slug) VALUES ($1, 'IT Cafe', $2) RETURNING id`, operatorID, portalSlug)
	q(&planID, `INSERT INTO plans (location_id, name, price_ugx, duration_mins, speed_down_kbps, speed_up_kbps)
	            VALUES ($1, 'IT Hour', $2, 60, 2048, 512) RETURNING id`, locationID, planPriceUGX)
	if _, err := db.Exec(ctx, `INSERT INTO agent_referrals (agent_id, operator_id) VALUES ($1, $2)`, agentID, operatorID); err != nil {
		t.Fatalf("seed referral: %v", err)
	}
	return planID
}

func webhookBody(externalRef, zengaRef string) []byte {
	b, _ := json.Marshal(map[string]any{
		"event": "collection.success",
		"data": map[string]string{
			"transactionReference":         zengaRef,
			"transactionExternalReference": externalRef,
			"transactionStatus":            "SUCCEEDED",
			"amount":                       fmt.Sprintf("%d.00", planPriceUGX),
			"msisdn":                       customerPhone,
		},
	})
	return b
}

func signHMAC(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func postWebhook(t *testing.T, baseURL string, body []byte, sig string) int {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, baseURL+"/webhooks/zengapay", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Signature", sig)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("webhook post: %v", err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func count(t *testing.T, ctx context.Context, db *pgxpool.Pool, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
		t.Fatalf("count %q: %v", sql, err)
	}
	return n
}

func waitFor(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out after %s waiting for %s", timeout, what)
}
