package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alekpopovic/orch/internal/api"
	"github.com/alekpopovic/orch/internal/auth"
	"github.com/alekpopovic/orch/internal/cli"
	"github.com/alekpopovic/orch/internal/config"
	"github.com/alekpopovic/orch/internal/controlplane"
	"github.com/alekpopovic/orch/internal/gitops"
	"github.com/alekpopovic/orch/internal/logging"
	"github.com/alekpopovic/orch/internal/metrics"
	"github.com/alekpopovic/orch/internal/node"
	"github.com/alekpopovic/orch/internal/rollout"
	"github.com/alekpopovic/orch/internal/secrets"
	"gopkg.in/yaml.v3"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, printConfig, err := loadConfig(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if printConfig {
		if err := yaml.NewEncoder(os.Stdout).Encode(cfg.Redacted()); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	logger := logging.NewLogger(cfg.LogLevel)
	if err := run(ctx, logger, cfg); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func loadConfig(args []string) (config.ServerConfig, bool, error) {
	printConfig := false
	if len(args) >= 2 && args[0] == "config" && args[1] == "print" {
		printConfig = true
		args = args[2:]
	}
	var configPath string
	var overrides config.ServerOverrides
	fs := flag.NewFlagSet("orch-server", flag.ContinueOnError)
	fs.StringVar(&configPath, "config", "", "YAML config file")
	fs.StringVar(&overrides.Addr, "addr", "", "server listen address")
	fs.StringVar(&overrides.DatabaseURL, "database-url", "", "PostgreSQL database URL")
	fs.StringVar(&overrides.LogLevel, "log-level", "", "log level")
	fs.StringVar(&overrides.BootstrapToken, "bootstrap-token", "", "agent bootstrap token")
	fs.StringVar(&overrides.JWTSecret, "jwt-secret", "", "JWT signing secret")
	fs.StringVar(&overrides.Users, "users", "", "static user role map")
	fs.StringVar(&overrides.SecretKey, "secret-key", "", "secret encryption key")
	fs.StringVar(&overrides.GracefulShutdownTTL, "shutdown-timeout", "", "graceful shutdown timeout")
	fs.StringVar(&overrides.HeartbeatTimeout, "node-heartbeat-timeout", "", "node heartbeat timeout")
	fs.StringVar(&overrides.NodeMonitorInterval, "node-monitor-interval", "", "node monitor interval")
	if err := fs.Parse(args); err != nil {
		return config.ServerConfig{}, false, err
	}
	cfg, err := config.LoadServerWithFile(configPath, overrides)
	return cfg, printConfig, err
}

func run(ctx context.Context, logger *slog.Logger, cfg config.ServerConfig) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	users, err := auth.ParseStaticUsers(cfg.Users)
	if err != nil {
		return err
	}

	envelope, err := secrets.NewLocalEnvelope(cfg.SecretKey)
	if err != nil {
		return err
	}
	controlPlane := controlplane.NewMemoryService(controlplane.WithSecretEnvelope(envelope), controlplane.WithClusterPolicy(cfg.ClusterPolicy))
	gitopsController := gitops.NewController(controlPlane, nil, cli.ParseDeploy, logger)
	go func() {
		if err := gitopsController.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Warn("GitOps controller stopped", "error", err)
		}
	}()
	serverMetrics := metrics.NewServer()
	rolloutController := rollout.NewController(controlPlane, logger, rollout.WithMetrics(serverMetrics))
	go func() {
		if err := rolloutController.Run(ctx, 5*time.Second); err != nil && !errors.Is(err, context.Canceled) {
			logger.Warn("rollout controller stopped", "error", err)
		}
	}()
	nodeMonitor := node.NewMonitor(controlPlane, logger, cfg.HeartbeatTimeout, cfg.NodeMonitorInterval)
	go func() {
		if err := nodeMonitor.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Warn("node monitor stopped", "error", err)
		}
	}()

	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           api.NewHandler(logger, controlPlane, api.WithBootstrapToken(cfg.BootstrapToken), api.WithUserJWT(cfg.JWTSecret), api.WithStaticUsers(users), api.WithRequestMetrics(serverMetrics), api.WithControlMetrics(serverMetrics), api.WithMetricsHandler(serverMetrics.Handler()), api.WithGitOpsSyncer(gitopsController)),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("starting server", "addr", cfg.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		logger.Info("shutting down server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.GracefulShutdownTTL)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return nil
	}
}
