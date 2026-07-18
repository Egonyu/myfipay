package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/myfibase/myfibase/internal/config"
	"github.com/myfibase/myfibase/internal/database"
	"github.com/myfibase/myfibase/internal/rdb"
	"github.com/myfibase/myfibase/internal/server"
)

func main() {
	cfg := config.Load()

	pool, err := database.Connect(cfg)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()

	cache, err := rdb.Connect(cfg)
	if err != nil {
		log.Fatalf("redis: %v", err)
	}
	defer cache.Close()

	// ctx tied to process lifetime — cancelled on SIGINT/SIGTERM
	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()

	srv := server.New(ctx, cfg, pool, cache)

	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Port),
		Handler:      srv,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("myFiBase starting on :%s (env=%s)", cfg.Port, cfg.AppEnv)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("shutting down...")
	cancelCtx() // stop background goroutines
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutCancel()
	httpServer.Shutdown(shutCtx)
}
