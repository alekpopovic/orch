package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/alekpopovic/orch/internal/agent"
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
	runner := agent.NewRunner(cfg, nil, logger)
	return runner.Run(ctx)
}
