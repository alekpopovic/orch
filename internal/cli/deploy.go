package cli

import (
	"fmt"
	"os"
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
	Routes      []DeployRoute     `yaml:"routes"`
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
	Type               string `yaml:"type"`
	Path               string `yaml:"path"`
	Interval           string `yaml:"interval"`
	Timeout            string `yaml:"timeout"`
	HealthyThreshold   int    `yaml:"healthyThreshold"`
	UnhealthyThreshold int    `yaml:"unhealthyThreshold"`
}

type DeployRestart struct {
	Policy string `yaml:"policy"`
}

type DeployPlacement struct {
	Labels map[string]string `yaml:"labels"`
}

type DeployRoute struct {
	Host       string `yaml:"host"`
	PathPrefix string `yaml:"pathPrefix"`
	Port       int    `yaml:"port"`
	TLS        bool   `yaml:"tls"`
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
	for _, route := range deploy.Routes {
		spec.Routes = append(spec.Routes, types.Route{
			Host:       strings.TrimSpace(route.Host),
			PathPrefix: strings.TrimSpace(route.PathPrefix),
			Port:       route.Port,
			TLS:        route.TLS,
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
	spec, err = types.NormalizeServiceSpec(spec, types.DefaultResourceDefaults())
	if err != nil {
		return types.ServiceSpec{}, fmt.Errorf("invalid deploy file: %w", err)
	}
	return spec, nil
}

func (r DeployResources) toRequests() (types.Resources, error) {
	cpu, err := types.ParseCPU(r.CPU)
	if err != nil {
		return types.Resources{}, err
	}
	memory, err := types.ParseMemory(r.Memory)
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
	if h.HealthyThreshold > 0 {
		check.HealthyThreshold = h.HealthyThreshold
	}
	if h.UnhealthyThreshold > 0 {
		check.UnhealthyThreshold = h.UnhealthyThreshold
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
