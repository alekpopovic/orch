package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/alekpopovic/orch/internal/config"
)

func TestServerStartsAndStopsCleanly(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- run(ctx, slog.Default(), config.ServerConfig{
			Addr:                "127.0.0.1:0",
			DatabaseURL:         "postgres://orch:orch@localhost:5432/orch?sslmode=disable",
			BootstrapToken:      "test-registration-token",
			SecretKey:           "test-secret-key",
			GracefulShutdownTTL: time.Second,
			HeartbeatTimeout:    time.Second,
			NodeMonitorInterval: 10 * time.Millisecond,
		})
	}()

	time.Sleep(25 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("server stopped with error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("server did not stop after cancellation")
	}
}

func TestServerConfigPrintModeParsesFlags(t *testing.T) {
	cfg, printConfig, err := loadConfig([]string{"config", "print", "--addr", "127.0.0.1:9090", "--secret-key", "super-secret"})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !printConfig {
		t.Fatal("expected config print mode")
	}
	if cfg.Addr != "127.0.0.1:9090" {
		t.Fatalf("expected flag addr, got %q", cfg.Addr)
	}
	redacted := cfg.Redacted()
	if got := redacted["secret_key"]; got != "[REDACTED]" || strings.Contains(fmt.Sprint(redacted), "super-secret") {
		t.Fatalf("expected redacted secret, got %#v", redacted)
	}
}
