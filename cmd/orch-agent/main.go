package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alekpopovic/orch/internal/agent"
	"github.com/alekpopovic/orch/internal/config"
	orchdocker "github.com/alekpopovic/orch/internal/docker"
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
	runtime, err := orchdocker.NewEngineRuntimeFromEnv()
	if err != nil {
		return err
	}
	runner := agent.NewRunner(cfg, nil, logger).WithRuntime(runtime)
	server := &http.Server{
		Addr:              cfg.AgentAddr,
		Handler:           agent.NewLogHandler(runtime, cfg.BootstrapToken, logger),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 2)
	go func() {
		logger.Info("starting agent log server", "addr", cfg.AgentAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()
	go func() {
		errCh <- runner.Run(ctx)
	}()

	select {
	case err := <-errCh:
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.GracefulShutdownTTL)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.GracefulShutdownTTL)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return nil
	}
}
