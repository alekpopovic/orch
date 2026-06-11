package config

import "testing"

func TestLoadServerDefaultsAreValid(t *testing.T) {
	t.Setenv("ORCH_SERVER_ADDR", "")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("ORCH_SHUTDOWN_TIMEOUT", "")

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
