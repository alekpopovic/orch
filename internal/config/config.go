package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type ServerConfig struct {
	Addr                string        `json:"addr" yaml:"addr"`
	DatabaseURL         string        `json:"database_url" yaml:"database_url"`
	LogLevel            string        `json:"log_level" yaml:"log_level"`
	BootstrapToken      string        `json:"bootstrap_token" yaml:"bootstrap_token"`
	JWTSecret           string        `json:"jwt_secret" yaml:"jwt_secret"`
	Users               string        `json:"users" yaml:"users"`
	SecretKey           string        `json:"secret_key" yaml:"secret_key"`
	GracefulShutdownTTL time.Duration `json:"graceful_shutdown_ttl" yaml:"graceful_shutdown_ttl"`
	HeartbeatTimeout    time.Duration `json:"heartbeat_timeout" yaml:"heartbeat_timeout"`
	NodeMonitorInterval time.Duration `json:"node_monitor_interval" yaml:"node_monitor_interval"`
}

type AgentConfig struct {
	NodeName            string            `json:"node_name" yaml:"node_name"`
	AdvertiseAddress    string            `json:"advertise_address" yaml:"advertise_address"`
	AgentAddr           string            `json:"agent_addr" yaml:"agent_addr"`
	Labels              map[string]string `json:"labels" yaml:"labels"`
	ServerURL           string            `json:"server_url" yaml:"server_url"`
	BootstrapToken      string            `json:"bootstrap_token" yaml:"bootstrap_token"`
	DockerSocketPath    string            `json:"docker_socket_path" yaml:"docker_socket_path"`
	LogLevel            string            `json:"log_level" yaml:"log_level"`
	HeartbeatInterval   time.Duration     `json:"heartbeat_interval" yaml:"heartbeat_interval"`
	GracefulShutdownTTL time.Duration     `json:"graceful_shutdown_ttl" yaml:"graceful_shutdown_ttl"`
}

type CLIConfig struct {
	ServerURL string `yaml:"server_url" json:"server_url"`
	Token     string `yaml:"token" json:"token"`
}

type ServerOverrides struct {
	Addr                string
	DatabaseURL         string
	LogLevel            string
	BootstrapToken      string
	JWTSecret           string
	Users               string
	SecretKey           string
	GracefulShutdownTTL string
	HeartbeatTimeout    string
	NodeMonitorInterval string
}

type AgentOverrides struct {
	NodeName            string
	AdvertiseAddress    string
	AgentAddr           string
	Labels              string
	ServerURL           string
	BootstrapToken      string
	DockerSocketPath    string
	LogLevel            string
	HeartbeatInterval   string
	GracefulShutdownTTL string
}

type CLIOverrides struct {
	ServerURL string
	Token     string
}

func LoadServer() ServerConfig {
	cfg, _ := LoadServerWithFile("", ServerOverrides{})
	return cfg
}

func LoadAgent() AgentConfig {
	cfg, _ := LoadAgentWithFile("", AgentOverrides{})
	return cfg
}

func LoadServerWithFile(path string, overrides ServerOverrides) (ServerConfig, error) {
	cfg := defaultServer()
	if err := applyServerFile(&cfg, path); err != nil {
		return ServerConfig{}, err
	}
	if err := applyServerEnv(&cfg); err != nil {
		return ServerConfig{}, err
	}
	if err := applyServerOverrides(&cfg, overrides); err != nil {
		return ServerConfig{}, err
	}
	return cfg, cfg.Validate()
}

func LoadAgentWithFile(path string, overrides AgentOverrides) (AgentConfig, error) {
	cfg := defaultAgent()
	if err := applyAgentFile(&cfg, path); err != nil {
		return AgentConfig{}, err
	}
	if err := applyAgentEnv(&cfg); err != nil {
		return AgentConfig{}, err
	}
	if err := applyAgentOverrides(&cfg, overrides); err != nil {
		return AgentConfig{}, err
	}
	return cfg, cfg.Validate()
}

func LoadCLI(path string, overrides CLIOverrides) (CLIConfig, error) {
	cfg := CLIConfig{}
	if strings.TrimSpace(path) != "" {
		file, err := readOptionalYAMLFile[CLIConfig](path)
		if err != nil {
			return CLIConfig{}, err
		}
		cfg = file
	}
	if value := strings.TrimSpace(os.Getenv("ORCH_SERVER_URL")); value != "" {
		cfg.ServerURL = value
	}
	if value := strings.TrimSpace(os.Getenv("ORCH_TOKEN")); value != "" {
		cfg.Token = value
	}
	if strings.TrimSpace(overrides.ServerURL) != "" {
		cfg.ServerURL = strings.TrimSpace(overrides.ServerURL)
	}
	if strings.TrimSpace(overrides.Token) != "" {
		cfg.Token = strings.TrimSpace(overrides.Token)
	}
	return cfg, nil
}

func defaultServer() ServerConfig {
	return ServerConfig{
		Addr:                ":8080",
		DatabaseURL:         "postgres://orch:orch@localhost:5432/orch?sslmode=disable",
		LogLevel:            "info",
		BootstrapToken:      "dev-bootstrap-token",
		SecretKey:           "dev-secret-key-change-me",
		GracefulShutdownTTL: 10 * time.Second,
		HeartbeatTimeout:    30 * time.Second,
		NodeMonitorInterval: 5 * time.Second,
	}
}

func defaultAgent() AgentConfig {
	return AgentConfig{
		NodeName:            "local-node",
		AdvertiseAddress:    "http://127.0.0.1:8081",
		AgentAddr:           ":8081",
		Labels:              map[string]string{},
		ServerURL:           "http://localhost:8080",
		BootstrapToken:      "dev-bootstrap-token",
		DockerSocketPath:    "/var/run/docker.sock",
		LogLevel:            "info",
		HeartbeatInterval:   5 * time.Second,
		GracefulShutdownTTL: 10 * time.Second,
	}
}

type serverFileConfig struct {
	Addr                string `yaml:"addr"`
	DatabaseURL         string `yaml:"database_url"`
	LogLevel            string `yaml:"log_level"`
	BootstrapToken      string `yaml:"bootstrap_token"`
	JWTSecret           string `yaml:"jwt_secret"`
	Users               string `yaml:"users"`
	SecretKey           string `yaml:"secret_key"`
	GracefulShutdownTTL string `yaml:"graceful_shutdown_ttl"`
	HeartbeatTimeout    string `yaml:"heartbeat_timeout"`
	NodeMonitorInterval string `yaml:"node_monitor_interval"`
}

type agentFileConfig struct {
	NodeName            string            `yaml:"node_name"`
	AdvertiseAddress    string            `yaml:"advertise_address"`
	AgentAddr           string            `yaml:"agent_addr"`
	Labels              map[string]string `yaml:"labels"`
	ServerURL           string            `yaml:"server_url"`
	BootstrapToken      string            `yaml:"bootstrap_token"`
	DockerSocketPath    string            `yaml:"docker_socket_path"`
	LogLevel            string            `yaml:"log_level"`
	HeartbeatInterval   string            `yaml:"heartbeat_interval"`
	GracefulShutdownTTL string            `yaml:"graceful_shutdown_ttl"`
}

func applyServerFile(cfg *ServerConfig, path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	file, err := readYAMLFile[serverFileConfig](path)
	if err != nil {
		return err
	}
	applyString(&cfg.Addr, file.Addr)
	applyString(&cfg.DatabaseURL, file.DatabaseURL)
	applyString(&cfg.LogLevel, file.LogLevel)
	applyString(&cfg.BootstrapToken, file.BootstrapToken)
	applyString(&cfg.JWTSecret, file.JWTSecret)
	applyString(&cfg.Users, file.Users)
	applyString(&cfg.SecretKey, file.SecretKey)
	if err := applyDuration(&cfg.GracefulShutdownTTL, file.GracefulShutdownTTL); err != nil {
		return fmt.Errorf("graceful_shutdown_ttl: %w", err)
	}
	if err := applyDuration(&cfg.HeartbeatTimeout, file.HeartbeatTimeout); err != nil {
		return fmt.Errorf("heartbeat_timeout: %w", err)
	}
	if err := applyDuration(&cfg.NodeMonitorInterval, file.NodeMonitorInterval); err != nil {
		return fmt.Errorf("node_monitor_interval: %w", err)
	}
	return nil
}

func applyAgentFile(cfg *AgentConfig, path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	file, err := readYAMLFile[agentFileConfig](path)
	if err != nil {
		return err
	}
	applyString(&cfg.NodeName, file.NodeName)
	applyString(&cfg.AdvertiseAddress, file.AdvertiseAddress)
	applyString(&cfg.AgentAddr, file.AgentAddr)
	if file.Labels != nil {
		cfg.Labels = cloneStringMap(file.Labels)
	}
	applyString(&cfg.ServerURL, file.ServerURL)
	applyString(&cfg.BootstrapToken, file.BootstrapToken)
	applyString(&cfg.DockerSocketPath, file.DockerSocketPath)
	applyString(&cfg.LogLevel, file.LogLevel)
	if err := applyDuration(&cfg.HeartbeatInterval, file.HeartbeatInterval); err != nil {
		return fmt.Errorf("heartbeat_interval: %w", err)
	}
	if err := applyDuration(&cfg.GracefulShutdownTTL, file.GracefulShutdownTTL); err != nil {
		return fmt.Errorf("graceful_shutdown_ttl: %w", err)
	}
	return nil
}

func applyServerEnv(cfg *ServerConfig) error {
	applyString(&cfg.Addr, os.Getenv("ORCH_SERVER_ADDR"))
	applyString(&cfg.DatabaseURL, os.Getenv("DATABASE_URL"))
	applyString(&cfg.LogLevel, os.Getenv("ORCH_LOG_LEVEL"))
	if value := os.Getenv("ORCH_AGENT_REGISTRATION_TOKEN"); value != "" {
		cfg.BootstrapToken = value
	} else {
		applyString(&cfg.BootstrapToken, os.Getenv("ORCH_BOOTSTRAP_TOKEN"))
	}
	applyString(&cfg.JWTSecret, os.Getenv("ORCH_JWT_SECRET"))
	applyString(&cfg.Users, os.Getenv("ORCH_USERS"))
	applyString(&cfg.SecretKey, os.Getenv("ORCH_SECRET_KEY"))
	if err := applyDuration(&cfg.GracefulShutdownTTL, os.Getenv("ORCH_SHUTDOWN_TIMEOUT")); err != nil {
		return fmt.Errorf("ORCH_SHUTDOWN_TIMEOUT: %w", err)
	}
	if err := applyDuration(&cfg.HeartbeatTimeout, os.Getenv("ORCH_NODE_HEARTBEAT_TIMEOUT")); err != nil {
		return fmt.Errorf("ORCH_NODE_HEARTBEAT_TIMEOUT: %w", err)
	}
	if err := applyDuration(&cfg.NodeMonitorInterval, os.Getenv("ORCH_NODE_MONITOR_INTERVAL")); err != nil {
		return fmt.Errorf("ORCH_NODE_MONITOR_INTERVAL: %w", err)
	}
	return nil
}

func applyAgentEnv(cfg *AgentConfig) error {
	if value := os.Getenv("ORCH_NODE_NAME"); value != "" {
		cfg.NodeName = value
	} else {
		applyString(&cfg.NodeName, os.Getenv("ORCH_NODE_ID"))
	}
	applyString(&cfg.AdvertiseAddress, os.Getenv("ORCH_ADVERTISE_ADDRESS"))
	applyString(&cfg.AgentAddr, os.Getenv("ORCH_AGENT_ADDR"))
	if labels := labelsFromString(os.Getenv("ORCH_NODE_LABELS")); labels != nil {
		cfg.Labels = labels
	}
	applyString(&cfg.ServerURL, os.Getenv("ORCH_SERVER_URL"))
	if value := os.Getenv("ORCH_AGENT_REGISTRATION_TOKEN"); value != "" {
		cfg.BootstrapToken = value
	} else {
		applyString(&cfg.BootstrapToken, os.Getenv("ORCH_BOOTSTRAP_TOKEN"))
	}
	applyString(&cfg.DockerSocketPath, os.Getenv("ORCH_DOCKER_SOCKET"))
	applyString(&cfg.LogLevel, os.Getenv("ORCH_LOG_LEVEL"))
	if err := applyDuration(&cfg.HeartbeatInterval, os.Getenv("ORCH_AGENT_HEARTBEAT_INTERVAL")); err != nil {
		return fmt.Errorf("ORCH_AGENT_HEARTBEAT_INTERVAL: %w", err)
	}
	if err := applyDuration(&cfg.GracefulShutdownTTL, os.Getenv("ORCH_SHUTDOWN_TIMEOUT")); err != nil {
		return fmt.Errorf("ORCH_SHUTDOWN_TIMEOUT: %w", err)
	}
	return nil
}

func applyServerOverrides(cfg *ServerConfig, overrides ServerOverrides) error {
	applyString(&cfg.Addr, overrides.Addr)
	applyString(&cfg.DatabaseURL, overrides.DatabaseURL)
	applyString(&cfg.LogLevel, overrides.LogLevel)
	applyString(&cfg.BootstrapToken, overrides.BootstrapToken)
	applyString(&cfg.JWTSecret, overrides.JWTSecret)
	applyString(&cfg.Users, overrides.Users)
	applyString(&cfg.SecretKey, overrides.SecretKey)
	if err := applyDuration(&cfg.GracefulShutdownTTL, overrides.GracefulShutdownTTL); err != nil {
		return fmt.Errorf("shutdown timeout override: %w", err)
	}
	if err := applyDuration(&cfg.HeartbeatTimeout, overrides.HeartbeatTimeout); err != nil {
		return fmt.Errorf("heartbeat timeout override: %w", err)
	}
	if err := applyDuration(&cfg.NodeMonitorInterval, overrides.NodeMonitorInterval); err != nil {
		return fmt.Errorf("node monitor interval override: %w", err)
	}
	return nil
}

func applyAgentOverrides(cfg *AgentConfig, overrides AgentOverrides) error {
	applyString(&cfg.NodeName, overrides.NodeName)
	applyString(&cfg.AdvertiseAddress, overrides.AdvertiseAddress)
	applyString(&cfg.AgentAddr, overrides.AgentAddr)
	if labels := labelsFromString(overrides.Labels); labels != nil {
		cfg.Labels = labels
	}
	applyString(&cfg.ServerURL, overrides.ServerURL)
	applyString(&cfg.BootstrapToken, overrides.BootstrapToken)
	applyString(&cfg.DockerSocketPath, overrides.DockerSocketPath)
	applyString(&cfg.LogLevel, overrides.LogLevel)
	if err := applyDuration(&cfg.HeartbeatInterval, overrides.HeartbeatInterval); err != nil {
		return fmt.Errorf("heartbeat interval override: %w", err)
	}
	if err := applyDuration(&cfg.GracefulShutdownTTL, overrides.GracefulShutdownTTL); err != nil {
		return fmt.Errorf("shutdown timeout override: %w", err)
	}
	return nil
}

func applyString(target *string, value string) {
	if strings.TrimSpace(value) != "" {
		*target = strings.TrimSpace(value)
	}
}

func applyDuration(target *time.Duration, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	duration, err := parseDuration(value)
	if err != nil {
		return err
	}
	*target = duration
	return nil
}

func readYAMLFile[T any](path string) (T, error) {
	var value T
	data, err := os.ReadFile(path)
	if err != nil {
		return value, fmt.Errorf("read config file %q: %w", path, err)
	}
	if err := yaml.Unmarshal(data, &value); err != nil {
		return value, fmt.Errorf("parse config file %q: %w", path, err)
	}
	return value, nil
}

func readOptionalYAMLFile[T any](path string) (T, error) {
	var value T
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return value, nil
		}
		return value, fmt.Errorf("read config file %q: %w", path, err)
	}
	if err := yaml.Unmarshal(data, &value); err != nil {
		return value, fmt.Errorf("parse config file %q: %w", path, err)
	}
	return value, nil
}

func labelsFromString(raw string) map[string]string {
	labels := map[string]string{}
	if strings.TrimSpace(raw) == "" {
		return nil
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

func parseDuration(value string) (time.Duration, error) {
	duration, err := time.ParseDuration(value)
	if err == nil {
		return duration, nil
	}
	seconds, err := strconv.Atoi(value)
	if err != nil {
		return 0, err
	}
	return time.Duration(seconds) * time.Second, nil
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return map[string]string{}
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func (cfg ServerConfig) Redacted() map[string]any {
	return map[string]any{
		"addr":                  cfg.Addr,
		"database_url":          redact(cfg.DatabaseURL),
		"log_level":             cfg.LogLevel,
		"bootstrap_token":       redact(cfg.BootstrapToken),
		"jwt_secret":            redact(cfg.JWTSecret),
		"users":                 redact(cfg.Users),
		"secret_key":            redact(cfg.SecretKey),
		"graceful_shutdown_ttl": cfg.GracefulShutdownTTL.String(),
		"heartbeat_timeout":     cfg.HeartbeatTimeout.String(),
		"node_monitor_interval": cfg.NodeMonitorInterval.String(),
	}
}

func (cfg AgentConfig) Redacted() map[string]any {
	return map[string]any{
		"node_name":             cfg.NodeName,
		"advertise_address":     cfg.AdvertiseAddress,
		"agent_addr":            cfg.AgentAddr,
		"labels":                cloneStringMap(cfg.Labels),
		"server_url":            cfg.ServerURL,
		"bootstrap_token":       redact(cfg.BootstrapToken),
		"docker_socket_path":    cfg.DockerSocketPath,
		"log_level":             cfg.LogLevel,
		"heartbeat_interval":    cfg.HeartbeatInterval.String(),
		"graceful_shutdown_ttl": cfg.GracefulShutdownTTL.String(),
	}
}

func (cfg CLIConfig) Redacted() map[string]any {
	return map[string]any{
		"server_url": cfg.ServerURL,
		"token":      redact(cfg.Token),
	}
}

func redact(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return "[REDACTED]"
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
	if cfg.HeartbeatTimeout <= 0 {
		return fmt.Errorf("node heartbeat timeout must be positive")
	}
	if cfg.NodeMonitorInterval <= 0 {
		return fmt.Errorf("node monitor interval must be positive")
	}
	if strings.TrimSpace(cfg.SecretKey) == "" {
		return fmt.Errorf("secret encryption key is required")
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
