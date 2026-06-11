package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type ServerConfig struct {
	Addr                string
	DatabaseURL         string
	LogLevel            string
	BootstrapToken      string
	JWTSecret           string
	Users               string
	GracefulShutdownTTL time.Duration
}

type AgentConfig struct {
	NodeName            string
	AdvertiseAddress    string
	AgentAddr           string
	Labels              map[string]string
	ServerURL           string
	BootstrapToken      string
	DockerSocketPath    string
	LogLevel            string
	HeartbeatInterval   time.Duration
	GracefulShutdownTTL time.Duration
}

func LoadServer() ServerConfig {
	return ServerConfig{
		Addr:                getenv("ORCH_SERVER_ADDR", ":8080"),
		DatabaseURL:         getenv("DATABASE_URL", "postgres://orch:orch@localhost:5432/orch?sslmode=disable"),
		LogLevel:            getenv("ORCH_LOG_LEVEL", "info"),
		BootstrapToken:      getenv("ORCH_AGENT_REGISTRATION_TOKEN", getenv("ORCH_BOOTSTRAP_TOKEN", "dev-bootstrap-token")),
		JWTSecret:           getenv("ORCH_JWT_SECRET", ""),
		Users:               getenv("ORCH_USERS", ""),
		GracefulShutdownTTL: durationFromEnv("ORCH_SHUTDOWN_TIMEOUT", 10*time.Second),
	}
}

func LoadAgent() AgentConfig {
	return AgentConfig{
		NodeName:            getenv("ORCH_NODE_NAME", getenv("ORCH_NODE_ID", "local-node")),
		AdvertiseAddress:    getenv("ORCH_ADVERTISE_ADDRESS", "http://127.0.0.1:8081"),
		AgentAddr:           getenv("ORCH_AGENT_ADDR", ":8081"),
		Labels:              labelsFromEnv("ORCH_NODE_LABELS"),
		ServerURL:           getenv("ORCH_SERVER_URL", "http://localhost:8080"),
		BootstrapToken:      getenv("ORCH_AGENT_REGISTRATION_TOKEN", getenv("ORCH_BOOTSTRAP_TOKEN", "dev-bootstrap-token")),
		DockerSocketPath:    getenv("ORCH_DOCKER_SOCKET", "/var/run/docker.sock"),
		LogLevel:            getenv("ORCH_LOG_LEVEL", "info"),
		HeartbeatInterval:   durationFromEnv("ORCH_AGENT_HEARTBEAT_INTERVAL", 5*time.Second),
		GracefulShutdownTTL: durationFromEnv("ORCH_SHUTDOWN_TIMEOUT", 10*time.Second),
	}
}

func getenv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func labelsFromEnv(key string) map[string]string {
	raw := os.Getenv(key)
	labels := map[string]string{}
	if raw == "" {
		return labels
	}
	for _, part := range strings.Split(raw, ",") {
		pair := strings.SplitN(part, "=", 2)
		if len(pair) != 2 {
			continue
		}
		labelKey := strings.TrimSpace(pair[0])
		labelValue := strings.TrimSpace(pair[1])
		if labelKey != "" {
			labels[labelKey] = labelValue
		}
	}
	return labels
}

func durationFromEnv(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	duration, err := time.ParseDuration(value)
	if err == nil {
		return duration
	}

	seconds, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return time.Duration(seconds) * time.Second
}

func (cfg ServerConfig) Validate() error {
	if cfg.Addr == "" {
		return fmt.Errorf("server address is required")
	}
	if cfg.DatabaseURL == "" {
		return fmt.Errorf("database URL is required")
	}
	if cfg.GracefulShutdownTTL <= 0 {
		return fmt.Errorf("shutdown timeout must be positive")
	}
	return nil
}

func (cfg AgentConfig) Validate() error {
	if cfg.NodeName == "" {
		return fmt.Errorf("node name is required")
	}
	if cfg.AdvertiseAddress == "" {
		return fmt.Errorf("advertise address is required")
	}
	if cfg.AgentAddr == "" {
		return fmt.Errorf("agent address is required")
	}
	if cfg.ServerURL == "" {
		return fmt.Errorf("server URL is required")
	}
	if cfg.BootstrapToken == "" {
		return fmt.Errorf("agent registration token is required")
	}
	if cfg.DockerSocketPath == "" {
		return fmt.Errorf("Docker socket path is required")
	}
	if cfg.HeartbeatInterval <= 0 {
		return fmt.Errorf("heartbeat interval must be positive")
	}
	if cfg.GracefulShutdownTTL <= 0 {
		return fmt.Errorf("shutdown timeout must be positive")
	}
	return nil
}
