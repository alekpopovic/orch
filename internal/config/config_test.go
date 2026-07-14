package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadServerDefaultsAreValid(t *testing.T) {
	t.Setenv("ORCH_SERVER_ADDR", "")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("ORCH_SHUTDOWN_TIMEOUT", "")
	t.Setenv("ORCH_NODE_HEARTBEAT_TIMEOUT", "")
	t.Setenv("ORCH_NODE_MONITOR_INTERVAL", "")

	cfg := LoadServer()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected default server config to be valid: %v", err)
	}
}

func TestLoadAgentDefaultsAreValid(t *testing.T) {
	t.Setenv("ORCH_NODE_ID", "")
	t.Setenv("ORCH_NODE_NAME", "")
	t.Setenv("ORCH_ADVERTISE_ADDRESS", "")
	t.Setenv("ORCH_NODE_LABELS", "")
	t.Setenv("ORCH_SERVER_URL", "")
	t.Setenv("ORCH_AGENT_REGISTRATION_TOKEN", "")
	t.Setenv("ORCH_BOOTSTRAP_TOKEN", "")
	t.Setenv("ORCH_DOCKER_SOCKET", "")
	t.Setenv("ORCH_AGENT_HEARTBEAT_INTERVAL", "")
	t.Setenv("ORCH_SHUTDOWN_TIMEOUT", "")

	cfg := LoadAgent()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected default agent config to be valid: %v", err)
	}
}

func TestLoadAgentPrefersRegistrationToken(t *testing.T) {
	t.Setenv("ORCH_AGENT_REGISTRATION_TOKEN", "registration")
	t.Setenv("ORCH_BOOTSTRAP_TOKEN", "legacy")

	cfg := LoadAgent()
	if cfg.BootstrapToken != "registration" {
		t.Fatalf("expected registration token to take precedence, got %q", cfg.BootstrapToken)
	}
}

func TestServerConfigPrecedence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.yaml")
	writeConfigFile(t, path, `
addr: :9000
log_level: warn
bootstrap_token: file-token
graceful_shutdown_ttl: 20s
`)
	t.Setenv("ORCH_SERVER_ADDR", ":9100")
	t.Setenv("ORCH_LOG_LEVEL", "debug")

	cfg, err := LoadServerWithFile(path, ServerOverrides{Addr: ":9200"})
	if err != nil {
		t.Fatalf("load server config: %v", err)
	}
	if cfg.Addr != ":9200" {
		t.Fatalf("expected flag override addr, got %q", cfg.Addr)
	}
	if cfg.LogLevel != "debug" {
		t.Fatalf("expected env log level, got %q", cfg.LogLevel)
	}
	if cfg.BootstrapToken != "file-token" {
		t.Fatalf("expected config file token, got %q", cfg.BootstrapToken)
	}
	if cfg.GracefulShutdownTTL != 20*time.Second {
		t.Fatalf("expected file duration, got %s", cfg.GracefulShutdownTTL)
	}
}

func TestAgentConfigPrecedence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.yaml")
	writeConfigFile(t, path, `
node_name: file-node
server_url: http://file.example
bootstrap_token: file-token
labels:
  role: file
heartbeat_interval: 20s
`)
	t.Setenv("ORCH_SERVER_URL", "http://env.example")
	t.Setenv("ORCH_NODE_LABELS", "role=env,zone=a")

	cfg, err := LoadAgentWithFile(path, AgentOverrides{ServerURL: "http://flag.example"})
	if err != nil {
		t.Fatalf("load agent config: %v", err)
	}
	if cfg.ServerURL != "http://flag.example" {
		t.Fatalf("expected flag server URL, got %q", cfg.ServerURL)
	}
	if cfg.Labels["role"] != "env" || cfg.Labels["zone"] != "a" {
		t.Fatalf("expected env labels, got %#v", cfg.Labels)
	}
	if cfg.NodeName != "file-node" {
		t.Fatalf("expected file node name, got %q", cfg.NodeName)
	}
	if cfg.HeartbeatInterval != 20*time.Second {
		t.Fatalf("expected file heartbeat interval, got %s", cfg.HeartbeatInterval)
	}
}

func TestCLIConfigPrecedence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cli.yaml")
	writeConfigFile(t, path, "server_url: http://file.example\ntoken: file-token\n")
	t.Setenv("ORCH_SERVER_URL", "http://env.example")

	cfg, err := LoadCLI(path, CLIOverrides{ServerURL: "http://flag.example"})
	if err != nil {
		t.Fatalf("load cli config: %v", err)
	}
	if cfg.ServerURL != "http://flag.example" {
		t.Fatalf("expected flag server URL, got %q", cfg.ServerURL)
	}
	if cfg.Token != "file-token" {
		t.Fatalf("expected file token, got %q", cfg.Token)
	}
}

func TestRedactedConfigDoesNotExposeSecrets(t *testing.T) {
	server := ServerConfig{
		DatabaseURL:    "postgres://user:pass@example/db",
		BootstrapToken: "bootstrap-secret",
		JWTSecret:      "jwt-secret",
		Users:          "admin:admin",
		SecretKey:      "secret-key",
	}
	redacted := server.Redacted()
	for _, key := range []string{"database_url", "bootstrap_token", "jwt_secret", "users", "secret_key"} {
		if redacted[key] != "[REDACTED]" {
			t.Fatalf("expected %s redacted, got %#v", key, redacted[key])
		}
	}
}

func writeConfigFile(t *testing.T, path string, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}
}
