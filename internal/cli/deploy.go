package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/alekpopovic/orch/pkg/types"
	"gopkg.in/yaml.v3"
)

type DeployFile struct {
	Name        string            `yaml:"name"`
	Image       string            `yaml:"image"`
	Replicas    int               `yaml:"replicas"`
	Ports       []DeployPort      `yaml:"ports"`
	Env         map[string]string `yaml:"env"`
	Resources   DeployResources   `yaml:"resources"`
	Healthcheck DeployHealthcheck `yaml:"healthcheck"`
	Restart     DeployRestart     `yaml:"restart"`
	Placement   DeployPlacement   `yaml:"placement"`
}

type DeployPort struct {
	Container int `yaml:"container"`
	Public    int `yaml:"public"`
}

type DeployResources struct {
	CPU    string `yaml:"cpu"`
	Memory string `yaml:"memory"`
}

type DeployHealthcheck struct {
	Type     string `yaml:"type"`
	Path     string `yaml:"path"`
	Interval string `yaml:"interval"`
	Timeout  string `yaml:"timeout"`
}

type DeployRestart struct {
	Policy string `yaml:"policy"`
}

type DeployPlacement struct {
	Labels map[string]string `yaml:"labels"`
}

func ParseDeployFile(path string) (types.ServiceSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return types.ServiceSpec{}, fmt.Errorf("read deploy file %q: %w", path, err)
	}
	return ParseDeploy(data)
}

func ParseDeploy(data []byte) (types.ServiceSpec, error) {
	var deploy DeployFile
	if err := yaml.Unmarshal(data, &deploy); err != nil {
		return types.ServiceSpec{}, fmt.Errorf("parse deploy YAML: %w", err)
	}

	requests, err := deploy.Resources.toRequests()
	if err != nil {
		return types.ServiceSpec{}, err
	}

	spec := types.ServiceSpec{
		Name:     strings.TrimSpace(deploy.Name),
		Image:    strings.TrimSpace(deploy.Image),
		Replicas: deploy.Replicas,
		Env:      deploy.Env,
		ResourceRequirements: types.ResourceRequirements{
			Requests: requests,
			Limits:   requests,
		},
		RestartPolicy:        restartPolicy(deploy.Restart.Policy),
		PlacementConstraints: placementConstraints(deploy.Placement.Labels),
	}

	for _, port := range deploy.Ports {
		spec.Ports = append(spec.Ports, types.Port{
			Protocol:      types.PortTCP,
			ContainerPort: port.Container,
			PublishedPort: port.Public,
		})
	}

	if strings.TrimSpace(deploy.Healthcheck.Type) != "" {
		check, err := deploy.Healthcheck.toDomain()
		if err != nil {
			return types.ServiceSpec{}, err
		}
		if check.Port == 0 && len(spec.Ports) > 0 {
			check.Port = spec.Ports[0].ContainerPort
		}
		spec.Healthcheck = &check
	}

	if err := spec.Validate(); err != nil {
		return types.ServiceSpec{}, fmt.Errorf("invalid deploy file: %w", err)
	}
	return spec, nil
}

func (r DeployResources) toRequests() (types.Resources, error) {
	cpu, err := parseCPU(r.CPU)
	if err != nil {
		return types.Resources{}, err
	}
	memory, err := parseMemory(r.Memory)
	if err != nil {
		return types.Resources{}, err
	}
	return types.Resources{CPU: cpu, Memory: memory}, nil
}

func (h DeployHealthcheck) toDomain() (types.Healthcheck, error) {
	interval, err := parseOptionalDuration(h.Interval)
	if err != nil {
		return types.Healthcheck{}, fmt.Errorf("invalid healthcheck interval: %w", err)
	}
	timeout, err := parseOptionalDuration(h.Timeout)
	if err != nil {
		return types.Healthcheck{}, fmt.Errorf("invalid healthcheck timeout: %w", err)
	}

	check := types.Healthcheck{
		Type:               types.HealthcheckType(strings.TrimSpace(h.Type)),
		Path:               strings.TrimSpace(h.Path),
		Interval:           interval,
		Timeout:            timeout,
		HealthyThreshold:   1,
		UnhealthyThreshold: 3,
	}
	return check, nil
}

func restartPolicy(policy string) types.RestartPolicy {
	switch strings.TrimSpace(policy) {
	case "":
		return types.RestartPolicy{}
	case "always":
		return types.RestartPolicy{Condition: types.RestartAlways}
	case "never":
		return types.RestartPolicy{Condition: types.RestartNever}
	case "on_failure", "on-failure":
		return types.RestartPolicy{Condition: types.RestartOnFailure}
	default:
		return types.RestartPolicy{Condition: types.RestartCondition(policy)}
	}
}

func placementConstraints(labels map[string]string) []types.PlacementConstraint {
	constraints := make([]types.PlacementConstraint, 0, len(labels))
	for key, value := range labels {
		constraints = append(constraints, types.PlacementConstraint{
			Key:      key,
			Operator: types.ConstraintEquals,
			Value:    value,
		})
	}
	return constraints
}

func parseOptionalDuration(value string) (time.Duration, error) {
	if strings.TrimSpace(value) == "" {
		return 0, nil
	}
	return time.ParseDuration(value)
}

func parseCPU(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	if strings.HasSuffix(value, "m") {
		raw := strings.TrimSuffix(value, "m")
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid CPU value %q", value)
		}
		return parsed, nil
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid CPU value %q", value)
	}
	return int64(parsed * 1000), nil
}

func parseMemory(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}

	units := []struct {
		suffix string
		scale  int64
	}{
		{"Gi", 1024 * 1024 * 1024},
		{"Mi", 1024 * 1024},
		{"Ki", 1024},
		{"G", 1000 * 1000 * 1000},
		{"M", 1000 * 1000},
		{"K", 1000},
	}
	for _, unit := range units {
		if strings.HasSuffix(value, unit.suffix) {
			raw := strings.TrimSuffix(value, unit.suffix)
			parsed, err := strconv.ParseInt(raw, 10, 64)
			if err != nil {
				return 0, fmt.Errorf("invalid memory value %q", value)
			}
			return parsed * unit.scale, nil
		}
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid memory value %q", value)
	}
	return parsed, nil
}
