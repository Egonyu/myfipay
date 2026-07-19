package config

import (
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	AppEnv    string
	Port      string
	AppSecret string

	DatabaseURL   string
	RedisURL      string
	RedisPassword string

	JWTSecret      string
	JWTExpiryHours int

	ZengapayAPIURL        string
	ZengapayAPIToken      string
	ZengapayWebhookSecret string

	// CORSOrigins pins Access-Control-Allow-Origin. Empty list + development
	// env = echo any origin (dev convenience); empty list + production = no
	// cross-origin access (dashboard is same-origin behind nginx anyway).
	CORSOrigins []string

	// ZengapayWebhookIPs restricts /webhooks/zengapay to these IPs/CIDRs.
	// Empty = disabled, same pattern as the empty webhook HMAC secret.
	ZengapayWebhookIPs string

	// WGServerPublicKey is the wg0 public key on the host (cat
	// /etc/wireguard/server.pub). Empty = setup scripts omit the
	// management-tunnel section.
	WGServerPublicKey string
	WGEndpointHost    string
	WGEndpointPort    string
}

func Load() *Config {
	if err := godotenv.Load("/var/www/myfibase/.env"); err != nil {
		log.Println("no .env file, using environment variables")
	}

	jwtExpiry := 24
	if v := getEnv("JWT_EXPIRY_HOURS", ""); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			jwtExpiry = n
		}
	}

	return &Config{
		AppEnv:    getEnv("APP_ENV", "development"),
		Port:      getEnv("APP_PORT", "8080"),
		AppSecret: mustEnv("APP_SECRET"),

		DatabaseURL:   buildDSN(),
		RedisURL:      getEnv("REDIS_URL", "redis://localhost:6379"),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),

		JWTSecret:      getEnv("JWT_SECRET", mustEnv("APP_SECRET")),
		JWTExpiryHours: jwtExpiry,

		ZengapayAPIURL:        getEnv("ZENGAPAY_API_URL", "https://api.zengapay.com"),
		ZengapayAPIToken:      getEnv("ZENGAPAY_API_TOKEN", ""),
		ZengapayWebhookSecret: getEnv("ZENGAPAY_WEBHOOK_SECRET", ""),

		CORSOrigins:        splitList(getEnv("CORS_ALLOWED_ORIGINS", "")),
		ZengapayWebhookIPs: getEnv("ZENGAPAY_WEBHOOK_IPS", ""),

		WGServerPublicKey: getEnv("WG_SERVER_PUBLIC_KEY", ""),
		WGEndpointHost:    getEnv("WG_ENDPOINT_HOST", "170.64.177.20"),
		WGEndpointPort:    getEnv("WG_ENDPOINT_PORT", "51820"),
	}
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func buildDSN() string {
	host := getEnv("POSTGRES_HOST", "localhost")
	port := getEnv("POSTGRES_PORT", "5432")
	user := getEnv("POSTGRES_USER", "myfibase")
	pass := getEnv("POSTGRES_PASSWORD", "")
	db := getEnv("POSTGRES_DB", "myfibase")
	return "postgres://" + user + ":" + pass + "@" + host + ":" + port + "/" + db + "?sslmode=disable"
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("required env var %s is not set", key)
	}
	return v
}
