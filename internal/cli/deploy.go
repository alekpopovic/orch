package cli

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/alekpopovic/orch/pkg/types"
	"gopkg.in/yaml.v3"
)

type DeployFile struct {
	Name            string                `yaml:"name"`
	Image           string                `yaml:"image"`
	ImagePullSecret string                `yaml:"imagePullSecret"`
	Stateful        bool                  `yaml:"stateful"`
	Replicas        int                   `yaml:"replicas"`
	Ports           []DeployPort          `yaml:"ports"`
	Env             DeployEnv             `yaml:"env"`
	Resources       DeployResources       `yaml:"resources"`
	SecurityContext DeploySecurityContext `yaml:"securityContext"`
	Healthcheck     DeployHealthcheck     `yaml:"healthcheck"`
	Restart         DeployRestart         `yaml:"restart"`
	Placement       DeployPlacement       `yaml:"placement"`
	Routes          []DeployRoute         `yaml:"routes"`
}

type DeployPort struct {
	Container int `yaml:"container"`
	Public    int `yaml:"public"`
}

type DeployEnv map[string]DeployEnvValue

type DeployEnvValue struct {
	Value     string
	SecretRef string
}

func (value *DeployEnvValue) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		value.Value = node.Value
		return nil
	case yaml.MappingNode:
		var raw struct {
			Value     string `yaml:"value"`
			SecretRef string `yaml:"secretRef"`
		}
		if err := node.Decode(&raw); err != nil {
			return err
		}
		value.Value = raw.Value
		value.SecretRef = raw.SecretRef
		return nil
	default:
		return fmt.Errorf("env value must be a scalar or mapping")
	}
}

type DeployResources struct {
	CPU    string `yaml:"cpu"`
	Memory string `yaml:"memory"`
}

type DeploySecurityContext struct {
	User                   string                `yaml:"user"`
	ReadOnlyRootFilesystem bool                  `yaml:"readOnlyRootFilesystem"`
	Privileged             bool                  `yaml:"privileged"`
	CapAdd                 []string              `yaml:"capAdd"`
	CapDrop                []string              `yaml:"capDrop"`
	HostNetwork            bool                  `yaml:"hostNetwork"`
	HostPID                bool                  `yaml:"hostPID"`
	HostPathMounts         []DeployHostPathMount `yaml:"hostPathMounts"`
}

type DeployHostPathMount struct {
	HostPath      string `yaml:"hostPath"`
	ContainerPath string `yaml:"containerPath"`
	ReadOnly      bool   `yaml:"readOnly"`
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
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&deploy); err != nil {
		return types.ServiceSpec{}, fmt.Errorf("parse deploy YAML: %w", err)
	}
	if err := deploy.Validate(); err != nil {
		return types.ServiceSpec{}, fmt.Errorf("invalid deploy file: %w", err)
	}

	requests, err := deploy.Resources.toRequests()
	if err != nil {
		return types.ServiceSpec{}, err
	}

	spec := types.ServiceSpec{
		Name:            strings.TrimSpace(deploy.Name),
		Image:           strings.TrimSpace(deploy.Image),
		ImagePullSecret: strings.TrimSpace(deploy.ImagePullSecret),
		Stateful:        deploy.Stateful,
		Replicas:        deploy.Replicas,
		ResourceRequirements: types.ResourceRequirements{
			Requests: requests,
			Limits:   requests,
		},
		SecurityContext:      deploy.SecurityContext.toDomain(),
		RestartPolicy:        restartPolicy(deploy.Restart.Policy),
		PlacementConstraints: placementConstraints(deploy.Placement.Labels),
	}
	spec.Env, spec.SecretRefs = deploy.Env.toDomain()

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

func (context DeploySecurityContext) toDomain() types.SecurityContext {
	mounts := make([]types.HostPathMount, 0, len(context.HostPathMounts))
	for _, mount := range context.HostPathMounts {
		mounts = append(mounts, types.HostPathMount{
			HostPath:      strings.TrimSpace(mount.HostPath),
			ContainerPath: strings.TrimSpace(mount.ContainerPath),
			ReadOnly:      mount.ReadOnly,
		})
	}
	return types.SecurityContext{
		User:                   strings.TrimSpace(context.User),
		ReadOnlyRootFilesystem: context.ReadOnlyRootFilesystem,
		Privileged:             context.Privileged,
		CapAdd:                 append([]string(nil), context.CapAdd...),
		CapDrop:                append([]string(nil), context.CapDrop...),
		HostNetwork:            context.HostNetwork,
		HostPID:                context.HostPID,
		HostPathMounts:         mounts,
	}
}

func (deploy DeployFile) Validate() error {
	if strings.TrimSpace(deploy.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if strings.TrimSpace(deploy.Image) == "" {
		return fmt.Errorf("image is required")
	}
	if deploy.Replicas < 0 {
		return fmt.Errorf("replicas cannot be negative")
	}
	for i, port := range deploy.Ports {
		if port.Container < 1 || port.Container > 65535 {
			return fmt.Errorf("ports[%d].container must be between 1 and 65535", i)
		}
		if port.Public < 0 || port.Public > 65535 {
			return fmt.Errorf("ports[%d].public must be between 0 and 65535", i)
		}
	}
	for key, value := range deploy.Env {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("env keys must not be empty")
		}
		if strings.TrimSpace(value.SecretRef) != "" && strings.TrimSpace(value.Value) != "" {
			return fmt.Errorf("env %q cannot set both value and secretRef", key)
		}
	}
	for key := range deploy.Placement.Labels {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("placement label keys must not be empty")
		}
	}
	for i, route := range deploy.Routes {
		if strings.TrimSpace(route.Host) == "" {
			return fmt.Errorf("routes[%d].host is required", i)
		}
		if strings.TrimSpace(route.PathPrefix) == "" {
			return fmt.Errorf("routes[%d].pathPrefix is required", i)
		}
		if !strings.HasPrefix(strings.TrimSpace(route.PathPrefix), "/") {
			return fmt.Errorf("routes[%d].pathPrefix must start with /", i)
		}
		if route.Port < 1 || route.Port > 65535 {
			return fmt.Errorf("routes[%d].port must be between 1 and 65535", i)
		}
	}
	return nil
}

func (env DeployEnv) toDomain() (map[string]string, []types.SecretRef) {
	if len(env) == 0 {
		return nil, nil
	}
	literals := make(map[string]string)
	refs := make([]types.SecretRef, 0)
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		name := strings.TrimSpace(key)
		value := env[key]
		if strings.TrimSpace(value.SecretRef) != "" {
			refs = append(refs, types.SecretRef{Name: strings.TrimSpace(value.SecretRef), Env: name})
			continue
		}
		literals[name] = value.Value
	}
	if len(literals) == 0 {
		literals = nil
	}
	return literals, refs
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
