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

	"github.com/alekpopovic/orch/internal/agent"
	"github.com/alekpopovic/orch/internal/config"
	orchdocker "github.com/alekpopovic/orch/internal/docker"
	"github.com/alekpopovic/orch/internal/logging"
	"github.com/alekpopovic/orch/internal/metrics"
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
		logger.Error("agent stopped", "error", err)
		os.Exit(1)
	}
}

func loadConfig(args []string) (config.AgentConfig, bool, error) {
	printConfig := false
	if len(args) >= 2 && args[0] == "config" && args[1] == "print" {
		printConfig = true
		args = args[2:]
	}
	var configPath string
	var overrides config.AgentOverrides
	fs := flag.NewFlagSet("orch-agent", flag.ContinueOnError)
	fs.StringVar(&configPath, "config", "", "YAML config file")
	fs.StringVar(&overrides.NodeName, "node-name", "", "stable node name")
	fs.StringVar(&overrides.AdvertiseAddress, "advertise-address", "", "agent advertise address")
	fs.StringVar(&overrides.AgentAddr, "agent-addr", "", "agent listen address")
	fs.StringVar(&overrides.Labels, "labels", "", "comma-separated node labels")
	fs.StringVar(&overrides.ServerURL, "server-url", "", "control-plane URL")
	fs.StringVar(&overrides.BootstrapToken, "bootstrap-token", "", "agent bootstrap token")
	fs.StringVar(&overrides.DockerSocketPath, "docker-socket", "", "Docker socket path")
	fs.StringVar(&overrides.LogLevel, "log-level", "", "log level")
	fs.StringVar(&overrides.HeartbeatInterval, "heartbeat-interval", "", "heartbeat interval")
	fs.StringVar(&overrides.GracefulShutdownTTL, "shutdown-timeout", "", "graceful shutdown timeout")
	if err := fs.Parse(args); err != nil {
		return config.AgentConfig{}, false, err
	}
	cfg, err := config.LoadAgentWithFile(configPath, overrides)
	return cfg, printConfig, err
}

func run(ctx context.Context, logger *slog.Logger, cfg config.AgentConfig) error {
	runtime, err := orchdocker.NewEngineRuntimeFromEnv()
	if err != nil {
		return err
	}
	agentMetrics := metrics.NewAgent()
	instrumentedRuntime := orchdocker.WithMetrics(runtime, agentMetrics)
	runner := agent.NewRunner(cfg, nil, logger).WithRuntime(instrumentedRuntime).WithMetrics(agentMetrics)
	server := &http.Server{
		Addr:              cfg.AgentAddr,
		Handler:           agent.NewLogHandler(instrumentedRuntime, cfg.BootstrapToken, logger, agent.WithMetricsHandler(agentMetrics.Handler())),
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
