package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/elearning/checker-service/internal/config"
	"github.com/elearning/checker-service/internal/handlers"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	cfg := config.Load()
	h := handlers.New(cfg)

	addr := fmt.Sprintf(":%d", cfg.Port)
	slog.Info("checker-service starting", "addr", addr, "gitlab_base_url", cfg.GitLabBaseURL)

	if err := http.ListenAndServe(addr, h.BuildRouter()); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}
