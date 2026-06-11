package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type ServerConfig struct {
	Addr                string
	DatabaseURL         string
	LogLevel            string
	GracefulShutdownTTL time.Duration
}

type AgentConfig struct {
	NodeID              string
	ServerURL           string
	LogLevel            string
	HeartbeatInterval   time.Duration
	GracefulShutdownTTL time.Duration
}

func LoadServer() ServerConfig {
	return ServerConfig{
		Addr:                getenv("ORCH_SERVER_ADDR", ":8080"),
		DatabaseURL:         getenv("DATABASE_URL", "postgres://orch:orch@localhost:5432/orch?sslmode=disable"),
		LogLevel:            getenv("ORCH_LOG_LEVEL", "info"),
		GracefulShutdownTTL: durationFromEnv("ORCH_SHUTDOWN_TIMEOUT", 10*time.Second),
	}
}

func LoadAgent() AgentConfig {
	return AgentConfig{
		NodeID:              getenv("ORCH_NODE_ID", "local-node"),
		ServerURL:           getenv("ORCH_SERVER_URL", "http://localhost:8080"),
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
	if cfg.NodeID == "" {
		return fmt.Errorf("node ID is required")
	}
	if cfg.ServerURL == "" {
		return fmt.Errorf("server URL is required")
	}
	if cfg.HeartbeatInterval <= 0 {
		return fmt.Errorf("heartbeat interval must be positive")
	}
	if cfg.GracefulShutdownTTL <= 0 {
		return fmt.Errorf("shutdown timeout must be positive")
	}
	return nil
}
