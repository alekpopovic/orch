package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestAgentConfigPrintModeParsesFlags(t *testing.T) {
	cfg, printConfig, err := loadConfig([]string{
		"config",
		"print",
		"--node-name", "worker-a",
		"--server-url", "http://server.example",
		"--bootstrap-token", "super-secret",
	})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !printConfig {
		t.Fatal("expected config print mode")
	}
	if cfg.NodeName != "worker-a" || cfg.ServerURL != "http://server.example" {
		t.Fatalf("unexpected config %#v", cfg)
	}
	redacted := cfg.Redacted()
	if got := redacted["bootstrap_token"]; got != "[REDACTED]" || strings.Contains(fmt.Sprint(redacted), "super-secret") {
		t.Fatalf("expected redacted token, got %#v", redacted)
	}
}
