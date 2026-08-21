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
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/alekpopovic/orch/internal/api"
	"github.com/alekpopovic/orch/internal/auth"
	"github.com/alekpopovic/orch/internal/cli"
	"github.com/alekpopovic/orch/internal/config"
	"github.com/alekpopovic/orch/internal/controlplane"
	"github.com/alekpopovic/orch/internal/cronjobs"
	orchdns "github.com/alekpopovic/orch/internal/dns"
	"github.com/alekpopovic/orch/internal/gitops"
	"github.com/alekpopovic/orch/internal/logging"
	"github.com/alekpopovic/orch/internal/metrics"
	"github.com/alekpopovic/orch/internal/migrations"
	"github.com/alekpopovic/orch/internal/node"
	"github.com/alekpopovic/orch/internal/notifications"
	"github.com/alekpopovic/orch/internal/rollout"
	"github.com/alekpopovic/orch/internal/scheduler"
	"github.com/alekpopovic/orch/internal/secrets"
	versioninfo "github.com/alekpopovic/orch/internal/version"
	"gopkg.in/yaml.v3"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if len(os.Args) > 2 && os.Args[1] == "migrate" {
		if err := runMigrationCommand(ctx, os.Args[2], os.Args[3:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

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
	fs.StringVar(&overrides.DebugAddr, "debug-addr", "", "admin debug listener address")
	fs.Func("enable-pprof", "enable pprof on the admin debug listener", func(raw string) error {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return fmt.Errorf("enable-pprof: %w", err)
		}
		overrides.EnablePprof = &value
		return nil
	})
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
	if strings.EqualFold(os.Getenv("ORCH_SCHEMA_CHECK"), "true") {
		runner, err := migrations.Open(ctx, cfg.DatabaseURL, "migrations")
		if err != nil {
			return fmt.Errorf("database schema check: %w", err)
		}
		status, statusErr := runner.Status(ctx)
		runner.Close()
		if statusErr != nil {
			return statusErr
		}
		if err = checkSchemaVersion(status.Current); err != nil {
			return err
		}
	}
	users, err := auth.ParseStaticUsers(cfg.Users)
	if err != nil {
		return err
	}
	var debugServer *http.Server
	if cfg.EnablePprof {
		debugServer = &http.Server{Addr: cfg.DebugAddr, Handler: api.NewDebugHandler(true), ReadHeaderTimeout: 5 * time.Second}
		go func() {
			logger.Info("starting admin debug listener", "addr", cfg.DebugAddr)
			if err := debugServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				logger.Error("debug listener stopped", "error", err)
			}
		}()
		go func() {
			<-ctx.Done()
			shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.GracefulShutdownTTL)
			defer cancel()
			_ = debugServer.Shutdown(shutdownCtx)
		}()
	}

	envelope, err := secrets.NewLocalEnvelope(cfg.SecretKey)
	if err != nil {
		return err
	}
	controlPlane := controlplane.NewMemoryService(controlplane.WithSecretEnvelope(envelope), controlplane.WithClusterPolicy(cfg.ClusterPolicy), controlplane.WithRetentionConfig(cfg.Retention))
	controlPlane.SetNotificationDispatcher(notifications.NewDispatcher(controlPlane, nil))
	serverMetrics := metrics.NewServer()
	if dnsAddress := os.Getenv("ORCH_DNS_ADDR"); dnsAddress != "" {
		dnsTTL := 30 * time.Second
		if raw := os.Getenv("ORCH_DNS_TTL"); raw != "" {
			parsed, parseErr := time.ParseDuration(raw)
			if parseErr != nil || parsed <= 0 {
				return fmt.Errorf("ORCH_DNS_TTL must be a positive duration")
			}
			dnsTTL = parsed
		}
		dnsServer := orchdns.New(controlPlane, dnsTTL, "default", serverMetrics)
		go func() {
			if err := dnsServer.ServeUDP(ctx, dnsAddress); err != nil && !errors.Is(err, context.Canceled) {
				logger.Warn("internal DNS stopped", "error", err)
			}
		}()
	}
	gitopsController := gitops.NewController(controlPlane, nil, cli.ParseDeploy, logger)
	go func() {
		if err := gitopsController.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Warn("GitOps controller stopped", "error", err)
		}
	}()
	cronController := cronjobs.New(controlPlane, nil)
	go func() {
		if err := cronController.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Warn("cronjob controller stopped", "error", err)
		}
	}()
	schedulerController := scheduler.New(controlPlane, scheduler.WithMetrics(serverMetrics))
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, err := schedulerController.RunOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
					logger.Warn("scheduler iteration failed", "error", err)
				}
			}
		}
	}()
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		_, _ = controlPlane.CaptureUsageSnapshots(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_, _ = controlPlane.CaptureUsageSnapshots(ctx)
			}
		}
	}()
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, err := controlPlane.PruneRetention(ctx, false); err != nil && !errors.Is(err, context.Canceled) {
					logger.Warn("retention pruning failed", "error", err)
				}
			}
		}
	}()
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

func checkSchemaVersion(value string) error {
	normalized := strings.TrimLeft(strings.TrimSpace(value), "0")
	if normalized == "" {
		normalized = "0"
	}
	current, err := strconv.Atoi(normalized)
	if err != nil {
		return fmt.Errorf("invalid database schema version %q", value)
	}
	return versioninfo.CheckSchema(current)
}

func runMigrationCommand(ctx context.Context, action string, args []string) error {
	cfg := config.LoadServer()
	fs := flag.NewFlagSet("migrate "+action, flag.ContinueOnError)
	databaseURL := cfg.DatabaseURL
	allowDown := false
	fs.StringVar(&databaseURL, "database-url", databaseURL, "PostgreSQL database URL")
	fs.BoolVar(&allowDown, "allow-down", false, "explicitly allow one down migration")
	if err := fs.Parse(args); err != nil {
		return err
	}
	runner, err := migrations.Open(ctx, databaseURL, "migrations")
	if err != nil {
		return err
	}
	defer runner.Close()
	switch action {
	case "status":
		status, err := runner.Status(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("current=%s latest=%s pending=%d\n", status.Current, status.Latest, len(status.Pending))
		return nil
	case "up":
		return runner.Up(ctx)
	case "down":
		return runner.Down(ctx, allowDown)
	default:
		return fmt.Errorf("unknown migrate action %q (use status, up, or down)", action)
	}
}
