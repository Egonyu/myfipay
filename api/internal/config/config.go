package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	AppEnv  string
	Port    string
	AppSecret string

	DatabaseURL string
	RedisURL    string
	RedisPassword string

	ZengapayAPIURL       string
	ZengapayAPIToken     string
	ZengapayWebhookSecret string
}

func Load() *Config {
	if err := godotenv.Load("/var/www/myfibase/.env"); err != nil {
		log.Println("no .env file, using environment variables")
	}

	return &Config{
		AppEnv:    getEnv("APP_ENV", "development"),
		Port:      getEnv("APP_PORT", "8080"),
		AppSecret: mustEnv("APP_SECRET"),

		DatabaseURL:   buildDSN(),
		RedisURL:      getEnv("REDIS_URL", "redis://localhost:6379"),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),

		ZengapayAPIURL:        getEnv("ZENGAPAY_API_URL", "https://api.zengapay.com"),
		ZengapayAPIToken:      getEnv("ZENGAPAY_API_TOKEN", ""),
		ZengapayWebhookSecret: getEnv("ZENGAPAY_WEBHOOK_SECRET", ""),
	}
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
