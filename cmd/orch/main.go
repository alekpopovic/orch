package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/alekpopovic/orch/internal/logging"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger := logging.NewLogger(os.Getenv("ORCH_LOG_LEVEL"))
	if err := run(ctx, logger, os.Args[1:]); err != nil {
		logger.Error("command failed", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, logger *slog.Logger, args []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	command := "help"
	if len(args) > 0 {
		command = args[0]
	}

	switch command {
	case "help", "-h", "--help":
		fmt.Println("orch is the CLI for the orch container orchestrator.")
		fmt.Println()
		fmt.Println("Usage:")
		fmt.Println("  orch help")
		return nil
	case "version":
		fmt.Println("orch dev")
		return nil
	default:
		logger.Info("unknown command", "command", command)
		return fmt.Errorf("unknown command %q", command)
	}
}
