package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"github.com/elearning/ldap-service/internal/config"
	"github.com/elearning/ldap-service/internal/handlers"
)

func main() {
	_ = godotenv.Load()

	cfg := config.Load()

	pool, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("DB connect error: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(context.Background()); err != nil {
		log.Fatalf("DB ping failed: %v", err)
	}

	state := &handlers.State{Pool: pool, Config: cfg}
	router := handlers.BuildRouter(state, cfg, pool, os.Getenv("ENV") != "production")

	addr := ":" + strconv.Itoa(cfg.Port)
	srv := &http.Server{
		Addr:        addr,
		Handler:     router,
		ReadTimeout: 15 * time.Second,
		// No WriteTimeout: LDAP auth via Authentik can take minutes (auth flow evaluation)
		// Per-request timeouts are handled inside each handler via context.
		IdleTimeout: 60 * time.Second,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		fmt.Printf("ldap-service listening on %s\n", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	<-quit
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
}
