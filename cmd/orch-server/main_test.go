package main

import (
	"context"
	"log/slog"
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
