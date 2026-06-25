package rdb

import (
	"context"
	"fmt"
	"time"

	"github.com/myfibase/myfibase/internal/config"
	"github.com/redis/go-redis/v9"
)

func Connect(cfg *config.Config) (*redis.Client, error) {
	opt, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		opt = &redis.Options{Addr: "localhost:6379"}
	}
	if cfg.RedisPassword != "" {
		opt.Password = cfg.RedisPassword
	}

	client := redis.NewClient(opt)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping: %w", err)
	}

	return client, nil
}
