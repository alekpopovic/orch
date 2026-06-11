package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alekpopovic/orch/internal/config"
	"github.com/alekpopovic/orch/internal/logging"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := config.LoadAgent()
	logger := logging.NewLogger(cfg.LogLevel)
	if err := run(ctx, logger, cfg); err != nil {
		logger.Error("agent stopped", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, logger *slog.Logger, cfg config.AgentConfig) error {
	if err := cfg.Validate(); err != nil {
		return err
	}

	logger.Info("starting agent", "node_id", cfg.NodeID, "server_url", cfg.ServerURL)

	ticker := time.NewTicker(cfg.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("shutting down agent", "node_id", cfg.NodeID)
			shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.GracefulShutdownTTL)
			defer cancel()
			return shutdown(shutdownCtx)
		case checkedAt := <-ticker.C:
			logger.Info("agent heartbeat", "node_id", cfg.NodeID, "checked_at", checkedAt.UTC())
		}
	}
}

func shutdown(ctx context.Context) error {
	return ctx.Err()
}
